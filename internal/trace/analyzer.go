package trace

import (
	"fmt"
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
					ID:       fmt.Sprintf("node-%d", nodeCounter),
					FilePath: ev.File,
					Order:    nodeCounter,
					Depth:    depth,
					Entries:  []int{},
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

	// Post-process for Duplicates
	seen := make(map[string]int) // value -> index
	for i, e := range entries {
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
	}

	// Post-process Flow Graph: Clean up noise
	// 1. Attribute entries to nodes (reverse mapping)
	// 2. Filter nodes with 0 entries (unless it's the very first node, maybe?)
	// 3. Merge adjacent nodes with same FilePath

	// First, populate Entries indices in FlowNodes
	nodeMap := make(map[string]*model.ConfigNode)
	for i := range flowNodes {
		nodeMap[flowNodes[i].ID] = &flowNodes[i]
	}
	for i, e := range entries {
		if node, ok := nodeMap[e.FlowID]; ok {
			node.Entries = append(node.Entries, i)
		}
	}

	// Filter and Merge
	var cleanNodes []model.ConfigNode
	for _, node := range flowNodes {
		// Filter empty nodes - UNLESS it's an important config file.
		// User wants to see .zshenv, .zprofile, etc even if they don't change PATH.
		// We still want to filters out "noisy" internal files if empty.

		isImportant := isImportantConfig(node.FilePath)

		if len(node.Entries) == 0 && !isImportant {
			continue
		}

		// Merge with previous if same file
		if len(cleanNodes) > 0 {
			last := &cleanNodes[len(cleanNodes)-1]
			if last.FilePath == node.FilePath {
				// Merge
				// Entries are just indices into main list, so appending is fine
				// (Assuming main list isn't reordered, which it isn't)
				last.Entries = append(last.Entries, node.Entries...)
				// We need to update the FlowID of the entries that pointed to this 'node'
				// to point to 'last' instead.
				// iterate through entries... expensive?
				// No, we know which entries: `node.Entries`.
				for _, entryIdx := range node.Entries {
					entries[entryIdx].FlowID = last.ID
				}
				continue
			}
		}

		// Append new
		cleanNodes = append(cleanNodes, node)
	}

	// Renumber
	for i := range cleanNodes {
		cleanNodes[i].Order = i + 1
	}

	// Inject Missing Standard Files (Ghost Nodes)
	// We want to educate the user about files that didn't run.
	cleanNodes = injectMissingNodes(cleanNodes)
	// Renumber again
	for i := range cleanNodes {
		cleanNodes[i].Order = i + 1
	}

	return model.AnalysisResult{
		PathEntries: entries,
		FlowNodes:   cleanNodes,
	}
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
