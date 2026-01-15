package model

// Centralized icons for the UI components
// Using simple single-width characters for consistent terminal rendering
const (
	IconPriorityHigh = "¹" // Up arrow for highest priority
	IconPriorityLow  = "¶" // Down arrow for lowest priority
	IconFirst        = "¹" //
	IconLast         = "¶" //
	IconDuplicate    = "≈" // Almost equal (duplicate)
	IconSymlink      = "→" // Right arrow (symlink)
	IconMissing      = "✗" // Thin X (missing)
	IconOK           = " " // Space (OK - no icon to reduce noise)
	IconSession      = "◆" // Diamond for session-only paths
)
