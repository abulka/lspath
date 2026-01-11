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

	// Better: Maintain current list of `[]*model.PathEntry`.
	var currentEntries []*model.PathEntry

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
				// Stack Logic: Use the explicit depth form trace (truth)
				// Adjust depth: The user sees + as 1, ++ as 2.
				// We want 0-based indentation for Top Level.
				// Assuming standard zsh -x, the first level is usually 1 (+)
				// But we might be in a subshell or sourced file.

				depth := ev.Depth - 1
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
		// Filter empty nodes
		if len(node.Entries) == 0 {
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

	return model.AnalysisResult{
		PathEntries: entries,
		FlowNodes:   cleanNodes,
	}
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
