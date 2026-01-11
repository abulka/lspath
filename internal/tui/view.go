package tui

import (
	"fmt"
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
		return "\n  Scanning shell configuration... (running zsh -xli)\n"
	}
	if m.Err != nil {
		return fmt.Sprintf("\n  Error: %v\n", m.Err)
	}

	// Calculate Panel Widths
	width := m.WindowSize.Width
	height := m.WindowSize.Height
	if width == 0 {
		width = 100
	} // fallback

	halfWidth := width/2 - 2

	// LEFT PANEL: PATH List
	var leftView strings.Builder
	leftView.WriteString(titleStyle.Render("PATH Entries"))
	leftView.WriteString("\n\n")

	for i, idx := range m.FilteredIndices {
		// Only render visible items if we had a proper viewport, but for now render all
		// and let the term handle scrolling or limit to N.
		// Actually rendering 100 lines might flicker without viewport.
		// Let's implement primitive "windowing" based on SelectedIdx.
		// Center around selected.

		// Simple Viewport logic:
		viewportHeight := height - 4
		start := 0
		if m.SelectedIdx > viewportHeight/2 {
			start = m.SelectedIdx - viewportHeight/2
		}
		if i < start || i > start+viewportHeight {
			continue
		}

		entry := m.TraceResult.PathEntries[idx]
		cursor := " "
		style := unselectedItemStyle
		if i == m.SelectedIdx {
			cursor = ">"
			style = selectedItemStyle
		}

		// If search active and matched, maybe highlight?
		// Currently FilteredIndices ONLY contains matches, so they are all matches.

		leftView.WriteString(style.Render(fmt.Sprintf("%s %s", cursor, entry.Value)))
		leftView.WriteString("\n")
	}

	// RIGHT PANEL: Details
	var rightView strings.Builder

	// Header
	rightView.WriteString(titleStyle.Render("Details"))
	rightView.WriteString("\n")

	if len(m.FilteredIndices) > 0 {
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

	// Footer
	footer := "\n\nHelp: ↑/↓: Navigate • d: Diagnostics • w: Which • q: Quit"
	if m.InputMode {
		footer = fmt.Sprintf("\n\nSearch: %s", m.InputBuffer.View())
	}

	// Combine
	left := lipgloss.NewStyle().Width(halfWidth).Render(leftView.String())
	right := detailStyle.Width(halfWidth).Height(height - 4).Render(rightView.String())

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right) + footer
}

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, InitTraceCmd())
}
