package tui

import (
	"lspath/internal/model"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	_ "embed"
)

//go:embed help.md
var helpContent string

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
	ScrollPositions map[string]int // Map of file path -> scroll position

	// Components
	DetailsViewport  viewport.Model
	DirectoryListing string
	DetailsScrollY   int
	NormalRightFocus bool
	FileCount        int
	DirCount         int

	// Help State
	ShowHelp    bool
	HelpScrollY int
	HelpContent string

	// Diagnostics Popup State
	ShowDiagnosticsPopup bool
	DiagnosticsScrollY   int
	DiagnosticsReport    string
	DiagnosticsVerbose   bool
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
		Loading:         true,
		InputBuffer:     ti,
		SelectedIdx:     0,
		ScrollPositions: make(map[string]int),
		HelpContent:     strings.ReplaceAll(helpContent, "{{VERSION}}", model.Version),
	}
}
