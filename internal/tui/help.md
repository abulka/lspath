# lspath Help

lspath is a TUI tool for analyzing and navigating your system PATH.

## Keyboard Shortcuts

### Global
- `?`: Toggle this help dialog
- `q` / `Ctrl+C`: Quit the application
- `f`: Toggle Flow Mode (visualize shell startup steps)
- `d`: Toggle Diagnostics (show duplicates and remedies)
- `w`: run `which` on a command (if prompt is implemented)

### Normal Mode (PATH List)
- `↑`/`k`: Move selection up
- `↓`/`j`: Move selection down
- `PgUp`/`PgDn`: Scroll list by page
- `Home`/`g`: Jump to top
- `End`/`G`: Jump to bottom
- `/`: Start searching/filtering PATH entries
- `Enter` (during search): Confirm search
- `Esc` (during search): Cancel search
- `Tab`: Switch focus between PATH list and Details panel

### Details Panel (RHS)
- When focused (via `Tab`):
    - `↑`/`↓`: Scroll directory listing
    - `PgUp`/`PgDn`: Scroll by page
    - `Home`/`g`: Jump to top of listing
    - `End`/`G`: Jump to bottom of listing

### Flow Mode
- `↑`/`↓`: Navigate configuration steps
- `Tab`: Switch focus between Flow list and File Preview
- When Preview is focused:
    - `↑`/`↓`: Scroll file content
    - `PgUp`/`PgDn`: Scroll by page
    - `Home`/`g`: Jump to start of file
    - `End`/`G`: Jump to end of file
- `c`: Toggle Cumulative view (show PATH as it was at that step)
- `Esc`: Return to Normal Mode

## Features

### Directory Listing
The details panel shows a long-style (`ls -l`) listing of the selected directory, including permissions, size, and modification time.

### Diagnostics
Identifies duplicate PATH entries and provides remediation advice (e.g., which config file to edit to remove the duplicate).

### File Preview
In Flow Mode, you can see the exact lines in your shell configuration files (like `.zshrc` or `.zprofile`) that modify your PATH. Highlighting indicates `export`, `source`, and `eval` commands.
