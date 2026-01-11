package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	selectedItemStyle = lipgloss.NewStyle().
				PaddingLeft(2).
				Foreground(lipgloss.Color("205")) // Pinkish

	unselectedItemStyle = lipgloss.NewStyle().
				PaddingLeft(4).
				Foreground(lipgloss.Color("240")) // Grey

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238"))

	detailStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("63"))

	adviceStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")) // Orange
)

func (m AppModel) View() string {
	if m.Loading {
		return "\n  Scanning PATH trace... please wait.\n"
	}
	if m.Err != nil {
		return fmt.Sprintf("\n  Error: %v\n", m.Err)
	}

	// Layout dimensions
	width := m.WindowSize.Width
	height := m.WindowSize.Height
	leftWidth := width / 2
	rightWidth := width - leftWidth - 4 // borders/padding

	// Styles
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
	dimmedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	highlightStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true) // For matching flow items

	// LEFT PANEL: PATH List
	// Always filters by FilteredIndices
	var leftView strings.Builder
	leftView.WriteString(titleStyle.Render("PATH Entries"))
	leftView.WriteString("\n\n")

	// Determine Highlighting Context
	var activeFlowID string
	var activeFlowOrder int
	if m.ShowFlow && len(m.TraceResult.FlowNodes) > 0 && m.FlowSelectedIdx < len(m.TraceResult.FlowNodes) {
		node := m.TraceResult.FlowNodes[m.FlowSelectedIdx]
		activeFlowID = node.ID
		activeFlowOrder = node.Order
	}

	// Create a map of FlowID -> Order for fast lookup if needed,
	// or just rely on FlowID for specific and something else for cumulative.
	// Since Entry doesn't have Order, we need to map Entry.FlowID -> Order.
	// Optimization: Build this map once or on Update?
	// For TUI, building on View (small N) is fine.
	flowOrderMap := make(map[string]int)
	if m.CumulativeFlow {
		for _, n := range m.TraceResult.FlowNodes {
			flowOrderMap[n.ID] = n.Order
		}
	}

	// Windowing Logic for Left Panel
	// To ensure the selected item is always visible.
	visibleItems := height - 4 - 2 // Account for title and padding
	if visibleItems < 0 {
		visibleItems = 0
	}
	startIdx := 0
	endIdx := len(m.FilteredIndices)

	// Adjust window based on selection
	if len(m.FilteredIndices) > visibleItems {
		if m.SelectedIdx >= visibleItems/2 {
			startIdx = m.SelectedIdx - (visibleItems / 2)
		}
		if startIdx < 0 {
			startIdx = 0
		}
		if startIdx+visibleItems > len(m.FilteredIndices) {
			startIdx = len(m.FilteredIndices) - visibleItems
		}
		endIdx = startIdx + visibleItems
	}

	for i := startIdx; i < endIdx; i++ {
		idx := m.FilteredIndices[i]
		entry := m.TraceResult.PathEntries[idx]

		line := fmt.Sprintf("%d. %s", idx+1, entry.Value)
		if entry.IsDuplicate {
			line += " (dup)"
		}
		// Truncate
		if len(line) > leftWidth-2 {
			line = line[:leftWidth-2] + "…"
		}

		// Styling logic
		var style lipgloss.Style
		isRowSelected := (i == m.SelectedIdx)

		if m.ShowFlow {
			// Highlighting Condition
			highlight := false

			if m.CumulativeFlow {
				// Cumulative: Highlight if Entry's Node Order <= Active Node Order
				if order, ok := flowOrderMap[entry.FlowID]; ok {
					if order <= activeFlowOrder {
						highlight = true
					}
				}
			} else {
				// Specific: Highlight only if Entry belong to THIS specific node split
				if entry.FlowID == activeFlowID {
					highlight = true
				}
			}

			if highlight {
				if isRowSelected {
					style = selectedStyle
				} else {
					style = highlightStyle
				}
			} else {
				// Dimmed
				if isRowSelected {
					style = selectedStyle // Selection always visible
				} else {
					style = dimmedStyle
				}
			}
		} else {
			// Normal Mode
			if isRowSelected {
				style = selectedStyle
			} else {
				style = normalStyle
			}
		}

		leftView.WriteString(style.Render(line))
		leftView.WriteString("\n")
	}

	left := lipgloss.NewStyle().
		Width(leftWidth).
		Height(height - 4).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("63")).
		Render(leftView.String())

	// RIGHT PANEL: Details OR Flow List
	var rightView strings.Builder

	if m.ShowFlow {
		// FLOW MODE
		rightView.WriteString(titleStyle.Render("Configuration Flow"))
		rightView.WriteString("\n\n")

		// Windowing for Right Panel
		visibleItems := height - 4
		startIdx := 0
		endIdx := len(m.TraceResult.FlowNodes)

		if len(m.TraceResult.FlowNodes) > visibleItems {
			if m.FlowSelectedIdx >= visibleItems/2 {
				startIdx = m.FlowSelectedIdx - (visibleItems / 2)
			}
			if startIdx < 0 {
				startIdx = 0
			}
			if startIdx+visibleItems > len(m.TraceResult.FlowNodes) {
				startIdx = len(m.TraceResult.FlowNodes) - visibleItems
			}
			endIdx = startIdx + visibleItems
		}

		// Pre-calculate seen counts to identify duplicates/continuations
		// We can't just check 'seen' in the render loop because of windowing (we might skip the first occurrence).
		// So we need a global map of "Order -> IsContinuation".
		isContinuation := make(map[int]bool)
		seenPath := make(map[string]bool)
		for _, n := range m.TraceResult.FlowNodes {
			if seenPath[n.FilePath] {
				isContinuation[n.Order] = true
			}
			seenPath[n.FilePath] = true
		}

		for i := startIdx; i < endIdx; i++ {
			node := m.TraceResult.FlowNodes[i]
			name := node.FilePath
			// Truncate home for readability
			if strings.HasPrefix(name, os.Getenv("HOME")) {
				name = "~" + strings.TrimPrefix(name, os.Getenv("HOME"))
			}

			// Indentation
			indent := strings.Repeat("  ", node.Depth)
			// prefix := ""
			// if node.Depth > 0 {
			//     prefix = "└─ "
			// }

			// Maybe just spaces?
			// "  .zshrc"
			// "    nvm.sh"
			// "  .zshrc"
			// The user wants to understand duplication.
			// If I see:
			// .zshrc
			//   nvm.sh
			// .zshrc
			// It visually implies return.

			// indent = strings.Repeat("  ", node.Depth) // This line is redundant as indent is already calculated above

			suffix := ""
			if isContinuation[node.Order] {
				// We can keep this or remove it. "cont" is still helpful text.
				suffix = " (cont.)"
			}
			// Actually, if indented back out, it's obvious.
			// But let's keep suffix for now as "extra" clarity.

			line := fmt.Sprintf("%d. %s%s%s", node.Order, indent, name, suffix)
			// Truncate width
			if len(line) > rightWidth-2 {
				line = line[:rightWidth-2] + "…"
			}

			if i == m.FlowSelectedIdx {
				rightView.WriteString(selectedStyle.Render(line))
			} else {
				rightView.WriteString(normalStyle.Render(line))
			}
			rightView.WriteString("\n")
		}
	} else {
		// NORMAL MODE: Details
		rightView.WriteString(titleStyle.Render("Details"))
		rightView.WriteString("\n")

		if len(m.FilteredIndices) > 0 && m.SelectedIdx < len(m.FilteredIndices) {
			idx := m.FilteredIndices[m.SelectedIdx]
			entry := m.TraceResult.PathEntries[idx]

			rightView.WriteString(fmt.Sprintf("\nDirectory:  %s", entry.Value))
			rightView.WriteString(fmt.Sprintf("\nSource:     %s", entry.SourceFile))
			rightView.WriteString(fmt.Sprintf("\nLine:       %d", entry.LineNumber))
			rightView.WriteString(fmt.Sprintf("\nMode:       %s", entry.Mode))

			if m.ShowDiagnostics {
				if entry.IsDuplicate {
					rightView.WriteString(adviceStyle.Render(fmt.Sprintf("\n\n⚠️ DUPLICATE detected!\n%s", entry.Remediation)))
				} else {
					rightView.WriteString("\n\n✅ No issues detected.")
				}
			} else {
				if entry.IsDuplicate {
					rightView.WriteString("\n\n(Duplicate detected. Press 'd' for details)")
				}
			}

			// Flow info
			rightView.WriteString(fmt.Sprintf("\n\nFlow Node: %s", entry.FlowID))
		} else {
			rightView.WriteString("\nNo entries found.")
		}
	}

	// Viewport for right panel content?
	// m.DetailsViewport.SetContent(rightView.String())
	// Actually, simple resize render is easier for now than managing viewport scrolling for both modes.
	// Just rendering string is safer unless content overflows.
	// Assuming content fits for now.

	right := lipgloss.NewStyle().
		Width(rightWidth).
		Height(height - 4).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("63")).
		Render(rightView.String())

	// Footer
	help := "Help: ↑/↓: Navigate • d: Diagnostics • f/F: Flow • w: Which • q: Quit"
	if m.ShowFlow {
		help = "Flow Mode: ↑/↓: Select Config File • f: Return to Path List • F: Toggle Cumulative • q: Quit"
	}

	footer := "\n\n" + help
	if m.InputMode {
		footer = fmt.Sprintf("\n\nSearch: %s", m.InputBuffer.View())
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right) + footer
}

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, InitTraceCmd())
}
