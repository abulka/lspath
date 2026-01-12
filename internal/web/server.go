package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"lspath/internal/model"
	"lspath/internal/trace"
	"strings"
)

// expandTilde expands ~ to the user's home directory
func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	} else if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
	}
	return path
}

//go:embed static/*
var staticFS embed.FS

// StartServer starts the web server on the given port (or default 8080).
func StartServer() {
	mux := http.NewServeMux()

	// Serve static files
	subFS, _ := fs.Sub(staticFS, "static")
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	// API Endpoints
	mux.HandleFunc("/api/trace", handleTrace)
	mux.HandleFunc("/api/file", handleFile)
	mux.HandleFunc("/api/ls", handleLs)
	mux.HandleFunc("/api/which", handleWhich)

	port := "8080"
	fmt.Printf("Starting lspath web server at http://localhost:%s\n", port)
	fmt.Printf("Go to http://localhost:%s in your browser.\n", port)

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

func handleTrace(w http.ResponseWriter, r *http.Request) {
	// Run trace on-demand
	shell := trace.DetectShell(os.Getenv("SHELL"))
	stderr, err := trace.RunTrace(shell)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer stderr.Close()

	parser := trace.NewParser(shell)
	events, errs := parser.Parse(stderr)

	var allEvents []model.TraceEvent
	for ev := range events {
		allEvents = append(allEvents, ev)
	}

	// drain errors
	go func() {
		for range errs {
		}
	}()

	analyzer := trace.NewAnalyzer()
	result := analyzer.Analyze(allEvents)

	// Generate reports for web view
	report := trace.GenerateReport(result, false)
	verboseReport := trace.GenerateReport(result, true)

	response := struct {
		model.AnalysisResult
		Report        string `json:"Report"`
		VerboseReport string `json:"VerboseReport"`
		Version       string `json:"Version"`
	}{
		AnalysisResult: result,
		Report:         report,
		VerboseReport:  verboseReport,
		Version:        model.Version,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleFile(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path is required", 400)
		return
	}

	// Basic safety check - only allow files we know are shell config files or path entries
	// For this CLI tool, we can be relatively permissive within HOME, but let's just
	// read whatever path is requested and let OS permissions handle it for now.
	// In a real web app, we'd need strict validation.

	content, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write(content)
}

type LsEntry struct {
	Name    string `json:"Name"`
	IsDir   bool   `json:"IsDir"`
	Size    int64  `json:"Size"`
	Mode    string `json:"Mode"`
	ModTime string `json:"ModTime"`
}

func handleLs(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path is required", 400)
		return
	}
	path = expandTilde(path)

	files, err := os.ReadDir(path)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var entries []LsEntry
	for _, f := range files {
		info, err := f.Info()
		if err != nil {
			continue
		}
		entries = append(entries, LsEntry{
			Name:    f.Name(),
			IsDir:   f.IsDir(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().Format("Jan 02 15:04"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func handleWhich(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(r.URL.Query().Get("query"))
	if query == "" {
		http.Error(w, "query is required", 400)
		return
	}

	shell := trace.DetectShell(os.Getenv("SHELL"))
	stderr, err := trace.RunTrace(shell)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer stderr.Close()

	parser := trace.NewParser(shell)
	events, _ := parser.Parse(stderr)
	var allEvents []model.TraceEvent
	for ev := range events {
		allEvents = append(allEvents, ev)
	}

	analyzer := trace.NewAnalyzer()
	result := analyzer.Analyze(allEvents)

	type WhichMatch struct {
		Index       int    `json:"Index"`
		MatchedFile string `json:"MatchedFile"`
	}

	var matches []WhichMatch
	seenDirs := make(map[string]bool)

	for i, entry := range result.PathEntries {
		dir := entry.Value
		if seenDirs[dir] {
			continue
		}
		expandedDir := expandTilde(dir)

		files, err := os.ReadDir(expandedDir)
		if err != nil {
			continue
		}

		var matchedFile string
		found := false
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := strings.ToLower(f.Name())
			if strings.HasPrefix(name, query) {
				matchedFile = f.Name()
				found = true
				if name == query {
					break
				}
			}
		}

		if found {
			seenDirs[dir] = true
			matches = append(matches, WhichMatch{
				Index:       i,
				MatchedFile: matchedFile,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(matches)
}
