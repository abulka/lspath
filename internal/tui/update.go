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
				if m.FlowSelectedIdx > 0 {
					m.FlowSelectedIdx--
				}
			} else {
				if m.SelectedIdx > 0 {
					m.SelectedIdx--
				}
			}
		case "down", "j":
			if m.ShowFlow {
				if m.FlowSelectedIdx < len(m.TraceResult.FlowNodes)-1 {
					m.FlowSelectedIdx++
				}
			} else {
				if m.SelectedIdx < len(m.FilteredIndices)-1 {
					m.SelectedIdx++
				}
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
		case "F":
			// Toggle Cumulative Mode
			m.CumulativeFlow = !m.CumulativeFlow
			if m.CumulativeFlow {
				m.ShowFlow = true
				m.ShowDiagnostics = false
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
		var result []int
		for i, entry := range m.TraceResult.PathEntries {
			dir := entry.Value

			// Filesystem Scan
			// Optimization: Skip valid check? No, TUI should show if dir exists.
			files, err := os.ReadDir(dir)
			if err != nil {
				// If directory doesn't exist, we can't find anything there.
				// Should we still check path string match? User said "typing 'pyt' (which should be a wildcard)".
				// Implies matching binaries.
				// But fallback to path match if dir missing?
				// Probably better to ignore missing dirs for *binary* search.
				continue
			}

			found := false
			for _, f := range files {
				if f.IsDir() {
					continue
				}
				name := strings.ToLower(f.Name())

				// Partial match starts with? Or contains?
				// "typing 'pyt' ... nothing displayed". Implies prefix.
				// "wildcard" usually implies prefix or glob.
				// Let's assume Prefix for standard "autocompletion" style feel,
				// but typical `which` might expect exact match.
				// User said "wildcard", so let's do HasPrefix for partial matching typing.

				if strings.HasPrefix(name, term) {
					found = true
					break
				}
			}

			if found {
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
