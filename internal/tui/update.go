package tui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"lspath/internal/model"
	"lspath/internal/trace"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

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

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
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
		case "pgup":
			// Page up in preview
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
			}
		case "pgdown":
			// Page down in preview
			if m.ShowFlow && m.RightPanelFocus == FocusFilePreview {
				m.PreviewScrollY += 10
			} else if !m.ShowFlow && m.NormalRightFocus {
				m.DetailsScrollY += 10
			}
		case "ctrl+d":
			// Vim: half page down
			if m.ShowFlow && m.RightPanelFocus == FocusFilePreview {
				m.PreviewScrollY += 5
			} else if !m.ShowFlow && m.NormalRightFocus {
				m.DetailsScrollY += 5
			}
		case "ctrl+u":
			// Vim: half page up
			if m.ShowFlow && m.RightPanelFocus == FocusFilePreview {
				if m.PreviewScrollY > 5 {
					m.PreviewScrollY -= 5
				} else {
					m.PreviewScrollY = 0
				}
			} else if !m.ShowFlow && m.NormalRightFocus {
				if m.DetailsScrollY > 5 {
					m.DetailsScrollY -= 5
				} else {
					m.DetailsScrollY = 0
				}
			}
		case "ctrl+f":
			// Vim: full page down
			if m.ShowFlow && m.RightPanelFocus == FocusFilePreview {
				m.PreviewScrollY += 10
			} else if !m.ShowFlow && m.NormalRightFocus {
				m.DetailsScrollY += 10
			}
		case "ctrl+b":
			// Vim: full page up
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
				// Get RHS content to calculate lines
				// Note: We don't have a direct "DetailsContent" field, it's built in View()
				// However, the main part that scrolls is DirectoryListing.
				// We can approximate or just look at DirectoryListing if it's significant.
				// Actually, View() uses strings.Split(finalRightViewContent, "\n")
				// We can reconstruct it or just use a large number if we don't want to re-render.
				// Re-calculating lines here:
				lines := strings.Split(m.DirectoryListing, "\n")
				// Headers add ~8-10 lines
				totalLines := len(lines) + 10
				interiorHeight := m.WindowSize.Height - 8
				if interiorHeight < 1 {
					interiorHeight = 1
				}
				lastLinePos := totalLines - interiorHeight
				if lastLinePos < 0 {
					lastLinePos = 0
				}
				m.DetailsScrollY = lastLinePos
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
			m.ShowDiagnostics = !m.ShowDiagnostics
			m.ShowFlow = false
		case "f":
			m.ShowFlow = !m.ShowFlow
			m.CumulativeFlow = false // Reset cumulative on toggle? Or persist?
			// Reset to Specific mode when entering normally.
			m.ShowDiagnostics = false
			// Reset flow cursor if opening first time?
			if len(m.TraceResult.FlowNodes) > 0 && m.FlowSelectedIdx >= len(m.TraceResult.FlowNodes) {
				m.FlowSelectedIdx = 0
			}
			// Load file when entering flow mode
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
			} else {
				// If turning off cumulative, stay in flow mode?
			}
		case "w":
			m.InputMode = true
			m.InputBuffer.Focus()
			m.InputBuffer.SetValue("")
			return m, textinput.Blink
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
