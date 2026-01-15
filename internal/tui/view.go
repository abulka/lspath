package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lspath/internal/model"
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

	pathHighlightStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("81")). // Sky Blue/Cyan
				Bold(true)
)

func (m AppModel) View() string {
	if m.Loading {
		return "\n  Scanning PATH trace... please wait.\n"
	}
	if m.Err != nil {
		return fmt.Sprintf("\n  Error: %v\n", m.Err)
	}

	// Layout dimensions
	// Layout dimensions
	// Subtracting 6 for horizontal margin (borders x2 + buffer)
	// Subtracting 8 for vertical margin (title, footer, borders + buffer)
	width := m.WindowSize.Width
	height := m.WindowSize.Height

	netWidth := width - 6
	if netWidth < 20 {
		netWidth = 20
	}

	leftWidth := netWidth / 2
	rightWidth := netWidth - leftWidth

	// Total box height (including borders)
	boxHeight := height - 6
	if boxHeight < 6 {
		boxHeight = 6
	}

	// Interior height (excluding borders)
	interiorHeight := boxHeight - 2
	if interiorHeight < 2 {
		interiorHeight = 2
	}

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
	var leftView strings.Builder
	leftView.WriteString(titleStyle.Render("PATH Entries"))
	leftView.WriteString("\n\n") // 2 newlines = 3 lines total (Title + blank + blank)

	// Determine Highlighting Context
	var activeFlowOrder int
	var activeFlowID string // For highlighting entries from the selected flow node
	if m.ShowFlow && len(m.TraceResult.FlowNodes) > 0 && m.FlowSelectedIdx < len(m.TraceResult.FlowNodes) {
		node := m.TraceResult.FlowNodes[m.FlowSelectedIdx]
		activeFlowOrder = node.Order
		activeFlowID = node.ID
	}

	// Create a map of FlowID -> Order for fast lookup if needed,
	// or just rely on FlowID for specific and something else for cumulative.
	// Optimization: Build this map once or on Update?
	// For TUI, building on View (small N) is fine.
	flowOrderMap := make(map[string]int)
	if m.CumulativeFlow {
		for _, n := range m.TraceResult.FlowNodes {
			flowOrderMap[n.ID] = n.Order
		}
	}

	// Windowing Logic for Left Panel
	// Header is 2 lines (Title + 1 blank line)
	visibleItems := interiorHeight - 2
	if visibleItems < 1 {
		visibleItems = 1
	}
	startIdx := 0
	endIdx := len(m.FilteredIndices)

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

		// Determine status icon
		statusIcon := model.IconOK
		if entry.IsSessionOnly {
			statusIcon = model.IconSession // Session-only entry (not from config files)
		} else if entry.IsDuplicate || entry.SymlinkPointsTo >= 0 {
			statusIcon = model.IconDuplicate
		} else if entry.IsSymlink {
			statusIcon = model.IconSymlink
		} else {
			// Check if missing by looking at diagnostics or just use OK
			for _, diag := range entry.Diagnostics {
				if strings.Contains(diag, "does not exist") {
					statusIcon = model.IconMissing
					break
				}
			}
		}

		line := fmt.Sprintf("%2d. %s %s", idx+1, statusIcon, entry.Value)
		if entry.IsSessionOnly {
			line += " (session)"
		} else if entry.IsDuplicate {
			line += " (duplicate)"
		} else if entry.SymlinkPointsTo >= 0 {
			line += " (duplicate, symlink)"
		} else if entry.IsSymlink {
			line += " (symlink)"
		}

		// Priority indicators
		if idx == 0 {
			line += " (highest priority " + model.IconPriorityHigh + ")"
		} else if idx == len(m.TraceResult.PathEntries)-1 {
			line += " (lowest priority " + model.IconPriorityLow + ")"
		}

		// Truncate
		if len(line) > leftWidth-2 {
			line = line[:leftWidth-5] + "..."
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
				// Specific: Highlight entries belonging to the selected flow node
				if entry.FlowID == activeFlowID {
					highlight = true
				}
			}

			if highlight {
				// Don't show selection in flow mode, just highlight
				style = highlightStyle
			} else {
				// Dimmed - no selection shown
				style = dimmedStyle
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

	lBorderColor := borderColor
	if !m.ShowFlow && !m.NormalRightFocus {
		lBorderColor = activeColor
	}

	left := lipgloss.NewStyle().
		Width(leftWidth).
		Height(interiorHeight).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lBorderColor).
		Render(strings.TrimSuffix(leftView.String(), "\n"))

	// RIGHT PANEL: Details OR Flow List
	var rightView strings.Builder

	if m.ShowFlow {
		// FLOW MODE
		// RHS interior space matches LHS interior space
		topH := interiorHeight / 2
		botH := interiorHeight - topH

		rightView.WriteString(titleStyle.Render("Configuration Flow"))
		rightView.WriteString("\n\n") // 2 lines overhead (Title + blank line)

		// Flow mode with unified trace + session data
		// Windowing for Flow List (Top Panel)
		// Total panel height is topH.
		// One line is taken by the bottom divider border.
		// Two lines are taken by the header (Title + blank).
		// So visible items = topH - 1 - 2 = topH - 3.
		visibleFlowItems := topH - 3
		if visibleFlowItems < 1 {
			visibleFlowItems = 1
		}

		startIdx := 0
		endIdx := len(m.TraceResult.FlowNodes)

		if len(m.TraceResult.FlowNodes) > visibleFlowItems {
			if m.FlowSelectedIdx >= visibleFlowItems/2 {
				startIdx = m.FlowSelectedIdx - (visibleFlowItems / 2)
			}
			if startIdx < 0 {
				startIdx = 0
			}
			if startIdx+visibleFlowItems > len(m.TraceResult.FlowNodes) {
				startIdx = len(m.TraceResult.FlowNodes) - visibleFlowItems
			}
			endIdx = startIdx + visibleFlowItems
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

			// Special display for session-only nodes
			isSessionNode := name == "Session (Manual/Runtime)"
			if isSessionNode {
				name = "Current Session"
			} else if strings.HasPrefix(name, os.Getenv("HOME")) {
				// Truncate home for readability
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

			if isSessionNode {
				// Session nodes show "session-only" instead of file status
				pStr := "path"
				if ownCount > 1 {
					pStr = "paths"
				}
				statusStr = fmt.Sprintf(" [%d %s]", ownCount, pStr)
			} else if node.NotExecuted {
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

			if i == 0 {
				line += " (executed first " + model.IconFirst + ")"
			} else if i == len(m.TraceResult.FlowNodes)-1 {
				line += " (executed last " + model.IconLast + ")"
			}

			// If NotExecuted, maybe dim it even more?
			styleToUse := normalStyle
			if node.NotExecuted {
				styleToUse = dimStyle
			}

			// Truncate width strictly
			if len(line) > rightWidth-2 {
				line = line[:rightWidth-5] + "..."
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
		flowListStr := rightView.String()
		rightView.Reset()

		// Split the list lines and ensure we only take what fits the window
		// visibleFlowItems was used for slicing the node list, so this should already be correct.
		// However, we trim trailing newlines to prevent Lipgloss expansion.
		flowListContent := strings.TrimSpace(flowListStr)

		// Render flow list with interior height topH-1 (subtracting 1 for bottom border line).
		flowListView := lipgloss.NewStyle().
			Width(rightWidth).
			Height(topH-1).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(borderColor).
			Render(strings.TrimSuffix(flowListContent, "\n"))

		if m.RightPanelFocus == FocusFlowList {
			flowListView = lipgloss.NewStyle().
				Width(rightWidth).
				Height(topH-1).
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(activeColor).
				Render(strings.TrimSuffix(flowListContent, "\n"))
		}

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

		// Content space = botH - 1 (header line)
		previewContentHeight := botH - 1
		if previewContentHeight < 1 {
			previewContentHeight = 1
		}

		lines := strings.Split(strings.TrimSuffix(m.PreviewContent, "\n"), "\n")
		startY := m.PreviewScrollY
		if startY >= len(lines) && len(lines) > 0 {
			startY = len(lines) - previewContentHeight
		}
		if startY < 0 {
			startY = 0
		}

		endY := startY + previewContentHeight
		if endY > len(lines) {
			endY = len(lines)
		}

		if len(lines) > 0 && startY < len(lines) && m.PreviewPath != "" {
			visibleLines := lines[startY:endY]

			// Pre-calculate line number width
			lnWidth := len(fmt.Sprintf("%d", len(lines)))
			if lnWidth < 2 {
				lnWidth = 2
			}

			for i, line := range visibleLines {
				lineNum := startY + i + 1
				lnPrefix := fmt.Sprintf(" %*d | ", lnWidth, lineNum)
				prefixLen := len(lnPrefix)

				contentWidth := rightWidth - prefixLen
				if contentWidth < 10 {
					contentWidth = 10
				}

				// Highlighting check
				trimmedLine := strings.TrimSpace(line)
				isHighlighted := false
				if !strings.HasPrefix(trimmedLine, "#") {
					// 1. Explicit PATH exports/assignments
					isHighlighted = strings.Contains(line, "export PATH") || strings.Contains(line, "PATH=")

					// 2. Sourcing commands (source, ., \.)
					if !isHighlighted {
						sourcingKeywords := []string{"source ", ". ", "\\. "}
						for _, k := range sourcingKeywords {
							if strings.HasPrefix(trimmedLine, k) ||
								strings.Contains(trimmedLine, "; "+k) ||
								strings.Contains(trimmedLine, "&& "+k) {
								isHighlighted = true
								break
							}
						}
					}

					// 3. Execution/Helper commands
					if !isHighlighted {
						isHighlighted = strings.Contains(line, "eval ") ||
							strings.Contains(line, "brew shellenv") ||
							(strings.Contains(line, "path_helper") && !strings.Contains(line, "if "))
					}
				}

				// Truncate
				displayLine := line
				if len(displayLine) > contentWidth {
					displayLine = displayLine[:contentWidth-3] + "..."
				}

				// Render
				previewBuilder.WriteString(dimStyle.Render(lnPrefix))
				if isHighlighted {
					previewBuilder.WriteString(pathHighlightStyle.Render(displayLine))
				} else {
					previewBuilder.WriteString(displayLine)
				}
				previewBuilder.WriteString("\n")
			}
		} else if len(lines) == 0 && m.PreviewPath != "" {
			previewBuilder.WriteString("(empty)\n")
		} else if m.PreviewPath == "" {
			previewBuilder.WriteString("(no file selected)\n")
		}

		previewView := lipgloss.NewStyle().
			Width(rightWidth).
			Height(botH).
			Render(strings.TrimSuffix(previewBuilder.String(), "\n"))

		finalRight := lipgloss.JoinVertical(lipgloss.Left, flowListView, previewView)
		rightView.WriteString(finalRight)

	} else {
		// NORMAL MODE: Details
		rightView.WriteString(titleStyle.Render("Details"))
		rightView.WriteString("\n")

		if len(m.FilteredIndices) > 0 && m.SelectedIdx < len(m.FilteredIndices) {
			idx := m.FilteredIndices[m.SelectedIdx]
			entry := m.TraceResult.PathEntries[idx]

			// Build directory line with optional hint
			dirLine := fmt.Sprintf("\nDirectory:  %s", entry.Value)
			if !m.ShowDiagnostics {
				if entry.IsSessionOnly {
					dirLine += "  (⚡ session-only)"
				} else if entry.IsDuplicate {
					dirLine += fmt.Sprintf("  (%s. Press 'd' for details)", entry.DuplicateMessage)
				} else if entry.SymlinkPointsTo >= 0 {
					dirLine += fmt.Sprintf("  (%s. Press 'd' for details)", entry.SymlinkMessage)
				} else if entry.IsSymlink {
					dirLine += fmt.Sprintf("  (symlink %s → %s)", model.IconSymlink, entry.SymlinkTarget)
				}
			}
			rightView.WriteString(dirLine)

			// Show source info - different for session-only entries
			if entry.IsSessionOnly {
				rightView.WriteString("\nCaused by:  Current Session (not from config files)")
				if entry.SessionNote != "" {
					rightView.WriteString(fmt.Sprintf("\nNote:       %s", entry.SessionNote))
				}
				rightView.WriteString("\n\n--- Session-Only Entry ---")
				rightView.WriteString("\nThis path exists in your current terminal but was not")
				rightView.WriteString("\nadded by any shell configuration file. Common causes:")
				rightView.WriteString("\n  • Virtual environment (.venv, conda)")
				rightView.WriteString("\n  • Manual: export PATH=\"...\"")
				rightView.WriteString("\n  • Added by a tool after shell startup")
			} else {
				causedBy := fmt.Sprintf("\nCaused by:  %s", entry.SourceFile)
				if entry.Mode != "Unknown" {
					causedBy += fmt.Sprintf(" (Startup Phase: %s)", entry.Mode)
				}
				rightView.WriteString(causedBy)
				if entry.SourceFile == "System (Default)" && entry.LineNumber == 0 {
					rightView.WriteString("\nLine:       N/A")
				} else {
					rightView.WriteString(fmt.Sprintf("\nLine:       %d", entry.LineNumber))
				}

				// Show the actual line from the config file with context
				lineContext := model.GetLineContext(entry.SourceFile, entry.LineNumber)
				if lineContext.ErrorMsg == "" && (entry.LineNumber > 0 || entry.SourceFile != "System (Default)") {
					rightView.WriteString(fmt.Sprintf("\n\n--- Source Line Context (%s) ---", entry.SourceFile))
					if lineContext.HasBefore2 {
						rightView.WriteString(fmt.Sprintf("\n  %4d  %s", lineContext.LineNumber-2, lineContext.Before2))
					}
					if lineContext.HasBefore1 {
						rightView.WriteString(fmt.Sprintf("\n  %4d  %s", lineContext.LineNumber-1, lineContext.Before1))
					}
					rightView.WriteString(fmt.Sprintf("\n» %4d  %s", lineContext.LineNumber, lineContext.Target))
					if lineContext.HasAfter1 {
						rightView.WriteString(fmt.Sprintf("\n  %4d  %s", lineContext.LineNumber+1, lineContext.After1))
					}
					if lineContext.HasAfter2 {
						rightView.WriteString(fmt.Sprintf("\n  %4d  %s", lineContext.LineNumber+2, lineContext.After2))
					}
				}
			}

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
					rightView.WriteString(adviceStyle.Render(fmt.Sprintf("\n\n⚠️ DUPLICATE %s detected!\n%s", model.IconDuplicate, entry.DuplicateMessage)))
				} else if entry.SymlinkPointsTo >= 0 {
					rightView.WriteString(adviceStyle.Render(fmt.Sprintf("\n\n🔗 SYMLINK %s%s detected\n%s\n\nThis is normal on modern Linux systems.", model.IconDuplicate, model.IconSymlink, entry.SymlinkMessage)))
				} else {
					rightView.WriteString("\n\n" + model.IconOK + " No issues detected.")
				}
			}

			// Stats
			rightView.WriteString(fmt.Sprintf("\n\nPath Directory Stats:   %d files, %d directories", m.FileCount, m.DirCount))

			// Directory Listing
			if m.DirectoryListing != "" {
				rightView.WriteString("\n\n--- Directory Listing ---")
				rightView.WriteString("\n" + m.DirectoryListing)
			}

		} else {
			rightView.WriteString("\nNo entries found.")
		}
	}

	rBorderColor := borderColor
	if !m.ShowFlow && m.NormalRightFocus {
		rBorderColor = activeColor
	}

	// Line slicing for NORMAL details mode
	finalRightViewContent := rightView.String()
	if !m.ShowFlow {
		lines := strings.Split(strings.TrimSuffix(finalRightViewContent, "\n"), "\n")
		// Content height = interiorHeight
		startY := m.DetailsScrollY
		if startY >= len(lines) && len(lines) > 0 {
			startY = len(lines) - interiorHeight
		}
		if startY < 0 {
			startY = 0
		}
		endY := startY + interiorHeight
		if endY > len(lines) {
			endY = len(lines)
		}

		if len(lines) > interiorHeight {
			// Add scroll indicator if focused or not?
			// Let's just slice for now.
		}

		visibleLines := lines[startY:endY]
		var sb strings.Builder
		for i, line := range visibleLines {
			if len(line) > rightWidth {
				line = line[:rightWidth-4] + "..."
			}
			sb.WriteString(line)
			if i < len(visibleLines)-1 {
				sb.WriteString("\n")
			}
		}
		finalRightViewContent = sb.String()
	}

	right := lipgloss.NewStyle().
		Width(rightWidth).
		Height(interiorHeight).
		Border(lipgloss.NormalBorder()).
		BorderForeground(rBorderColor).
		Render(finalRightViewContent)

	// Footer
	help := "Help: ↑/↓: Navigate • Tab: Switch Panel • d: Diagnostics • f/c: Flow • w: Which • ?: Help • q: Quit"
	if m.NormalRightFocus && !m.ShowFlow {
		help = "Details Mode: ↑/↓: Scroll • Tab: Return to Path List • ?: Help • q: Quit"
	} else if m.ShowFlow {
		help = "Flow Mode: ↑/↓: Select Config File • Tab: Switch Focus • f: Return to Path List • c: Toggle Cumulative • ?: Help • q: Quit"
	}

	footer := "\n\n" + help
	if m.InputMode {
		footer = fmt.Sprintf("\n\nSearch: %s", m.InputBuffer.View())
	}

	mainView := lipgloss.JoinHorizontal(lipgloss.Top, left, right) + footer
	if m.ShowHelp {
		return m.renderHelpDialog()
	}
	if m.ShowDiagnosticsPopup {
		return m.renderDiagnosticsPopup()
	}
	return mainView
}

func (m *AppModel) renderDiagnosticsPopup() string {
	w, h := m.WindowSize.Width, m.WindowSize.Height
	if w < 20 || h < 10 {
		return "Window too small"
	}

	popupWidth := w * 90 / 100
	if popupWidth < 40 {
		popupWidth = 40
	}
	if popupWidth > w-4 {
		popupWidth = w - 4
	}
	popupHeight := h - 6
	if popupHeight < 5 {
		popupHeight = 5
	}

	lines := strings.Split(m.DiagnosticsReport, "\n")
	contentHeight := popupHeight - 4 // minus border and footer

	startY := m.DiagnosticsScrollY
	if startY > len(lines)-contentHeight {
		startY = len(lines) - contentHeight
	}
	if startY < 0 {
		startY = 0
	}
	m.DiagnosticsScrollY = startY

	endY := startY + contentHeight
	if endY > len(lines) {
		endY = len(lines)
	}

	visibleLines := lines[startY:endY]
	content := strings.Join(visibleLines, "\n")

	title := titleStyle.Render("Global Diagnostics Report")
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(
		"\nPress 's' to save, 'v' for verbose, 'd'/Esc to close")

	dialog := lipgloss.NewStyle().
		Width(popupWidth).
		Height(popupHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("208")). // Orange
		Padding(0, 1).
		Render(title + "\n\n" + content + footer)

	return lipgloss.Place(w, h,
		lipgloss.Center, lipgloss.Center,
		dialog,
	)
}

func (m *AppModel) renderHelpDialog() string {
	w, h := m.WindowSize.Width, m.WindowSize.Height
	if w < 20 || h < 10 {
		return "Window too small"
	}

	helpWidth := w * 80 / 100
	if helpWidth < 40 {
		helpWidth = 40
	}
	if helpWidth > w-4 {
		helpWidth = w - 4
	}
	helpHeight := h - 6
	if helpHeight < 5 {
		helpHeight = 5
	}

	lines := strings.Split(m.HelpContent, "\n")
	// Adjust height for title and border
	contentHeight := helpHeight - 2

	startY := m.HelpScrollY
	if startY > len(lines)-contentHeight {
		startY = len(lines) - contentHeight
	}
	if startY < 0 {
		startY = 0
	}
	m.HelpScrollY = startY // Correct it back

	endY := startY + contentHeight
	if endY > len(lines) {
		endY = len(lines)
	}

	visibleLines := lines[startY:endY]
	content := strings.Join(visibleLines, "\n")

	dialog := lipgloss.NewStyle().
		Width(helpWidth).
		Height(helpHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(0, 1).
		Render(content)

	return lipgloss.Place(w, h,
		lipgloss.Center, lipgloss.Center,
		dialog,
	)
}

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, InitTraceCmd())
}
