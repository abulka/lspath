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
