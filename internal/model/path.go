package model

// PathEntry represents a single directory in the system PATH.
type PathEntry struct {
	Value       string   // The directory path (e.g., /usr/bin)
	SourceFile  string   // File where it was added (e.g., .zshrc)
	LineNumber  int      // Line number in the source file
	Mode        string   // "Login" or "Interactive" or "Unknown"
	Shadows     []string // List of paths that this entry shadows (if applicable)
	IsDuplicate bool     // True if this is a duplicate entry
	DuplicateOf int      // Index of the original entry if this is a duplicate
	Remediation string   // Advice on how to fix/remove if duplicate

	// Flow Attribution
	FlowID string // ID of the ConfigNode this belongs to
}

// TraceEvent represents a single line of debug output from the shell.
type TraceEvent struct {
	Directory  string // Directory context of execution
	File       string // Source file
	Line       int    // Line number
	RawCommand string // The command being executed
	PathChange string // If this event modified PATH, what was the new value?
}

// ConfigNode represents a file in the config loading flow.
type ConfigNode struct {
	ID       string // e.g. "node-1"
	FilePath string // e.g. "/etc/zshenv"
	Order    int    // Sequence order (1, 2, 3...)
	Entries  []int  // Indices of PathEntries contributed by this node
}

// AnalysisResult contains the processed data from a trace.
type AnalysisResult struct {
	PathEntries []PathEntry
	FlowNodes   []ConfigNode
	Diagnostics []string
}
