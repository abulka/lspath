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

	// Colors for split view
	dimColor := lipgloss.Color("240")
	activeColor := lipgloss.Color("205")
	borderColor := lipgloss.Color("63")

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
			// It visually implies return.

			// indent = strings.Repeat("  ", node.Depth) // This line is redundant as indent is already calculated
			// Annotations (User Requested Educational Descriptions)
			note := ""
			// Normalize for check
			checkPath := node.FilePath
			if strings.HasPrefix(checkPath, "~") {
				// Expand for check if needed, or just check suffix
			}

			if strings.HasSuffix(checkPath, "/etc/zshenv") {
				note = " (system-wide env)"
			}
			if strings.HasSuffix(checkPath, "/.zshenv") || checkPath == "~/.zshenv" {
				note = " (your personal env file)"
			}

			if strings.HasSuffix(checkPath, "/etc/zprofile") {
				note = " (system-wide)"
			}
			if strings.HasSuffix(checkPath, "/.zprofile") || checkPath == "~/.zprofile" {
				note = " (your personal profile)"
			}

			if strings.HasSuffix(checkPath, "/etc/zshrc") {
				note = " (system-wide)"
			}
			if strings.HasSuffix(checkPath, "/.zshrc") || checkPath == "~/.zshrc" {
				note = " (your personal rc file)"
			}

			if strings.HasSuffix(checkPath, "/etc/zlogin") {
				note = " (system-wide)"
			}
			if strings.HasSuffix(checkPath, "/.zlogin") || checkPath == "~/.zlogin" {
				note = " (your personal login file)"
			}

			if strings.HasSuffix(checkPath, "/etc/zshrc_Apple_Terminal") {
				note = " (Apple Terminal)"
			}
			if strings.Contains(checkPath, "cargo/env") {
				note = " (Rust Cargo)"
			}
			if strings.Contains(checkPath, "nvm.sh") {
				note = " (Node Version Manager)"
			}

			contStr := ""
			if isContinuation[node.Order] {
				contStr = " (cont.)"
			}

			// Status Indicator [..]
			statusStr := ""

			// Calculate nested paths
			// Look ahead for children (depth > node.Depth)
			nestedCount := 0
			for j := i + 1; j < len(m.TraceResult.FlowNodes); j++ {
				sub := m.TraceResult.FlowNodes[j]
				if sub.Depth <= node.Depth {
					break // End of children
				}
				nestedCount += len(sub.Entries)
			}

			ownCount := len(node.Entries)
			totalCount := ownCount + nestedCount

			if node.NotExecuted {
				statusStr = " [Not Executed]"
			} else if totalCount == 0 {
				statusStr = " [No Change]"
			} else {
				if ownCount > 0 && nestedCount > 0 {
					// [1 path (+2 nested)] or [2 paths (+2 nested)]
					pStr := "path"
					if ownCount > 1 {
						pStr = "paths"
					}
					statusStr = fmt.Sprintf(" [%d %s (+%d nested)]", ownCount, pStr, nestedCount)
				} else if ownCount == 0 && nestedCount > 0 {
					// [4 nested paths]
					statusStr = fmt.Sprintf(" [%d nested paths]", nestedCount)
				} else {
					// [1 path]
					pStr := "path"
					if ownCount > 1 {
						pStr = "paths"
					}
					statusStr = fmt.Sprintf(" [%d %s]", ownCount, pStr)
				}
			}

			// Combine: Order. Indent Name (cont) (Description) [Status]
			line := fmt.Sprintf("%d. %s%s%s%s%s", node.Order, indent, name, contStr, note, statusStr)

			// If NotExecuted, maybe dim it even more?
			styleToUse := normalStyle
			if node.NotExecuted {
				styleToUse = dimStyle
			}

			// Truncate width
			if len(line) > rightWidth-2 {
				line = line[:rightWidth-2] + "…"
			}

			if i == m.FlowSelectedIdx {
				// Highlight row
				rendered := selectedStyle.Render(line)
				// If focused on List, add extra indicator
				if m.RightPanelFocus == FocusFlowList {
					rendered = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Render(line)
				}
				rightView.WriteString(rendered)
			} else {
				rightView.WriteString(styleToUse.Render(line))
			}
			rightView.WriteString("\n")
		}

		// --- PREVIEW PANEL (BOTTOM) ---
		// We need to split the view properly

		// Split Dimensions
		totalH := height - 4
		topH := totalH / 2
		botH := totalH - topH - 1 // -1 for separator

		// Truncate flow list string to fit top half
		flowListStr := rightView.String()
		flowListLines := strings.Split(flowListStr, "\n")

		// Keep only the lines that fit in topH (accounting for title and borders)
		maxFlowLines := topH - 3 // -3 for title, padding, border
		if maxFlowLines < 1 {
			maxFlowLines = 1
		}

		if len(flowListLines) > maxFlowLines {
			flowListLines = flowListLines[:maxFlowLines]
		}

		flowListStr = strings.Join(flowListLines, "\n")
		rightView.Reset()

		// Render flow list with border
		borderStyle := lipgloss.NewStyle().
			Width(rightWidth).
			Border(lipgloss.NormalBorder(), false, false, true, false). // Bottom border only
			BorderForeground(borderColor)

		if m.RightPanelFocus == FocusFlowList {
			borderStyle = borderStyle.BorderForeground(activeColor)
		}

		flowListView := borderStyle.Render(flowListStr)

		// Preview View
		var previewBuilder strings.Builder

		previewHeader := " File Preview "
		headerStyle := lipgloss.NewStyle().Foreground(dimColor).Bold(true)

		if m.RightPanelFocus == FocusFilePreview {
			previewHeader = " File Preview (Active) "
			headerStyle = lipgloss.NewStyle().Foreground(activeColor).Bold(true)
		}

		previewBuilder.WriteString(headerStyle.Render(previewHeader))
		previewBuilder.WriteString("\n")

		// Content Slicing
		lines := strings.Split(m.PreviewContent, "\n")
		startY := m.PreviewScrollY
		if startY >= len(lines) && len(lines) > 0 {
			startY = len(lines) - 1
		}
		if startY < 0 {
			startY = 0
		}

		// Available space for preview content
		previewContentHeight := botH - 1 // -1 for header
		if previewContentHeight < 1 {
			previewContentHeight = 1
		}

		endY := startY + previewContentHeight
		if endY > len(lines) {
			endY = len(lines)
		}

		if startY < len(lines) {
			visibleLines := lines[startY:endY]
			for _, line := range visibleLines {
				// Truncate long lines
				if len(line) > rightWidth {
					line = line[:rightWidth]
				}
				previewBuilder.WriteString(line)
				previewBuilder.WriteString("\n")
			}
		} else {
			previewBuilder.WriteString("(empty)\n")
		}

		// Join Top and Bottom
		finalRight := lipgloss.JoinVertical(lipgloss.Left, flowListView, previewBuilder.String())
		rightView.WriteString(finalRight)

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

			// Search Match Details
			if m.SearchActive {
				if filename, ok := m.SearchMatches[idx]; ok {
					// Get File Info
					fullPath := fmt.Sprintf("%s/%s", entry.Value, filename) // Simple join
					// os.Join is better but this works for unix

					info, err := os.Lstat(fullPath)
					if err == nil {
						rightView.WriteString("\n\n--- Found Binary ---")
						rightView.WriteString(fmt.Sprintf("\nName:       %s", filename))
						rightView.WriteString(fmt.Sprintf("\nPath:       %s", fullPath))
						rightView.WriteString(fmt.Sprintf("\nSize:       %d bytes", info.Size()))
						rightView.WriteString(fmt.Sprintf("\nMode:       %s", info.Mode()))
						rightView.WriteString(fmt.Sprintf("\nModified:   %s", info.ModTime().Format("2006-01-02 15:04:05")))

						// Check for Symlink
						if info.Mode()&os.ModeSymlink != 0 {
							target, err := os.Readlink(fullPath)
							if err == nil {
								rightView.WriteString(fmt.Sprintf("\n\n🔗 Symlink -> %s", target))
								// Maybe Stat the target too?
								if tInfo, err := os.Stat(fullPath); err == nil {
									rightView.WriteString(fmt.Sprintf("\nTarget Mode: %s", tInfo.Mode()))
								} else {
									rightView.WriteString(" (Broken Link)")
								}
							}
						}

						// Check Executable
						// Check bit 0100 (User Exec), 0010 (Group), 0001 (Other)
						perm := info.Mode().Perm()
						isExec := (perm&0100) != 0 || (perm&0010) != 0 || (perm&0001) != 0
						if isExec {
							rightView.WriteString("\n✅ Executable")
						} else {
							rightView.WriteString("\n❌ Not Executable")
						}
					}
				}
			}

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
