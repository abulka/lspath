# lspath

[![Version](https://img.shields.io/badge/version-1.3.0-blue.svg)](internal/model/version.go)

**lspath** is a powerful terminal-based tool designed to help you analyze, debug, and optimize your system's `PATH`. It visualizes how your PATH is constructed by your shell's startup sequence and identifies common issues like duplicates and missing directories.

---

## � Screenshots

### Interactive TUI
![TUI Mode](doco/screenshot-1-tui.png)

### Shell Startup Flow
![Flow Mode](doco/screenshot-2-flow.png)

### Web Dashboard
![Web Mode](doco/screenshot-3-web.png)

---

## �🚀 Features

### 🖥️ TUI Mode (Default)
Interactive terminal interface built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).
- **Flow Mode**: Visualize the "evolution" of your PATH as shell startup files (`.zshrc`, `.zprofile`, etc.) are executed.
- **Diagnostics**: Instantly identify broken links, missing directories, and duplicate entries.
- **Which Mode**: Find where commands are located and identify "shadowed" binaries.
- **File Preview**: Inspect the exact lines in your configuration files that modify the PATH.

### 🌐 Web Mode
Start a local web server to explore your PATH in a modern browser.
- **Interactive Visualization**: Explore the directory structure and shell trace visually.
- **Status Dashboard**: Quick overview of PATH health.

### ⌨️ CLI Mode
Non-interactive mode for scripting and quick reports.
- **JSON Output**: Export raw analysis data for downstream processing.
- **Diagnostic Reports**: Generate compact or detailed human-readable reports.

---

## 🛠️ Installation

### Building from Source
Ensure you have [Go](https://go.dev/) installed (version 1.24+ recommended).

```bash
git clone https://github.com/youruser/lspath.git
cd lspath
go build -o lspath main.go
```

---

## 📖 Usage

Run `lspath` without arguments to enter the interactive TUI.

```bash
lspath [options]
```

### Options

| Short | Long | Description |
| :--- | :--- | :--- |
| `-h` | `--help` | Show help message |
| `-r` | `--report` | Generate a detailed diagnostic report (CLI mode) |
| `-v` | `--verbose` | Include detailed internal model data in the report |
| `-o` | `--output` | Save report to a specified file (requires `-r`) |
| `-j` | `--json` | Output raw analysis data as JSON |
| `-w` | `--web` | Start Web Mode on http://localhost:8080 |
| `-V` | `--version` | Print version information |
| `-u` | `--update` | Check for latest version (not implemented) |

### Examples

```bash
# Start interactive TUI
lspath

# Print a diagnostic report to stdout
lspath --report

# Save a verbose report to a text file
lspath -r -v -o path_debug.txt

# Export analysis as JSON for other tools
lspath --json > path_data.json

# Start the web interface
lspath --web
```

---

## ⌨️ TUI Keyboard Shortcuts

| Key | Action |
| :--- | :--- |
| `↑/↓` or `k/j` | Navigate PATH entries |
| `f` | Toggle **Flow Mode** (trace shell startup) |
| `w` | Toggle **Which Mode** (search for binaries) |
| `d` | Show **Diagnostics** report |
| `c` | Toggle **Cumulative View** in Flow Mode |
| `q` or `Ctrl+C` | Quit |

---

---

## Help 

See [internal/tui/help.md](internal/tui/help.md) for help shown by the internal help system.

## 📜 License
[MIT](LICENSE)

---

## 🔄 Versioning
Current Version: **1.3.0**
