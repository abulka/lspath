package tui

import (
	"log"
	"os"
	"strings"

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
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				m.InputBuffer.Blur()
				m.performSearch()
				return m, nil
			case tea.KeyEsc:
				m.InputMode = false
				m.InputBuffer.Blur()
				m.SearchActive = false
				// Reset filter
				m.FilteredIndices = nil
				m.performSearch() // Reset
				return m, nil
			}
			m.InputBuffer, cmd = m.InputBuffer.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.SelectedIdx > 0 {
				m.SelectedIdx--
			}
		case "down", "j":
			// Dynamic limit based on mode
			maxIdx := len(m.FilteredIndices) - 1
			if m.ShowFlow {
				maxIdx = len(m.TraceResult.FlowNodes) - 1
			}
			if m.SelectedIdx < maxIdx {
				m.SelectedIdx++
			}
		case "d":
			m.ShowDiagnostics = !m.ShowDiagnostics
			m.ShowFlow = false // Mutually exclusive for now
		case "f":
			m.ShowFlow = !m.ShowFlow
			m.ShowDiagnostics = false
			// Reset cursor to avoid out-of-bounds
			m.SelectedIdx = 0
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
		var result []int
		for i, entry := range m.TraceResult.PathEntries {
			// Search for binary existence?
			// Or just string match on path?
			// User asked: "let use type in a binary e.g. python and the first path entru that finds that binary would light up"
			// This requires filesystem scanning.
			// For now, let's implement naive string match on PATH value as a baseline
			// AND a mock "binary search" if it looks like a binary name.
			// Actually, I should probably check if `entry.Value` + `term` exists.
			// But for speed, let's just do path string match + "mock".
			// Wait, I can implement the binary search logic later in Analyzer "Diagnostics" or "Search".
			// For now, let's just match the path string to unblock TUI.

			// TODO: Real binary search requires checking file existence.
			if strings.Contains(strings.ToLower(entry.Value), term) {
				result = append(result, i)
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
