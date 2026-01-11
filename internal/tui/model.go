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
	CumulativeFlow  bool // Cumulative highlighting mode ('F')
	NotExecuted     bool // True if this file was inserted as a placeholder (didn't appear in trace)

	// Search State
	InputMode       bool
	InputBuffer     textinput.Model
	FilteredIndices []int          // Indices of PathEntries to show
	SearchMatches   map[int]string // Map of PathEntry Index -> Matched Filename
	SearchActive    bool

	// Flow Preview State
	RightPanelFocus int // 0 = Flow List, 1 = File Preview
	PreviewContent  string
	PreviewScrollY  int
	PreviewPath     string

	// Components
	DetailsViewport viewport.Model
}

const (
	FocusFlowList    = 0
	FocusFilePreview = 1
)

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
