package tui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"lspath/internal/model"
	"lspath/internal/trace"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// expandTilde expands ~ to the user's home directory
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

// MsgTraceReady indicates that the trace has completed.
type MsgTraceReady model.AnalysisResult

// MsgError indicates an error occurred.
type MsgError error

// Update handles events.
func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.WindowSize = msg
		m.DetailsViewport.Width = msg.Width / 2
		m.DetailsViewport.Height = msg.Height - 4 // minus footer/header
		return m, nil

	case MsgTraceReady:
		m.Loading = false
		m.TraceResult = model.AnalysisResult(msg)
		// Generate global report
		m.DiagnosticsReport = trace.GenerateReport(m.TraceResult, m.DiagnosticsVerbose)

		// Auto-populate filtered indices with all
		m.FilteredIndices = make([]int, len(m.TraceResult.PathEntries))
		for i := range m.TraceResult.PathEntries {
			m.FilteredIndices[i] = i
		}
		if len(m.FilteredIndices) > 0 {
			m.SelectedIdx = 0
			m.loadDirectoryListing()
		}
		return m, nil

	case MsgError:
		m.Err = msg
		m.Loading = false
		return m, nil

	case tea.KeyMsg:
		if m.InputMode {
			switch msg.Type {
			case tea.KeyEnter:
				// Search finished (handled in View via highlighting)
				// Just exit input mode? Or keep it?
				// For now, exit input mode but keep search active state.
				m.InputMode = false
				m.performSearch()
				return m, nil
			case tea.KeyEsc:
				// Exit search mode and clear search
				m.InputMode = false
				m.InputBuffer.Blur()
				m.SearchActive = false // Disable search
				m.InputBuffer.SetValue("")
				m.performSearch() // Reset filter to all
				return m, nil
			}
			m.InputBuffer, cmd = m.InputBuffer.Update(msg)
			return m, cmd
		}

		if m.ShowHelp {
			switch msg.String() {
			case "?", "h", "esc", "q":
				m.ShowHelp = false
				return m, nil
			case "up", "k":
				if m.HelpScrollY > 0 {
					m.HelpScrollY--
				}
			case "down", "j":
				m.HelpScrollY++
			case "pgdown", "ctrl+d", "ctrl+f", " ":
				m.HelpScrollY += 10
			case "pgup", "ctrl+u", "ctrl+b", "b":
				if m.HelpScrollY > 10 {
					m.HelpScrollY -= 10
				} else {
					m.HelpScrollY = 0
				}
			case "home", "g":
				m.HelpScrollY = 0
			case "end", "G":
				m.HelpScrollY = 1000 // Just a high number, we cap below
			}

			// Robust Capping for Help
			helpLines := strings.Split(m.HelpContent, "\n")
			maxHelpScroll := len(helpLines) - (m.WindowSize.Height - 8)
			if maxHelpScroll < 0 {
				maxHelpScroll = 0
			}
			if m.HelpScrollY > maxHelpScroll {
				m.HelpScrollY = maxHelpScroll
			}
			if m.HelpScrollY < 0 {
				m.HelpScrollY = 0
			}

			return m, nil
		}

		if m.ShowDiagnosticsPopup {
			switch msg.String() {
			case "d", "esc", "q":
				m.ShowDiagnosticsPopup = false
				return m, nil
			case "up", "k":
				if m.DiagnosticsScrollY > 0 {
					m.DiagnosticsScrollY--
				}
			case "down", "j":
				m.DiagnosticsScrollY++
			case "pgup", "ctrl+u", "ctrl+b", "b":
				if m.DiagnosticsScrollY > 10 {
					m.DiagnosticsScrollY -= 10
				} else {
					m.DiagnosticsScrollY = 0
				}
			case "pgdown", "ctrl+d", "ctrl+f", " ":
				m.DiagnosticsScrollY += 10
			case "home", "g":
				m.DiagnosticsScrollY = 0
			case "end", "G":
				m.DiagnosticsScrollY = 1000 // High number, capped below
			case "v":
				m.DiagnosticsVerbose = !m.DiagnosticsVerbose
				m.DiagnosticsReport = trace.GenerateReport(m.TraceResult, m.DiagnosticsVerbose)
			case "s":
				timestamp := time.Now().Format("2006-01-02-15-04-05")
				filename := fmt.Sprintf("lspath-report-%s.txt", timestamp)
				_ = os.WriteFile(filename, []byte(m.DiagnosticsReport), 0644)
			}

			// Robust Capping for Diagnostics
			diagLines := strings.Split(m.DiagnosticsReport, "\n")
			maxDiagScroll := len(diagLines) - (m.WindowSize.Height - 10) // 10 matches view.go popupHeight - 4
			if maxDiagScroll < 0 {
				maxDiagScroll = 0
			}
			if m.DiagnosticsScrollY > maxDiagScroll {
				m.DiagnosticsScrollY = maxDiagScroll
			}
			if m.DiagnosticsScrollY < 0 {
				m.DiagnosticsScrollY = 0
			}

			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case " ":
			// Spacebar global page down logic
			if m.ShowFlow {
				if m.RightPanelFocus == FocusFilePreview {
					m.PreviewScrollY += 10
				} else {
					m.FlowSelectedIdx += 10
					if m.FlowSelectedIdx >= len(m.TraceResult.FlowNodes) {
						m.FlowSelectedIdx = len(m.TraceResult.FlowNodes) - 1
					}
				}
			} else if m.NormalRightFocus {
				m.DetailsScrollY += 10
			} else {
				// PATH list paging
				m.SelectedIdx += 10
				if m.SelectedIdx >= len(m.FilteredIndices) {
					m.SelectedIdx = len(m.FilteredIndices) - 1
				}
				m.loadDirectoryListing()
			}
			return m, nil
		case "?", "h":
			m.ShowHelp = true
			m.HelpScrollY = 0
			return m, nil
		case "esc":
			// Global ESC handler
			if m.SearchActive {
				m.InputMode = false
				m.InputBuffer.Blur()
				m.SearchActive = false
				m.InputBuffer.SetValue("")
				m.performSearch()
				return m, nil
			}
			if m.ShowFlow {
				m.ShowFlow = false
				m.CumulativeFlow = false
				return m, nil
			}
		case "up", "k":
			if m.ShowFlow {
				if m.RightPanelFocus == FocusFilePreview {
					// Scroll preview up
					if m.PreviewScrollY > 0 {
						m.PreviewScrollY--
					}
				} else {
					// Navigate flow list
					if m.FlowSelectedIdx > 0 {
						m.FlowSelectedIdx--
						m.loadSelectedFile()
					}
				}
			} else {
				if m.NormalRightFocus {
					if m.DetailsScrollY > 0 {
						m.DetailsScrollY--
					}
				} else {
					if m.SelectedIdx > 0 {
						m.SelectedIdx--
						m.loadDirectoryListing()
					}
				}
			}
		case "down", "j":
			if m.ShowFlow {
				if m.RightPanelFocus == FocusFilePreview {
					// Scroll preview down
					m.PreviewScrollY++
				} else {
					// Navigate flow list
					if m.FlowSelectedIdx < len(m.TraceResult.FlowNodes)-1 {
						m.FlowSelectedIdx++
						m.loadSelectedFile()
					}
				}
			} else {
				if m.NormalRightFocus {
					m.DetailsScrollY++
				} else {
					if m.SelectedIdx < len(m.FilteredIndices)-1 {
						m.SelectedIdx++
						m.loadDirectoryListing()
					}
				}
			}
		case "pgup", "ctrl+u", "ctrl+b", "b":
			// Page up
			if m.ShowFlow && m.RightPanelFocus == FocusFilePreview {
				if m.PreviewScrollY > 10 {
					m.PreviewScrollY -= 10
				} else {
					m.PreviewScrollY = 0
				}
			} else if !m.ShowFlow && m.NormalRightFocus {
				if m.DetailsScrollY > 10 {
					m.DetailsScrollY -= 10
				} else {
					m.DetailsScrollY = 0
				}
			} else if !m.ShowFlow && !m.NormalRightFocus {
				// Page up LHS PATH list
				m.SelectedIdx -= 10
				if m.SelectedIdx < 0 {
					m.SelectedIdx = 0
				}
				m.loadDirectoryListing()
			}
		case "pgdown", "ctrl+d", "ctrl+f":
			// Page down
			if m.ShowFlow && m.RightPanelFocus == FocusFilePreview {
				m.PreviewScrollY += 10
			} else if !m.ShowFlow && m.NormalRightFocus {
				m.DetailsScrollY += 10
			} else if !m.ShowFlow && !m.NormalRightFocus {
				// Page down LHS PATH list
				m.SelectedIdx += 10
				if m.SelectedIdx >= len(m.FilteredIndices) {
					m.SelectedIdx = len(m.FilteredIndices) - 1
				}
				m.loadDirectoryListing()
			}
		case "home", "g":
			// Jump to top of preview
			if m.ShowFlow && m.RightPanelFocus == FocusFilePreview {
				m.PreviewScrollY = 0
			} else if !m.ShowFlow && m.NormalRightFocus {
				m.DetailsScrollY = 0
			}
		case "end", "G":
			// Jump to end of preview - show last page
			if m.ShowFlow && m.RightPanelFocus == FocusFilePreview {
				// Calculate the actual number of lines
				lines := strings.Split(m.PreviewContent, "\n")
				if len(lines) > 0 {
					// Calculate visible height (matches view.go logic)
					contentHeight := m.WindowSize.Height - 10
					topH := contentHeight / 2
					botH := contentHeight - topH
					visibleHeight := botH - 1 // -1 for header

					if visibleHeight < 1 {
						visibleHeight = 1
					}

					// Set scroll to show last page
					lastLinePos := len(lines) - visibleHeight
					if lastLinePos < 0 {
						lastLinePos = 0
					}
					m.PreviewScrollY = lastLinePos
				}
			} else if !m.ShowFlow && m.NormalRightFocus {
				lines := strings.Split(m.DirectoryListing, "\n")
				totalLines := len(lines) + 12 // Approx overhead
				interiorHeight := m.WindowSize.Height - 8
				max := totalLines - interiorHeight
				if max < 0 {
					max = 0
				}
				m.DetailsScrollY = max
			}
		case "tab":
			// Tab switches focus
			if m.ShowFlow {
				if m.RightPanelFocus == FocusFlowList {
					m.RightPanelFocus = FocusFilePreview
				} else {
					m.RightPanelFocus = FocusFlowList
				}
			} else {
				m.NormalRightFocus = !m.NormalRightFocus
			}
		case "d":
			m.ShowDiagnosticsPopup = true
			m.DiagnosticsScrollY = 0
			return m, nil
		case "f":
			m.ShowFlow = !m.ShowFlow
			m.CumulativeFlow = m.ShowFlow // Default to cumulative when entering flow mode
			m.ShowDiagnostics = false
			if len(m.TraceResult.FlowNodes) > 0 && m.FlowSelectedIdx >= len(m.TraceResult.FlowNodes) {
				m.FlowSelectedIdx = 0
			}
			if m.ShowFlow {
				m.loadSelectedFile()
			}
		case "F":
			// Toggle Cumulative Mode
			m.CumulativeFlow = !m.CumulativeFlow
			if m.CumulativeFlow {
				m.ShowFlow = true
				m.ShowDiagnostics = false
				m.loadSelectedFile()
			}
		case "w":
			m.InputMode = true
			m.InputBuffer.Focus()
			m.InputBuffer.SetValue("")
			return m, textinput.Blink
		}
	}

	// GLOBAL SCROLL CAPPING
	// Help
	if m.ShowHelp {
		lines := strings.Split(m.HelpContent, "\n")
		max := len(lines) - (m.WindowSize.Height - 8)
		if max < 0 {
			max = 0
		}
		if m.HelpScrollY > max {
			m.HelpScrollY = max
		}
		if m.HelpScrollY < 0 {
			m.HelpScrollY = 0
		}
	}

	// Diagnostics
	if m.ShowDiagnosticsPopup {
		lines := strings.Split(m.DiagnosticsReport, "\n")
		max := len(lines) - (m.WindowSize.Height - 10)
		if max < 0 {
			max = 0
		}
		if m.DiagnosticsScrollY > max {
			m.DiagnosticsScrollY = max
		}
		if m.DiagnosticsScrollY < 0 {
			m.DiagnosticsScrollY = 0
		}
	}

	// Preview
	if m.ShowFlow && m.RightPanelFocus == FocusFilePreview {
		lines := strings.Split(m.PreviewContent, "\n")
		contentHeight := m.WindowSize.Height - 10
		visibleHeight := (contentHeight - contentHeight/2) - 1
		max := len(lines) - visibleHeight
		if max < 0 {
			max = 0
		}
		if m.PreviewScrollY > max {
			m.PreviewScrollY = max
		}
		if m.PreviewScrollY < 0 {
			m.PreviewScrollY = 0
		}
	}

	// Details
	if !m.ShowFlow && m.NormalRightFocus {
		lines := strings.Split(m.DirectoryListing, "\n")
		totalLines := len(lines) + 12 // Approx overhead
		interiorHeight := m.WindowSize.Height - 8
		max := totalLines - interiorHeight
		if max < 0 {
			max = 0
		}
		if m.DetailsScrollY > max {
			m.DetailsScrollY = max
		}
		if m.DetailsScrollY < 0 {
			m.DetailsScrollY = 0
		}
	}

	return m, cmd
}

func (m *AppModel) performSearch() {
	term := strings.ToLower(m.InputBuffer.Value())
	if term == "" {
		// Reset
		m.SearchActive = false
		m.FilteredIndices = make([]int, len(m.TraceResult.PathEntries))
		for i := range m.TraceResult.PathEntries {
			m.FilteredIndices[i] = i
		}
	} else {
		m.SearchActive = true
		m.SearchMatches = make(map[int]string)
		seenDirs := make(map[string]bool)

		var result []int
		for i, entry := range m.TraceResult.PathEntries {
			dir := entry.Value

			// Deduplication: Only show unique directories in search results
			if seenDirs[dir] {
				continue
			}

			// Filesystem Scan
			files, err := os.ReadDir(dir)
			if err != nil {
				continue
			}

			// Find *best* match (exact matches preferred over prefix)
			// Or just first one? User said "first path entry that finds that binary".
			// If we have multiple matches in the directory, we need to pick one to show details for.
			// Let's store the first one we find, but prefer exact term match.

			var matchedFile string
			found := false

			// First pass: Exact match check (if we had efficient lookup).
			// Since we are iterating, let's just find first prefix match,
			// but if we find exact match later, swap it?

			for _, f := range files {
				if f.IsDir() {
					continue
				}
				name := strings.ToLower(f.Name())

				if strings.HasPrefix(name, term) {
					matchedFile = f.Name() // Store original case
					found = true

					// Optimisation: If exact match, we can stop looking in this dir.
					if name == term {
						break
					}
					// Continue to see if there is a better (exact) match?
					// Example: term="py", matches "python", "pypi".
					// If we find "python" first, that's good.
				}
			}

			if found {
				seenDirs[dir] = true
				result = append(result, i)
				m.SearchMatches[i] = matchedFile
			}
		}
		m.FilteredIndices = result
	}

	// Bounds check
	if m.SelectedIdx >= len(m.FilteredIndices) {
		if len(m.FilteredIndices) > 0 {
			m.SelectedIdx = len(m.FilteredIndices) - 1
		} else {
			m.SelectedIdx = 0
		}
	}

	m.loadDirectoryListing()
}

func (m *AppModel) loadDirectoryListing() {
	if len(m.FilteredIndices) == 0 || m.SelectedIdx >= len(m.FilteredIndices) {
		m.DirectoryListing = ""
		return
	}

	idx := m.FilteredIndices[m.SelectedIdx]
	dir := m.TraceResult.PathEntries[idx].Value
	dir = expandTilde(dir)

	files, err := os.ReadDir(dir)
	if err != nil {
		m.DirectoryListing = fmt.Sprintf("Error reading directory: %v", err)
		return
	}

	m.FileCount = 0
	m.DirCount = 0
	var sb strings.Builder
	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)

	for _, f := range files {
		info, err := f.Info()
		if err != nil {
			continue
		}

		if f.IsDir() {
			m.DirCount++
		} else {
			m.FileCount++
		}

		// Permissions
		mode := info.Mode().String()

		// Size
		size := info.Size()
		sizeStr := fmt.Sprintf("%d", size)
		if size > 1024*1024 {
			sizeStr = fmt.Sprintf("%.1fM", float64(size)/(1024*1024))
		} else if size > 1024 {
			sizeStr = fmt.Sprintf("%.1fK", float64(size)/1024)
		}

		// Time
		modTime := info.ModTime().Format("Jan 02 15:04")

		// Icon and Name
		icon := "📄"
		if f.IsDir() {
			icon = "📁"
		} else if info.Mode().Perm()&0111 != 0 {
			icon = "🚀"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s %s\n", mode, sizeStr, modTime, icon, f.Name())
	}
	w.Flush()
	m.DirectoryListing = sb.String()
	m.DetailsScrollY = 0 // Reset scroll position when loading new directory
}

func (m *AppModel) loadSelectedFile() {
	// Save current scroll position before switching files
	if m.PreviewPath != "" {
		m.ScrollPositions[m.PreviewPath] = m.PreviewScrollY
	}

	if m.FlowSelectedIdx < 0 || m.FlowSelectedIdx >= len(m.TraceResult.FlowNodes) {
		m.PreviewContent = ""
		m.PreviewPath = ""
		return
	}

	node := m.TraceResult.FlowNodes[m.FlowSelectedIdx]
	path := node.FilePath

	// Expand ~
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[1:])
		}
	}

	m.PreviewPath = path

	// Restore previous scroll position if we've viewed this file before
	if savedScroll, exists := m.ScrollPositions[path]; exists {
		m.PreviewScrollY = savedScroll
	} else {
		m.PreviewScrollY = 0 // Reset scroll for new files
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			m.PreviewContent = fmt.Sprintf("File not found: %s\n(This file does not exist on your system)", path)
		} else {
			m.PreviewContent = fmt.Sprintf("Error reading file: %v", err)
		}
	} else {
		m.PreviewContent = string(content)
	}
}

// InitTraceCmd starts the trace in background.
func InitTraceCmd() tea.Cmd {
	return func() tea.Msg {
		shell := trace.DetectShell(os.Getenv("SHELL"))

		// Use RunTraceSync for simplicity in Tea command if it blocks reading.
		// We need a non-stream version to just return the result for now.
		// Or we can stream updates. For now, batch mode is simpler for V1.
		stderr, err := trace.RunTrace(shell)
		if err != nil {
			return MsgError(err)
		}
		defer stderr.Close()

		parser := trace.NewParser(shell)
		events, errs := parser.Parse(stderr)

		var allEvents []model.TraceEvent
		for ev := range events {
			allEvents = append(allEvents, ev)
		}

		// Wait for errs
		if e := <-errs; e != nil {
			log.Printf("Parser warning: %v", e)
		}

		analyzer := trace.NewAnalyzer()
		res := analyzer.Analyze(allEvents)
		return MsgTraceReady(res)
	}
}
