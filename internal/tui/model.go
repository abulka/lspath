package tui

import (
	"lspath/internal/model"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// AppModel holds the TUI state.
type AppModel struct {
	// Data
	TraceResult model.AnalysisResult
	Loading     bool
	Err         error

	// UI State
	SelectedIdx     int
	FlowSelectedIdx int // Index of selected flow node in Flow Mode
	WindowSize      tea.WindowSizeMsg

	// View Modes
	ShowDiagnostics bool
	ShowFlow        bool

	// Input Mode ('w' command)
	InputMode       bool
	InputBuffer     textinput.Model
	FilteredIndices []int // Indices of PathEntries matching valid search
	SearchActive    bool

	// Components
	DetailsViewport viewport.Model
}

// InitialModel returns the initial state.
func InitialModel() AppModel {
	ti := textinput.New()
	ti.Placeholder = "Binary name..."
	ti.CharLimit = 50
	ti.Width = 20

	return AppModel{
		Loading:     true,
		InputBuffer: ti,
		SelectedIdx: 0,
	}
}
