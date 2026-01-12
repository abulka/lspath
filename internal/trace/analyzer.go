package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"lspath/internal/model"
)

// Analyzer processes trace events to reconstruct the PATH evolution.
type Analyzer struct {
	events []model.TraceEvent
}

func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

func (a *Analyzer) Analyze(events []model.TraceEvent) model.AnalysisResult {
	var flowNodes []model.ConfigNode
	var lastFile string
	var currentNode *model.ConfigNode
	var lastPathStr string
	// Track current attribution for each path entry string
	// Map: PathString -> *PathEntry
	// We need order too.
	// Actually, just tracking the previous list of entries is enough?
	// When PATH changes, we have a new list of strings.
	// For each string in new list:
	//   If it exists in old list (by value), reuse its attribution.
	//   Else, attribute to current node.

	// We need to be careful about duplicates. "A:B:A".
	// If we reuse, we should reuse the *first* matching instance?
	// Or simpler: Map Value -> Entry. But duplicates confuse map.

	// Maintain current list of `[]*model.PathEntry`.
	var currentEntries []*model.PathEntry

	// Maintain a call stack to infer depth manually since zsh trace depth is unreliable for sourcing.
	// Stack contains file paths.
	fileStack := []string{}

	// Helper to calculate depth from stack
	getStackDepth := func() int {
		return len(fileStack) - 1
	}

	nodeCounter := 0

	for _, ev := range events {
		// Flow Graph Construction
		if ev.File != lastFile {
			// Check if this file is "noisy" (system functions)
			// If it is, and it DOES NOT change the PATH, we skip creating a new node
			// and attribute events to the previous node (effectively coalecsing).
			// However, if it changes PATH, we MUST record it.

			isSystem := strings.HasPrefix(ev.File, "/usr/share/zsh") || strings.HasPrefix(ev.File, "/etc/zshrc_Apple_Terminal")
			isPathChange := (ev.PathChange != "")

			if isSystem && !isPathChange {
				// Skip creating a new node, stay on current.
				// But update lastFile so we don't check this every event?
				// No, if we update lastFile, next event will think we are in context.
				// We just effectively "ignore" this switch.
				// But wait, if we ignore the switch, ev.File is different.
				// We should map this event to the *current* flow node.
				// So we do nothing.
			} else {
				// Maintain a call stack to infer depth manually since zsh trace depth is unreliable for sourcing.
				// Stack Management
				// Did we go deeper?

				// Find in stack
				stackIdx := -1
				for i := len(fileStack) - 1; i >= 0; i-- {
					if fileStack[i] == ev.File {
						stackIdx = i
						break
					}
				}

				if stackIdx != -1 {
					// Returning to a parent file
					fileStack = fileStack[:stackIdx+1]
				} else {
					// New file - assume nesting?
					// Unless it's a top level sibling switch.

					isTopLevel := isImportantConfig(ev.File) && !strings.Contains(ev.File, "cargo") && !strings.Contains(ev.File, "nvm")

					if isTopLevel {
						// Force reset for known top-level sequence (zprofile, zshrc, etc)
						// They are triggered by the shell, not sourced by each other usually (except /etc/... -> ~/. ...)
						// Actually, /etc/zprofile might source ~/.zprofile? NO, usually zsh runs them sequentially.
						// BUT, /etc/zshrc sources ~/.zshrc? No, sequential.

						// If it is a System file (/etc/...), it's definitely a new start.
						// If it is a User file (~/...), it might be sourced by System file?
						// Darwin zshrc (/etc/zshrc) sources /etc/zshrc_Apple_Terminal usually.

						// Safer heuristic: If isTopLevel, reset stack to just this file.
						fileStack = []string{ev.File}
					} else {
						// Deeper
						fileStack = append(fileStack, ev.File)
					}
				}

				depth := getStackDepth()
				if depth < 0 {
					depth = 0
				}

				// Create new node
				nodeCounter++
				node := model.ConfigNode{
					ID:          fmt.Sprintf("node-%d", nodeCounter),
					FilePath:    ev.File,
					Order:       nodeCounter,
					Depth:       depth,
					Description: getPathDescription(ev.File),
					Entries:     []int{},
				}
				flowNodes = append(flowNodes, node)
				currentNode = &flowNodes[len(flowNodes)-1]
				lastFile = ev.File
			}
		}

		// Check if this event changes PATH
		if ev.PathChange != "" && ev.PathChange != lastPathStr {
			// Parse the new PATH string
			newPaths := strings.Split(ev.PathChange, ":")
			var newEntries []*model.PathEntry

			// Build a pool of existing entries to reuse
			// To handle duplicates and reordering correctly is tricky.
			// Heuristic: If we see path "P" and we had "P" before, keep the old one.
			// If we had multiple "P"s, which one? The first one?

			// Optimization: Map[Value] -> *Entry (last seen or list?)
			// Let's iterate.

			for _, p := range newPaths {
				if p == "" {
					continue
				}

				// Is this p in currentEntries?
				var existing *model.PathEntry
				for _, curr := range currentEntries {
					if curr.Value == p {
						existing = curr
						break
					}
				}

				if existing != nil {
					// Reuse
					// Make a copy or point to same?
					// If we point to same, we can't detect if it moved?
					// We just want to preserve Source info.
					e := *existing // shallow copy
					// Update Mode? Mode comes later.
					newEntries = append(newEntries, &e)
				} else {
					// New Entry
					e := model.PathEntry{
						Value:      p,
						SourceFile: ev.File,
						LineNumber: ev.Line,
						FlowID:     currentNode.ID,
						Mode:       GuessShellMode(ev.File),
					}
					newEntries = append(newEntries, &e)
				}
			}
			currentEntries = newEntries
			lastPathStr = ev.PathChange
		}
	}

	entries := make([]model.PathEntry, len(currentEntries))
	for i, e := range currentEntries {
		entries[i] = *e
	}

	// Post-process for Duplicates and Disk existence
	seen := make(map[string]int) // value -> index
	for i, e := range entries {
		// 1. Duplicate check
		if firstIdx, ok := seen[e.Value]; ok {
			entries[i].IsDuplicate = true
			entries[i].DuplicateOf = firstIdx

			// Advice
			firstSrc := entries[firstIdx].SourceFile
			entries[i].Remediation = fmt.Sprintf(
				"Duplicate of entry %d (from %s). Check %s:%d to see why it's re-added.",
				firstIdx+1, firstSrc, e.SourceFile, e.LineNumber,
			)
		} else {
			seen[e.Value] = i
		}

		// 2. Disk existence check
		if _, err := os.Stat(e.Value); os.IsNotExist(err) {
			entries[i].Diagnostics = append(entries[i].Diagnostics, "Directory does not exist on disk.")
		}
	}

	// Post-process Flow Graph: Clean up noise
	// 1. Attribute entries to nodes (reverse mapping)
	nodeMap := make(map[string]*model.ConfigNode)
	for i := range flowNodes {
		nodeMap[flowNodes[i].ID] = &flowNodes[i]
	}
	for i, e := range entries {
		if node, ok := nodeMap[e.FlowID]; ok {
			node.Entries = append(node.Entries, i)
		}
	}

	// 2. Filter and Merge
	var cleanNodes []model.ConfigNode
	for _, node := range flowNodes {
		isImportant := isImportantConfig(node.FilePath)
		if len(node.Entries) == 0 && !isImportant {
			continue
		}

		if len(cleanNodes) > 0 {
			last := &cleanNodes[len(cleanNodes)-1]
			if last.FilePath == node.FilePath {
				last.Entries = append(last.Entries, node.Entries...)
				for _, entryIdx := range node.Entries {
					entries[entryIdx].FlowID = last.ID
				}
				continue
			}
		}
		cleanNodes = append(cleanNodes, node)
	}

	// 3. Renumber and inject gaps
	for i := range cleanNodes {
		cleanNodes[i].Order = i + 1
		if cleanNodes[i].Description == "" {
			cleanNodes[i].Description = getPathDescription(cleanNodes[i].FilePath)
		}
	}
	cleanNodes = injectMissingNodes(cleanNodes)
	for i := range cleanNodes {
		cleanNodes[i].Order = i + 1
	}

	globalDiagnostics := []string{}

	// Shell Mode Advice
	if isLoginShell(cleanNodes) {
		globalDiagnostics = append(globalDiagnostics, "INFO: Detected as a LOGIN shell. This is typical for terminal startups on macOS.")
	} else {
		globalDiagnostics = append(globalDiagnostics, "INFO: Detected as an INTERACTIVE (non-login) shell.")
	}

	// Priority checks
	brewIdx := -1
	usrLocalIdx := -1
	for i, e := range entries {
		if strings.HasPrefix(e.Value, "/opt/homebrew") || strings.HasPrefix(e.Value, "/usr/local/bin") {
			if strings.HasPrefix(e.Value, "/opt/homebrew") && brewIdx == -1 {
				brewIdx = i
			}
			if strings.HasPrefix(e.Value, "/usr/local/bin") && usrLocalIdx == -1 {
				usrLocalIdx = i
			}
		}
	}
	if brewIdx != -1 && usrLocalIdx != -1 && usrLocalIdx < brewIdx {
		globalDiagnostics = append(globalDiagnostics, "ADVICE: /usr/local/bin appears before Homebrew in PATH. Brew packages may be shadowed by system-installed ones.")
	}

	return model.AnalysisResult{
		PathEntries: entries,
		FlowNodes:   cleanNodes,
		Diagnostics: globalDiagnostics,
	}
}

func getPathDescription(path string) string {
	if strings.HasPrefix(path, "/etc/") {
		if strings.Contains(path, "env") {
			return "(system-wide env)"
		}
		if strings.Contains(path, "profile") {
			return "(system-wide profile)"
		}
		if strings.Contains(path, "rc") {
			return "(system-wide rc)"
		}
		return "(system-wide)"
	}
	if strings.Contains(path, "/.zshrc") || strings.Contains(path, "/.zprofile") || strings.Contains(path, "/.zshenv") ||
		strings.Contains(path, "/.zlogin") || strings.Contains(path, "/.profile") || strings.HasPrefix(path, "~") {
		return "(user-specific)"
	}
	return ""
}

func isLoginShell(nodes []model.ConfigNode) bool {
	for _, n := range nodes {
		if strings.Contains(n.FilePath, "zprofile") || strings.Contains(n.FilePath, "zlogin") || strings.Contains(n.FilePath, "bash_profile") {
			if !n.NotExecuted {
				return true
			}
		}
	}
	return false
}

// GenerateReport creates a human-readable text report of the analysis.
func GenerateReport(res model.AnalysisResult, verbose bool) string {
	var sb strings.Builder
	sb.WriteString("LS-PATH ANALYSIS REPORT\n")
	sb.WriteString("========================\n\n")

	sb.WriteString("GLOBAL DIAGNOSTICS\n")
	sb.WriteString("------------------\n")
	if len(res.Diagnostics) == 0 {
		sb.WriteString("No global issues detected.\n")
	} else {
		for _, d := range res.Diagnostics {
			sb.WriteString("• " + d + "\n")
		}
	}
	sb.WriteString("\n")

	if verbose {
		sb.WriteString("PATH ENTRIES (PRIORITY ORDER)\n")
		sb.WriteString("-----------------------------\n\n")
		lastCat := ""
		for i, e := range res.PathEntries {
			cat := getPathCategory(e.Value)
			if cat != lastCat {
				sb.WriteString(fmt.Sprintf("[%s]\n", strings.ToUpper(cat)))
				lastCat = cat
			}

			status := "OK     "
			if e.IsDuplicate {
				status = "DUP    "
			} else if isMissing(e.Value) {
				status = "MISSING"
			}

			sb.WriteString(fmt.Sprintf(" %2d. [%-7s] %s\n", i+1, status, e.Value))
			sb.WriteString(fmt.Sprintf("    Source: %s:%d (%s)\n", e.SourceFile, e.LineNumber, e.Mode))
			if !e.IsDuplicate && !isMissing(e.Value) {
				sb.WriteString(fmt.Sprintf("    Stats:  %s\n", getDirStats(e.Value)))
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString(fmt.Sprintf("PATH (%d ENTRIES) - Use --verbose (or 'v' in TUI) for details\n", len(res.PathEntries)))
		sb.WriteString("-----------------------------------------------------------\n\n")
		for i, e := range res.PathEntries {
			icon := "✓"
			statusLabel := ""
			if e.IsDuplicate {
				icon = "⚡"
				statusLabel = fmt.Sprintf(" [DUPLICATE → see #%d]", e.DuplicateOf+1)
			} else if isMissing(e.Value) {
				icon = "⚠"
				statusLabel = " [MISSING]"
			}

			displayPath := e.Value
			if len(displayPath) > 60 {
				displayPath = displayPath[:57] + "..."
			}
			sb.WriteString(fmt.Sprintf("%s %2d. %s%s\n", icon, i+1, displayPath, statusLabel))
		}
		sb.WriteString("\n")
	}

	// Summary Section
	sb.WriteString("SUMMARY\n")
	sb.WriteString("-------\n")
	okCount, dupCount, missCount := 0, 0, 0
	sources := make(map[string]int)
	for _, e := range res.PathEntries {
		if e.IsDuplicate {
			dupCount++
		} else if isMissing(e.Value) {
			missCount++
		} else {
			okCount++
		}
		src := e.SourceFile
		if strings.HasPrefix(src, "/Users/") {
			parts := strings.Split(src, "/")
			if len(parts) > 3 {
				src = "~/" + strings.Join(parts[3:], "/")
			}
		}
		sources[src]++
	}

	total := len(res.PathEntries)
	sb.WriteString(fmt.Sprintf("Total PATH Entries: %d\n", total))
	if total > 0 {
		sb.WriteString(fmt.Sprintf("├─ OK:       %2d (%d%%)\n", okCount, okCount*100/total))
		sb.WriteString(fmt.Sprintf("├─ Missing:  %2d (%d%%)\n", missCount, missCount*100/total))
		sb.WriteString(fmt.Sprintf("└─ Duplicate: %2d (%d%%)\n", dupCount, dupCount*100/total))
	}

	sb.WriteString("\n")

	// Issues Section
	sb.WriteString("ISSUES FOUND\n")
	sb.WriteString("------------\n")
	foundAny := false

	// Duplicates
	if dupCount > 0 {
		foundAny = true
		sb.WriteString(fmt.Sprintf("DUPLICATES (%d) [NOT SERIOUS]\n", dupCount))
		for i, e := range res.PathEntries {
			if e.IsDuplicate {
				sb.WriteString(fmt.Sprintf("%2d. %s\n", i+1, e.Value))
				sb.WriteString(fmt.Sprintf("    » Already added at entry %d\n", e.DuplicateOf+1))
				sb.WriteString(fmt.Sprintf("    » Remove from: %s:%d\n\n", e.SourceFile, e.LineNumber))
			}
		}
	}

	// Missing
	if missCount > 0 {
		foundAny = true
		sb.WriteString(fmt.Sprintf("MISSING DIRECTORIES (%d) [NOT SERIOUS]\n", missCount))
		for i, e := range res.PathEntries {
			if isMissing(e.Value) {
				sb.WriteString(fmt.Sprintf("%2d. %s (from %s:%d)\n", i+1, e.Value, e.SourceFile, e.LineNumber))
			}
		}
		sb.WriteString("\n")
	}

	if !foundAny {
		sb.WriteString("No specific issues found.\n\n")
	}

	sb.WriteString("CONFIGURATION FILES FLOW\n")
	sb.WriteString("------------------------\n")
	for _, n := range res.FlowNodes {
		indent := strings.Repeat("  ", n.Depth)
		status := ""
		if n.NotExecuted {
			status = " [Not Executed]"
		} else if len(n.Entries) > 0 {
			status = fmt.Sprintf(" [%d paths]", len(n.Entries))
		} else {
			status = " [no change]"
		}
		desc := ""
		if n.Description != "" {
			desc = " " + n.Description
		}
		// logic labels match TUI: (executed first/last) added?
		// User said: include same helpful labels
		execLabel := ""
		if n.Order == 1 {
			execLabel = " (executed first)"
		} else if n.Order == len(res.FlowNodes) {
			execLabel = " (executed last)"
		}

		sb.WriteString(fmt.Sprintf("%2d. %s%s%s%s%s\n", n.Order, indent, n.FilePath, desc, status, execLabel))
	}

	if verbose {
		sb.WriteString("\nINTERNAL MODEL (VERBOSE)\n")
		sb.WriteString("-----------------------\n")
		modelJson, _ := json.MarshalIndent(res, "", "  ")
		sb.WriteString(string(modelJson))
	}

	return sb.String()
}

func isMissing(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

type standardConfig struct {
	PathSuffix string
	Rank       int
}

var zshStandard = []standardConfig{
	{"/etc/zshenv", 1},
	{"/.zshenv", 2},
	{"/etc/zprofile", 3},
	{"/.zprofile", 4},
	{"/etc/zshrc", 5},
	{"/.zshrc", 6},
	{"/etc/zlogin", 7},
	{"/.zlogin", 8},
}

func injectMissingNodes(nodes []model.ConfigNode) []model.ConfigNode {
	var result []model.ConfigNode
	standardIdx := 0

	// Helper to get rank of a node (if it matches standard)
	getRank := func(path string) int {
		for _, s := range zshStandard {
			if strings.HasSuffix(path, s.PathSuffix) {
				return s.Rank
			}
		}
		return 999 // Non-standard / Nested
	}

	// We iterate through the actual trace nodes.
	// Before adding an actual node, we check if we skipped any standard nodes that have a lower Rank.

	for _, node := range nodes {
		nodeRank := getRank(node.FilePath)

		// If the current actual node has a rank, we fill in gaps up to that rank.
		if nodeRank != 999 {
			for standardIdx < len(zshStandard) {
				std := zshStandard[standardIdx]
				if std.Rank < nodeRank {
					// This standard file comes BEFORE the current node, and we haven't seen it.
					// Insert it.
					// Construct a nice display path.
					// We don't know the absolute path easily for HOME, but we can guess.
					// Or just use the suffix for unique ID and let View handle display?
					// View expects FilePath.
					// For /etc, use absolute. For home, use ~/.

					displayPath := std.PathSuffix
					if strings.HasPrefix(displayPath, "/.") {
						displayPath = "~" + displayPath // ~/.zshrc
					}

					result = append(result, model.ConfigNode{
						ID:          fmt.Sprintf("ghost-%d", std.Rank), // Temp ID
						FilePath:    displayPath,
						Depth:       0,
						NotExecuted: true,
						Description: getPathDescription(std.PathSuffix),
						Entries:     []int{},
					})
					standardIdx++
				} else if std.Rank == nodeRank {
					// Match!
					standardIdx++ // Advance standard
					break         // Stop checking gaps, add the actual node below
				} else {
					// Standard rank > Node rank.
					// Should not happen if we are sorted?
					// But maybe we are re-visiting a lower rank (e.g. zshrc sourcing zshenv??)
					// If we go BACKWARDS in rank, we just ignore the gap filling?
					break
				}
			}
		}

		// Add the actual node
		// If it was a match, we effectively "replaced" the ghost with real.
		// If it was non-standard (999), we just append it (nested file).
		result = append(result, node)
	}

	// Append remaining standard files
	for standardIdx < len(zshStandard) {
		std := zshStandard[standardIdx]
		displayPath := std.PathSuffix
		if strings.HasPrefix(displayPath, "/.") {
			displayPath = "~" + displayPath
		}
		result = append(result, model.ConfigNode{
			ID:          fmt.Sprintf("ghost-%d", std.Rank),
			FilePath:    displayPath,
			Depth:       0,
			NotExecuted: true,
			Description: getPathDescription(std.PathSuffix),
			Entries:     []int{},
		})
		standardIdx++
	}

	return result
}

// GuessShellMode infers shell mode from filename.
func GuessShellMode(filename string) string {
	if strings.Contains(filename, "zprofile") || strings.Contains(filename, "zlogin") || strings.Contains(filename, "bash_profile") || strings.Contains(filename, "profile") {
		return "Login"
	}
	if strings.Contains(filename, "zshrc") || strings.Contains(filename, "bashrc") {
		return "Interactive"
	}
	if strings.Contains(filename, "zshenv") || strings.Contains(filename, "environment") {
		return "Env/All"
	}
	return "Unknown"
}

// isImportantConfig checks if a file is a standard shell configuration file
// that should be shown in the flow even if empty.
func isImportantConfig(path string) bool {
	// Check standard zsh/bash files
	// Use Contains or Suffix to handle absolute paths
	keys := []string{
		"zshenv", ".zshenv",
		"zprofile", ".zprofile",
		"zshrc", ".zshrc",
		"zlogin", ".zlogin",
		"zlogout", ".zlogout",
		"bash_profile", ".bash_profile",
		"bashrc", ".bashrc",
		"profile", ".profile",
		"bash_login",
	}

	for _, k := range keys {
		if strings.HasSuffix(path, "/"+k) || path == k {
			return true
		}
	}
	return false
}
func getPathCategory(path string) string {
	p := strings.ToLower(path)

	// Tools & Languages
	if strings.Contains(p, "flutter") || strings.Contains(p, "cargo") || strings.Contains(p, "go/bin") ||
		strings.Contains(p, "dotnet") || strings.Contains(p, "dart") || strings.Contains(p, "rust") ||
		strings.Contains(p, "antigravity") || strings.Contains(p, "bun") {
		return "User Tools & Languages"
	}

	// Version Managers
	if strings.Contains(p, "nvm") || strings.Contains(p, "nodenv") ||
		strings.Contains(p, "pyenv") || strings.Contains(p, "rbenv") {
		return "Version Managers"
	}

	// Package Managers
	if strings.HasPrefix(p, "/opt/homebrew") || strings.HasPrefix(p, "/usr/local") ||
		strings.Contains(p, "cellar") || strings.Contains(p, "npm") {
		return "Package Managers"
	}

	// User Binaries
	if strings.HasPrefix(p, "/users/") && (strings.Contains(p, "/bin") || strings.Contains(p, "/.local/bin")) {
		return "User Binaries"
	}

	// System Paths
	if strings.HasPrefix(p, "/usr/bin") || strings.HasPrefix(p, "/bin") ||
		strings.HasPrefix(p, "/usr/sbin") || strings.HasPrefix(p, "/sbin") ||
		strings.HasPrefix(p, "/system") {
		return "System Paths"
	}

	// Applications
	if strings.Contains(p, "/applications/") || strings.Contains(p, ".app/") {
		return "Applications"
	}

	return "Other Paths"
}

func getDirStats(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "unknown"
	}

	f, err := os.Open(path)
	if err != nil {
		return "unknown"
	}
	defer f.Close()

	files, err := f.Readdirnames(-1)
	if err != nil {
		return "unknown"
	}

	fileCount := 0
	dirCount := 0
	for _, name := range files {
		full := path
		if !strings.HasSuffix(full, "/") {
			full += "/"
		}
		full += name

		if info, err := os.Stat(full); err == nil {
			if info.IsDir() {
				dirCount++
			} else {
				fileCount++
			}
		}
	}

	mode := info.Mode().String()
	mtime := info.ModTime().Format("2006-01-02 15:04")
	return fmt.Sprintf("%d files, %d dirs (Perms: %s, Mod: %s)", fileCount, dirCount, mode, mtime)
}
