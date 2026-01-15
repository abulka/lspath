package trace

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"lspath/internal/model"
)

// expandTilde expands ~ to the user's home directory for path normalization
func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	} else if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
	}
	return path
}

// isLikelySystemPath returns true if the path looks like it should be part
// of the system default PATH rather than a session-specific addition.
// Common system paths that might be added by /etc/bash.bashrc or /etc/environment
// but missed in the trace due to the minimal SandboxInitialPath baseline.
func isLikelySystemPath(path string) bool {
	commonSystemPaths := []string{
		"/usr/local/sbin",
		"/usr/local/bin",
		"/usr/sbin",
		"/usr/bin",
		"/sbin",
		"/bin",
		"/usr/games",
		"/usr/local/games",
		"/snap/bin",
		"/opt/local/bin",
		"/opt/local/sbin",
	}

	for _, sysPath := range commonSystemPaths {
		if path == sysPath {
			return true
		}
	}

	return false
}

// getLineFromFile reads a specific line number from a file
func getLineFromFile(filePath string, lineNum int) string {
	f, err := os.Open(expandTilde(filePath))
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	currentLine := 0
	for scanner.Scan() {
		currentLine++
		if currentLine == lineNum {
			return strings.TrimSpace(scanner.Text())
		}
	}
	return ""
}

// Analyzer processes trace events to reconstruct the PATH evolution.
type Analyzer struct {
	events []model.TraceEvent
}

func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

// AnalyzeSessionPath analyzes the current PATH directly without running a trace.
// This gives an accurate view of the current session's PATH without duplicates
// caused by re-running shell startup scripts.
func (a *Analyzer) AnalyzeSessionPath(currentPath string) model.AnalysisResult {
	var entries []model.PathEntry

	// Create a single "Current Session" node
	sessionNode := model.ConfigNode{
		ID:          "node-0",
		FilePath:    "Current Session",
		Order:       0,
		Depth:       0,
		Description: "Your current terminal session's PATH",
		Entries:     []int{},
	}

	// Parse the PATH
	parts := strings.Split(currentPath, ":")
	for _, p := range parts {
		if p == "" {
			continue
		}
		entries = append(entries, model.PathEntry{
			Value:           p,
			SourceFile:      "Current Session",
			LineNumber:      0,
			Mode:            "Session",
			FlowID:          "node-0",
			SymlinkPointsTo: -1,
		})
	}

	// Post-process for duplicates and disk existence
	seen := make(map[string]int)
	resolvedPaths := make(map[string]int)

	for i := range entries {
		e := &entries[i]
		normalizedPath := expandTilde(e.Value)

		// Check if this path is a symlink
		fileInfo, err := os.Lstat(normalizedPath)
		var resolvedPath string
		if err == nil && fileInfo.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(normalizedPath)
			if err == nil {
				var absTarget string
				if filepath.IsAbs(target) {
					absTarget = target
				} else {
					parent := filepath.Dir(normalizedPath)
					absTarget = filepath.Join(parent, target)
				}
				absTarget = filepath.Clean(absTarget)
				e.IsSymlink = true
				e.SymlinkTarget = absTarget
				resolvedPath = absTarget
			}
		}

		if resolvedPath == "" {
			resolvedPath = normalizedPath
		}

		// Duplicate check
		if firstIdx, ok := seen[normalizedPath]; ok {
			e.IsDuplicate = true
			e.DuplicateOf = firstIdx
			e.DuplicateMessage = fmt.Sprintf(
				"Duplicates PATH entry #%d (%s)",
				firstIdx+1, entries[firstIdx].Value,
			)
		} else if e.IsSymlink {
			if firstIdx, ok := resolvedPaths[resolvedPath]; ok {
				e.SymlinkPointsTo = firstIdx
				e.SymlinkMessage = fmt.Sprintf(
					"Symlink resolves to PATH entry #%d (%s)",
					firstIdx+1, e.SymlinkTarget,
				)
			}
		}

		if !e.IsDuplicate {
			seen[normalizedPath] = i
			resolvedPaths[resolvedPath] = i
		}

		// Disk existence check
		if _, err := os.Stat(normalizedPath); os.IsNotExist(err) {
			e.Diagnostics = append(e.Diagnostics, "Directory does not exist on disk.")
		}

		// Add to session node's entries
		sessionNode.Entries = append(sessionNode.Entries, i)
	}

	globalDiagnostics := []string{
		"INFO: Showing current session PATH. Use --trace flag to see where paths originate from shell config files.",
	}

	return model.AnalysisResult{
		PathEntries: entries,
		FlowNodes:   []model.ConfigNode{sessionNode},
		Diagnostics: globalDiagnostics,
	}
}

// AnalyzeUnified runs both session and trace analysis, then merges them.
// Session PATH entries that don't appear in trace are marked as session-only.
// This provides the most complete view: actual PATH with full attribution.
func (a *Analyzer) AnalyzeUnified(sessionPath string, events []model.TraceEvent) model.AnalysisResult {
	// First, run the trace analysis to get config-based attribution and full flow structure
	traceResult := a.Analyze(events, SandboxInitialPath)

	// Build a map of traced paths for quick lookup (path value -> entry)
	tracedPaths := make(map[string]*model.PathEntry)
	for i := range traceResult.PathEntries {
		entry := &traceResult.PathEntries[i]
		if _, exists := tracedPaths[entry.Value]; !exists {
			tracedPaths[entry.Value] = entry
		}
	}

	// Process the actual session PATH in order
	sessionParts := strings.Split(sessionPath, ":")
	var unifiedEntries []model.PathEntry
	var sessionOnlyEntries []int // indices of session-only entries

	for _, pathValue := range sessionParts {
		if pathValue == "" {
			continue
		}

		entryIdx := len(unifiedEntries)

		// Check if this path was in the trace
		tracedEntry, inTrace := tracedPaths[pathValue]

		var entry model.PathEntry
		if inTrace {
			// Use trace attribution - copy the entry
			entry = *tracedEntry
			entry.SymlinkPointsTo = -1 // Will be recalculated
			entry.IsDuplicate = false  // Will be recalculated
			entry.DuplicateOf = 0
			entry.DuplicateMessage = ""
		} else {
			// Not in trace - could be session-only OR could be a system path
			// that the trace missed due to starting with minimal SandboxInitialPath
			if isLikelySystemPath(pathValue) {
				// Attribute to System (Default) rather than marking as session-only
				entry = model.PathEntry{
					Value:           pathValue,
					SourceFile:      "System (Default)",
					LineNumber:      0,
					Mode:            "System",
					IsSessionOnly:   false,
					SymlinkPointsTo: -1,
					FlowID:          "node-0",
				}
				// Don't add to sessionOnlyEntries, add to System node instead
			} else {
				// Truly session-only entry (e.g., virtualenv, manual export)
				entry = model.PathEntry{
					Value:           pathValue,
					SourceFile:      "Session (Manual/Runtime)",
					LineNumber:      0,
					Mode:            "Session",
					IsSessionOnly:   true,
					SessionNote:     "Added manually or by runtime tool (not in shell config)",
					SymlinkPointsTo: -1,
					FlowID:          "session-node",
				}
				sessionOnlyEntries = append(sessionOnlyEntries, entryIdx)
			}
		}

		unifiedEntries = append(unifiedEntries, entry)
	}

	// Use the trace's flow nodes as base (preserves shell startup order, depth, all config files)
	flowNodes := traceResult.FlowNodes

	// Remap flow node entries to point to unified entry indices
	// Build a map from old trace path value -> new unified index
	// Also track system paths that need to be added to the System (Default) node
	pathToUnifiedIdx := make(map[string][]int)
	var systemNodeEntries []int

	for i, entry := range unifiedEntries {
		if entry.IsSessionOnly {
			continue // Skip session-only entries
		}

		if entry.FlowID == "node-0" && entry.SourceFile == "System (Default)" {
			// This is a system path that wasn't in the trace but we attributed to system
			systemNodeEntries = append(systemNodeEntries, i)
		} else {
			// Normal traced entry
			pathToUnifiedIdx[entry.Value] = append(pathToUnifiedIdx[entry.Value], i)
		}
	}

	// Update each flow node's entries to point to unified indices
	for i := range flowNodes {
		var newEntries []int

		// Special handling for System (Default) node - add the extra system paths
		if flowNodes[i].FilePath == "System (Default)" {
			// First add the originally traced entries
			for _, oldIdx := range flowNodes[i].Entries {
				if oldIdx < len(traceResult.PathEntries) {
					pathValue := traceResult.PathEntries[oldIdx].Value
					if indices, ok := pathToUnifiedIdx[pathValue]; ok && len(indices) > 0 {
						newEntries = append(newEntries, indices[0])
						pathToUnifiedIdx[pathValue] = indices[1:]
					}
				}
			}
			// Then add any system paths we detected from session but weren't in trace
			newEntries = append(newEntries, systemNodeEntries...)
		} else {
			// For other nodes, just remap the entries
			for _, oldIdx := range flowNodes[i].Entries {
				if oldIdx < len(traceResult.PathEntries) {
					pathValue := traceResult.PathEntries[oldIdx].Value
					if indices, ok := pathToUnifiedIdx[pathValue]; ok && len(indices) > 0 {
						newEntries = append(newEntries, indices[0])
						pathToUnifiedIdx[pathValue] = indices[1:]
					}
				}
			}
		}

		flowNodes[i].Entries = newEntries
	}

	// Add session-only node if there are session-only entries
	if len(sessionOnlyEntries) > 0 {
		sessionNode := model.ConfigNode{
			ID:          "session-node",
			FilePath:    "Session (Manual/Runtime)",
			Order:       0, // Will be set properly below
			Depth:       0,
			Description: "Paths added in this terminal session",
			Entries:     sessionOnlyEntries,
		}

		// Insert AFTER "System (Default)" node but before other config files
		// Find the System (Default) node (should be first, but let's be safe)
		insertPos := 0
		for i, node := range flowNodes {
			if node.FilePath == "System (Default)" {
				insertPos = i + 1
				break
			}
		}

		// Insert at the calculated position
		flowNodes = append(flowNodes[:insertPos], append([]model.ConfigNode{sessionNode}, flowNodes[insertPos:]...)...)

		// Renumber orders
		for i := range flowNodes {
			flowNodes[i].Order = i + 1
		}
	}

	// FlowID is already preserved from the trace entry copy, no need to remap.
	// The trace correctly distinguishes between continuation nodes (e.g., .zshrc
	// before and after sourcing nvm.sh), so we keep the original FlowID.

	// Post-process for duplicates, symlinks, and disk existence
	seen := make(map[string]int)
	resolvedPaths := make(map[string]int)

	for i := range unifiedEntries {
		e := &unifiedEntries[i]
		normalizedPath := expandTilde(e.Value)

		// Check if this path is a symlink
		fileInfo, err := os.Lstat(normalizedPath)
		var resolvedPath string
		if err == nil && fileInfo.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(normalizedPath)
			if err == nil {
				var absTarget string
				if filepath.IsAbs(target) {
					absTarget = target
				} else {
					parent := filepath.Dir(normalizedPath)
					absTarget = filepath.Join(parent, target)
				}
				absTarget = filepath.Clean(absTarget)
				e.IsSymlink = true
				e.SymlinkTarget = absTarget
				resolvedPath = absTarget
			}
		}

		if resolvedPath == "" {
			resolvedPath = normalizedPath
		}

		// Duplicate check
		if firstIdx, ok := seen[normalizedPath]; ok {
			e.IsDuplicate = true
			e.DuplicateOf = firstIdx
			e.DuplicateMessage = fmt.Sprintf(
				"Duplicates PATH entry #%d (%s)",
				firstIdx+1, unifiedEntries[firstIdx].Value,
			)
		} else if e.IsSymlink {
			if firstIdx, ok := resolvedPaths[resolvedPath]; ok {
				e.SymlinkPointsTo = firstIdx
				e.SymlinkMessage = fmt.Sprintf(
					"Symlink resolves to PATH entry #%d (%s)",
					firstIdx+1, e.SymlinkTarget,
				)
			}
		}

		if !e.IsDuplicate {
			seen[normalizedPath] = i
			resolvedPaths[resolvedPath] = i
		}

		// Disk existence check
		if _, err := os.Stat(normalizedPath); os.IsNotExist(err) {
			e.Diagnostics = append(e.Diagnostics, "Directory does not exist on disk.")
		}
	}

	globalDiagnostics := []string{
		"INFO: Unified view - showing your actual PATH with full attribution.",
		"INFO: Entries marked as 'Session' were added manually or by tools (not from shell config files).",
	}

	return model.AnalysisResult{
		PathEntries: unifiedEntries,
		FlowNodes:   flowNodes,
		Diagnostics: globalDiagnostics,
	}
}

func (a *Analyzer) Analyze(events []model.TraceEvent, initialPath string) model.AnalysisResult {
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

	// Initialize lastPathStr with the baseline so diffs work correctly
	lastPathStr = initialPath

	// Maintain current list of `[]*model.PathEntry`.
	var currentEntries []*model.PathEntry

	// --- NEW LOGIC: Pre-populate from the passed argument ---
	if initialPath != "" {
		// Prepend Node 0 for System configuration
		flowNodes = append(flowNodes, model.ConfigNode{
			ID:          "node-0",
			FilePath:    "System (Default)",
			Order:       0,
			Depth:       0,
			Description: "Initial environment PATH",
			Entries:     []int{},
		})

		parts := strings.Split(initialPath, ":")
		for _, p := range parts {
			if p == "" {
				continue
			}
			currentEntries = append(currentEntries, &model.PathEntry{
				Value:      p,
				SourceFile: "System (Default)", // Or "(initial environment)"
				LineNumber: 0,
				Mode:       "System",
				FlowID:     "node-0", // Assign to the system node
			})
		}
	}
	// -------------------------------------------------------

	// Maintain a call stack to infer depth manually since zsh trace depth is unreliable for sourcing.
	// Stack contains file paths.
	fileStack := []string{}

	// Helper to calculate depth from stack
	getStackDepth := func() int {
		return len(fileStack) - 1
	}

	// Track eval contexts to properly attribute PATH changes
	// Map: file -> evalLine
	// We track evals that contain command substitution ($(...) or backticks)
	// because these can produce output that modifies PATH
	evalContext := make(map[string]int)
	// Track which files have had a PATH change attributed to an eval
	evalUsed := make(map[string]bool)

	nodeCounter := 0

	for _, ev := range events {
		// Detect eval commands with command substitution and track their line numbers
		if strings.Contains(ev.RawCommand, "eval ") &&
			(strings.Contains(ev.RawCommand, "$(") || strings.Contains(ev.RawCommand, "`")) {
			evalContext[ev.File] = ev.Line
			evalUsed[ev.File] = false
		}
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
					// Check if we're in an eval context
					lineNum := ev.Line
					if evalLine, inEval := evalContext[ev.File]; inEval && !evalUsed[ev.File] && ev.Line > evalLine {
						// This PATH change is happening after an eval on an earlier line
						// Attribute it to the eval's line instead
						lineNum = evalLine
						// Mark this eval as used so subsequent PATH changes get their real line numbers
						evalUsed[ev.File] = true
					}

					e := model.PathEntry{
						Value:      p,
						SourceFile: ev.File,
						LineNumber: lineNum,
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
		// Initialize SymlinkPointsTo to -1 to indicate "not a symlink" or "doesn't point to another entry"
		entries[i].SymlinkPointsTo = -1
	}

	// Post-process for Duplicates and Disk existence
	seen := make(map[string]int)          // normalized value -> index
	resolvedPaths := make(map[string]int) // resolved symlink path -> index

	for i, e := range entries {
		// Normalize path for comparison (expand ~)
		normalizedPath := expandTilde(e.Value)

		// Check if THIS path itself (not parent directories) is a symlink
		fileInfo, err := os.Lstat(normalizedPath)
		var resolvedPath string
		if err == nil && fileInfo.Mode()&os.ModeSymlink != 0 {
			// It's a direct symlink - read the immediate target
			target, err := os.Readlink(normalizedPath)
			if err == nil {
				// Convert relative symlink target to absolute path
				var absTarget string
				if filepath.IsAbs(target) {
					absTarget = target
				} else {
					// Relative symlink - resolve relative to parent directory
					parent := filepath.Dir(normalizedPath)
					absTarget = filepath.Join(parent, target)
				}
				// Clean the path to normalize it
				absTarget = filepath.Clean(absTarget)

				entries[i].IsSymlink = true
				entries[i].SymlinkTarget = absTarget
				resolvedPath = absTarget
				entries[i].SymlinkPointsTo = -1 // Will be set below if it matches another entry
			}
		}

		if resolvedPath == "" {
			resolvedPath = normalizedPath
		}

		// 1. Duplicate check - check both normalized path and resolved path
		if firstIdx, ok := seen[normalizedPath]; ok {
			entries[i].IsDuplicate = true
			entries[i].DuplicateOf = firstIdx

			// Advice - different message if both entries come from the same source
			orig := entries[firstIdx]
			if e.SourceFile == orig.SourceFile && e.LineNumber == orig.LineNumber {
				// Same source - likely a tracing limitation or path was already in $PATH
				entries[i].DuplicateMessage = fmt.Sprintf(
					"Duplicates PATH entry #%d which was already in $PATH",
					firstIdx+1,
				)
				entries[i].Remediation = fmt.Sprintf(
					"Advice: remove line %d from %s (tentative, advice may be wrong due to shell tracing limitations)",
					firstIdx+1, e.SourceFile,
				)
			} else {
				entries[i].DuplicateMessage = fmt.Sprintf(
					"Duplicates PATH entry #%d (from line %d of %s)",
					firstIdx+1, orig.LineNumber, orig.SourceFile,
				)
				entries[i].Remediation = fmt.Sprintf(
					"Advice: remove line %d from %s (tentative, advice may be wrong due to shell tracing limitations)",
					firstIdx+1, orig.SourceFile,
				)
			}
		} else if entries[i].IsSymlink {
			// Check if this symlink's target matches another PATH entry
			if firstIdx, ok := resolvedPaths[resolvedPath]; ok {
				entries[i].SymlinkPointsTo = firstIdx
				entries[i].SymlinkMessage = fmt.Sprintf(
					"Symlink resolves to PATH entry #%d (%s)",
					firstIdx+1, entries[i].SymlinkTarget,
				)
			}
		}

		// Always add to maps for future comparisons
		if !entries[i].IsDuplicate {
			seen[normalizedPath] = i
			resolvedPaths[resolvedPath] = i
		}

		// 2. Disk existence check (use normalized path)
		if _, err := os.Stat(normalizedPath); os.IsNotExist(err) {
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

	// Add trace mode explanation
	globalDiagnostics = append(globalDiagnostics, "INFO: Trace Mode - showing PATH derived from shell config files. This is a \"pure\" view of what a fresh terminal would have. Session-specific paths (e.g., activated virtual environments) are not shown.")

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
	if path == "System (Default)" {
		return "Initial environment PATH"
	}
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
		sb.WriteString(fmt.Sprintf("PATH ENTRIES (%d ENTRIES) - PRIORITY ORDER\n", len(res.PathEntries)))
		sb.WriteString("--------------------------------------------\n\n")
		for i, e := range res.PathEntries {
			cat := getPathCategory(e.Value)
			pathMissing := isMissing(e.Value)

			// Determine status icon (same as non-verbose mode)
			statusIcon := model.IconOK
			if e.IsSessionOnly {
				statusIcon = model.IconSession
			} else if e.IsDuplicate || e.SymlinkPointsTo >= 0 {
				statusIcon = model.IconDuplicate
			} else if pathMissing {
				statusIcon = model.IconMissing
			}

			// Build suffix labels (same as non-verbose mode)
			suffixLabel := ""
			if e.IsDuplicate {
				origPath := res.PathEntries[e.DuplicateOf].Value
				suffixLabel = fmt.Sprintf(" [duplicate → #%d: %s]", e.DuplicateOf+1, origPath)
			} else if e.SymlinkPointsTo >= 0 {
				targetPath := res.PathEntries[e.SymlinkPointsTo].Value
				suffixLabel = fmt.Sprintf(" [duplicate, symlink → #%d: %s]", e.SymlinkPointsTo+1, targetPath)
			} else if pathMissing {
				suffixLabel = " (missing)"
			}

			// Priority indicators
			if i == 0 {
				suffixLabel += " (highest priority " + model.IconPriorityHigh + ")"
			} else if i == len(res.PathEntries)-1 {
				suffixLabel += " (lowest priority " + model.IconPriorityLow + ")"
			}

			sb.WriteString(fmt.Sprintf("%2d. %s %s%s\n", i+1, statusIcon, e.Value, suffixLabel))

			// Source line
			if e.LineNumber == 0 {
				sb.WriteString(fmt.Sprintf("      - Source: %s\n", e.SourceFile))
			} else {
				sb.WriteString(fmt.Sprintf("      - Source: %s:%d\n", e.SourceFile, e.LineNumber))
			}

			// Path Contains line
			if !pathMissing {
				sb.WriteString(fmt.Sprintf("      - Path Contains: %s\n", getDirStats(e.Value)))
			} else {
				sb.WriteString("      - Path Contains: does not exist\n")
			}

			// Startup Phase line
			if e.Mode != "Unknown" {
				sb.WriteString(fmt.Sprintf("      - Startup Phase: %s\n", e.Mode))
			}

			// Category line
			sb.WriteString(fmt.Sprintf("      - Category: %s\n", cat))
		}
	} else {
		sb.WriteString(fmt.Sprintf("PATH (%d ENTRIES) - Use --verbose (or 'v' in TUI) for details\n", len(res.PathEntries)))
		sb.WriteString("-----------------------------------------------------------\n\n")
		for i, e := range res.PathEntries {
			// Determine status icon
			statusIcon := model.IconOK
			if e.IsSessionOnly {
				statusIcon = model.IconSession
			} else if e.IsDuplicate || e.SymlinkPointsTo >= 0 {
				statusIcon = model.IconDuplicate
			} else if isMissing(e.Value) {
				statusIcon = model.IconMissing
			}

			// Build suffix labels
			suffixLabel := ""
			if e.IsDuplicate {
				origPath := res.PathEntries[e.DuplicateOf].Value
				suffixLabel = fmt.Sprintf(" [duplicate → #%d: %s]", e.DuplicateOf+1, origPath)
			} else if e.SymlinkPointsTo >= 0 {
				targetPath := res.PathEntries[e.SymlinkPointsTo].Value
				suffixLabel = fmt.Sprintf(" [duplicate, symlink → #%d: %s]", e.SymlinkPointsTo+1, targetPath)
			} else if isMissing(e.Value) {
				suffixLabel = " (missing)"
			}

			// Priority indicators
			if i == 0 {
				suffixLabel += " (highest priority " + model.IconPriorityHigh + ")"
			} else if i == len(res.PathEntries)-1 {
				suffixLabel += " (lowest priority " + model.IconPriorityLow + ")"
			}

			displayPath := e.Value
			if len(displayPath) > 60 {
				displayPath = displayPath[:57] + "..."
			}
			sb.WriteString(fmt.Sprintf("%2d. %s %s%s\n", i+1, statusIcon, displayPath, suffixLabel))
		}
		sb.WriteString("\n")
	}

	// Summary Section
	sb.WriteString("SUMMARY\n")
	sb.WriteString("-------\n")
	okCount, dupCount, missCount := 0, 0, 0
	sources := make(map[string]int)
	for _, e := range res.PathEntries {
		if e.IsDuplicate || e.SymlinkPointsTo >= 0 {
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
		sb.WriteString(fmt.Sprintf("├─ %-13s %2d (%3d%%)\n", "OK:", okCount, okCount*100/total))
		sb.WriteString(fmt.Sprintf("├─ %-13s %2d (%3d%%)\n", fmt.Sprintf("Missing %s:", model.IconMissing), missCount, missCount*100/total))
		sb.WriteString(fmt.Sprintf("└─ %-13s %2d (%3d%%)\n", fmt.Sprintf("Duplicates %s:", model.IconDuplicate), dupCount, dupCount*100/total))
	}

	sb.WriteString("\n")

	// Issues Section
	sb.WriteString("ISSUES FOUND\n")
	sb.WriteString("------------\n")
	foundAny := false

	// Duplicates
	if dupCount > 0 {
		foundAny = true
		sb.WriteString(fmt.Sprintf("%s DUPLICATES (%d) [NOT SERIOUS]\n", model.IconDuplicate, dupCount))
		for i, e := range res.PathEntries {
			if e.IsDuplicate {
				sb.WriteString(fmt.Sprintf("%2d. %s\n", i+1, e.Value))
				orig := res.PathEntries[e.DuplicateOf]

				// Show where this entry was added
				sb.WriteString(fmt.Sprintf("    » Added by line %d of %s\n", e.LineNumber, e.SourceFile))

				// Quote the actual source line
				sourceLine := getLineFromFile(e.SourceFile, e.LineNumber)
				if sourceLine != "" {
					// Truncate if too long
					if len(sourceLine) > 70 {
						sourceLine = sourceLine[:67] + "..."
					}
					sb.WriteString(fmt.Sprintf("      %s\n", sourceLine))
				}

				// Check if both entries come from the same source file and line
				if e.SourceFile == orig.SourceFile && e.LineNumber == orig.LineNumber {
					sb.WriteString(fmt.Sprintf("    » Duplicates PATH entry #%d which was already in $PATH\n\n", e.DuplicateOf+1))
				} else {
					sb.WriteString(fmt.Sprintf("    » Duplicates PATH entry #%d (from line %d of %s)\n", e.DuplicateOf+1, orig.LineNumber, orig.SourceFile))
					sb.WriteString(fmt.Sprintf("    » Advice: remove line %d from %s\n\n", e.LineNumber, e.SourceFile))
				}
			} else if e.SymlinkPointsTo >= 0 {
				sb.WriteString(fmt.Sprintf("%2d. %s\n", i+1, e.Value))
				sb.WriteString(fmt.Sprintf("    » Symlink resolves to PATH entry %d (%s)\n", e.SymlinkPointsTo+1, e.SymlinkTarget))
				sb.WriteString(fmt.Sprintf("    » This is normal on modern Linux systems\n\n"))
			}
		}
	}

	// Missing
	if missCount > 0 {
		foundAny = true
		sb.WriteString(fmt.Sprintf("%s MISSING DIRECTORIES (%d) [NOT SERIOUS]\n", model.IconMissing, missCount))
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

	sb.WriteString("CONFIGURATION FILES FLOW - SUMMARY\n")
	sb.WriteString("----------------------------------\n")
	for _, n := range res.FlowNodes {
		indent := strings.Repeat("  ", n.Depth)
		status := ""
		if n.NotExecuted {
			// Check if file exists
			expandedPath := expandTilde(n.FilePath)
			if _, err := os.Stat(expandedPath); os.IsNotExist(err) {
				status = " [Not Executed - file does not exist]"
			} else {
				status = " [Not Executed - file exists]"
			}
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
			execLabel = " (executed first " + model.IconFirst + ")"
		} else if n.Order == len(res.FlowNodes) {
			execLabel = " (executed last " + model.IconLast + ")"
		}

		sb.WriteString(fmt.Sprintf("%2d. %s%s%s%s%s\n", n.Order, indent, n.FilePath, desc, status, execLabel))
	}

	// Add detailed view showing actual paths added by each node (verbose mode only)
	if verbose {
		sb.WriteString("\nCONFIGURATION FILES FLOW - DETAIL\n")
		sb.WriteString("---------------------------------\n")
		for _, n := range res.FlowNodes {
			indent := strings.Repeat("  ", n.Depth)

			// Build the header line
			status := ""
			if n.NotExecuted {
				// Check if file exists
				expandedPath := expandTilde(n.FilePath)
				if _, err := os.Stat(expandedPath); os.IsNotExist(err) {
					status = " [Not Executed - file does not exist]"
				} else {
					status = " [Not Executed - file exists]"
				}
			} else if len(n.Entries) > 0 {
				status = fmt.Sprintf(" [%d paths]", len(n.Entries))
			} else {
				status = " [no change]"
			}
			desc := ""
			if n.Description != "" {
				desc = " " + n.Description
			}
			execLabel := ""
			if n.Order == 1 {
				execLabel = " (executed first " + model.IconFirst + ")"
			} else if n.Order == len(res.FlowNodes) {
				execLabel = " (executed last " + model.IconLast + ")"
			}

			sb.WriteString(fmt.Sprintf("%2d. %s%s%s%s%s\n", n.Order, indent, n.FilePath, desc, status, execLabel))

			// List the actual paths added by this node
			if len(n.Entries) > 0 && !n.NotExecuted {
				// Path entries should be indented more than the flow node itself
				// Base indent is 6 spaces (to align after "    ") plus the node's depth-based indent
				pathIndent := "      " + indent

				for _, entryIdx := range n.Entries {
					if entryIdx < len(res.PathEntries) {
						entry := res.PathEntries[entryIdx]
						marker := ""
						if entry.IsDuplicate {
							marker = " " + model.IconDuplicate
						} else if entry.IsSessionOnly {
							marker = " " + model.IconSession
						}
						sb.WriteString(fmt.Sprintf("%s» %s%s\n", pathIndent, entry.Value, marker))
					}
				}
			}
		}
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

var bashStandard = []standardConfig{
	{"/etc/profile", 1},
	{"/etc/bash.bashrc", 2},
	{"/etc/bashrc", 3},
	{"/.bash_profile", 4},
	{"/.bash_login", 5},
	{"/.profile", 6},
	{"/.bashrc", 7},
}

// detectShellFromNodes determines if the executed files are bash or zsh
func detectShellFromNodes(nodes []model.ConfigNode) string {
	bashCount := 0
	zshCount := 0

	for _, node := range nodes {
		if node.NotExecuted {
			continue
		}
		path := strings.ToLower(node.FilePath)
		if strings.Contains(path, "bash") {
			bashCount++
		}
		if strings.Contains(path, "zsh") {
			zshCount++
		}
	}

	// If we see bash files executed, it's bash
	if bashCount > 0 && zshCount == 0 {
		return "bash"
	}
	// If we see zsh files executed, it's zsh
	if zshCount > 0 && bashCount == 0 {
		return "zsh"
	}
	// Default to zsh if ambiguous or no specific shell files found
	return "zsh"
}

func injectMissingNodes(nodes []model.ConfigNode) []model.ConfigNode {
	// Detect which shell is being used based on executed files
	detectedShell := detectShellFromNodes(nodes)

	// Only inject missing nodes for the detected shell
	var standardConfigs []standardConfig
	if detectedShell == "bash" {
		standardConfigs = bashStandard
	} else {
		standardConfigs = zshStandard
	}

	var result []model.ConfigNode
	standardIdx := 0

	// Helper to get rank of a node (if it matches standard)
	getRank := func(path string) int {
		for _, s := range standardConfigs {
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
			for standardIdx < len(standardConfigs) {
				std := standardConfigs[standardIdx]
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
						displayPath = "~" + displayPath // ~/.zshrc or ~/.bashrc
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
	for standardIdx < len(standardConfigs) {
		std := standardConfigs[standardIdx]
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
	if path == "System (Default)" {
		return true
	}
	// Check standard zsh/bash files
	// Use Contains or Suffix to handle absolute paths
	keys := []string{
		"zshenv", ".zshenv",
		"zprofile", ".zprofile",
		"zshrc", ".zshrc",
		"zlogin", ".zlogin",
		"bash_profile", ".bash_profile",
		"bashrc", ".bashrc", "bash.bashrc",
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
	_, err := os.Stat(path)
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

	return fmt.Sprintf("%d files, %d dirs", fileCount, dirCount)
}
