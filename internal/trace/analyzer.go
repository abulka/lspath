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
	var entries []model.PathEntry
	var flowNodes []model.ConfigNode
	var lastFile string
	var currentNode *model.ConfigNode
	var lastPathStr string

	// Helper to find if a path segment exists in current entries
	findEntry := func(val string, currentEntries []model.PathEntry) *model.PathEntry {
		for _, e := range currentEntries {
			if e.Value == val {
				return &e
			}
		}
		return nil
	}

	nodeCounter := 0

	for _, ev := range events {
		// Flow Graph Construction
		if ev.File != lastFile {
			// New file context
			// Check if we are returning to a previous stack?
			// For now, assuming linear flow or nested.
			// We'll simplisticly create a new node for every file switch
			// to show the trace timeline.

			nodeCounter++
			node := model.ConfigNode{
				ID:       fmt.Sprintf("node-%d", nodeCounter),
				FilePath: ev.File,
				Order:    nodeCounter,
				Entries:  []int{},
			}
			flowNodes = append(flowNodes, node)
			currentNode = &flowNodes[len(flowNodes)-1]
			lastFile = ev.File
		}

		if ev.PathChange != "" && ev.PathChange != lastPathStr {
			// PATH changed. Re-calculate entries.
			newPaths := strings.Split(ev.PathChange, ":")
			var newEntries []model.PathEntry

			// Clean paths (remove empty strings commonly at end of split if trailing :)
			var cleanedPaths []string
			for _, p := range newPaths {
				if p != "" {
					cleanedPaths = append(cleanedPaths, p)
				}
			}
			newPaths = cleanedPaths

			for _, pVal := range newPaths {
				// Is this pVal in our existing entries?
				existing := findEntry(pVal, entries)
				if existing != nil {
					// Persist existing attribution
					// Note: If the user explicitly re-adds it, does the attribution change?
					// Typically "export PATH=$PATH:/foo" keeps existing attributes.
					// If they "export PATH=/foo" (reset), then everything loses attribution except /foo.
					// But usually we build up.
					// If key is present, we keep usage of the *first* time it was seen?
					// Or do we strictly respect the current placement?
					// Let's keep the *original* attribution to show where it came from first.
					newEntries = append(newEntries, *existing)
				} else {
					// It's new! Attribute to this event.
					entry := model.PathEntry{
						Value:      pVal,
						SourceFile: ev.File,
						LineNumber: ev.Line,
						FlowID:     currentNode.ID,
						Mode:       GuessShellMode(ev.File),
					}
					newEntries = append(newEntries, entry)
					// Add to current node's contribution list
					// (We can't add index yet because index changes, but we track count)
					// Actually, we can just filter by FlowID later.
				}
			}

			entries = newEntries
			lastPathStr = ev.PathChange
		}
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

	return model.AnalysisResult{
		PathEntries: entries,
		FlowNodes:   flowNodes,
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
