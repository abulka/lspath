# Development Notes

This document provides information for developers working on **lspath**.

## 🛠️ Local Development Environment

### Prerequisites
- **Go**: Version 1.24 or higher.
- **Git**: For version control.
- **GoReleaser**: (Optional) For testing the release process locally.

### Setup
1. Clone the repository:
   ```bash
   git clone https://github.com/abulka/lspath.git
   cd lspath
   ```
2. Install dependencies:
   ```bash
   go mod tidy
   ```
3. Run in development mode:
   ```bash
   go run main.go
   ```

---

## 🏗️ Local Build Testing

We use a `Makefile` to simplify common development tasks.

### Snapshot Build
To test the full build process (including `.deb` and `.rpm` generation) without creating a GitHub release:
```bash
make build-test
```
This will create a `dist/` folder containing binaries and packages for all supported platforms.

### Cleanup
To remove the `dist/` folder and other build artifacts:
```bash
make clean
```

---

## 🚀 Making a Release

The project uses **GoReleaser** and **GitHub Actions** for automated releases.

### 1. Update Version
The version is stored in `internal/model/version.go`. Update the `Version` constant if necessary.

### 2. Tag the Release
Create a new git tag matching the version (must start with `v`):
```bash
git tag -a v1.3.1 -m "Release v1.3.1"
```

### 3. Push to GitHub
Pushing the tag will trigger the "Release" GitHub Action:
```bash
git push origin v1.3.1
```

### 4. Verification
Monitor the "Actions" tab in your GitHub repository. Once finished, the binaries and release notes will be automatically available on the "Releases" page.

---

## 📝 Code Structure
- `main.go`: Entry point and CLI flag parsing.
- `internal/model/`: Data structures and core constants (including `Version`).
- `internal/trace/`: The core logic for tracing and analyzing shell startup files.
- `internal/tui/`: Bubble Tea-based terminal user interface components.
- `internal/web/`: Web server and static assets for Web Mode.

---

## 🐛 Known Issues & Quirks

### Session vs Trace Mode PATH Differences

When running `lspath --report`, you may see different "CONFIGURATION FILES FLOW" output depending on whether output is redirected:

**Running directly** (`lspath -r`):
```
CONFIGURATION FILES FLOW
------------------------
 1. Session (Manual/Runtime) Paths added in this terminal session [4 paths]
 2. System (Default) Initial environment PATH [4 paths]
 3. /etc/profile (system-wide profile) [no change]
 ...
```

**With redirection** (`lspath -r > file`):
```
CONFIGURATION FILES FLOW
------------------------
 1. System (Default) Initial environment PATH [4 paths]
 2. /etc/profile (system-wide profile) [no change]
 3.   /etc/profile.d/apps-bin-path.sh (system-wide profile) [1 paths]
 ...
```

#### Why This Happens

1. **Interactive vs Non-Interactive Shells:**
   - When output is sent to terminal, bash runs in **interactive mode** and sources `.bashrc`
   - When redirected to a file (`> file`), bash may skip `.bashrc` (non-interactive behavior)
   - This causes the actual session PATH to differ between the two invocations

2. **Trace Uses Minimal Baseline:**
   - The trace (`bash -xli -c exit`) starts with `SandboxInitialPath = "/usr/bin:/bin:/usr/sbin:/sbin"`
   - Any paths in your actual session but not in the trace get marked as "Session (Manual/Runtime)"
   - Paths like `/usr/local/sbin`, `/usr/local/bin`, `/usr/games`, `/usr/local/games` may be added by `/etc/bash.bashrc` (interactive) but appear as "session" because the trace baseline doesn't include them

3. **Session Node Ordering Issue (Bug):**
   - Currently, session-only paths are placed FIRST in the flow (Order: 0)
   - This is incorrect - System (Default) should always be first logically
   - The code comment says "these are typically venvs etc that prepend to PATH" but this doesn't justify placing them before system defaults in the configuration flow visualization

#### Expected Behavior

- **System (Default)** should always appear first in the flow
- True session additions (from virtual environments, manual exports) should appear after system defaults but may prepend to the actual PATH
- Paths added by interactive config files like `.bashrc` should be attributed to those files, not marked as session-only

#### Workaround

For consistent output across invocations, use `lspath -r -o output.txt` to explicitly save to a file, or ensure you're running in the same shell context.
