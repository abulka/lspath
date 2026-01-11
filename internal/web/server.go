package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"lspath/internal/model"
	"lspath/internal/trace"
)

//go:embed static/*
var staticFS embed.FS

// StartServer starts the web server on the given port (or default 8080).
func StartServer() {
	mux := http.NewServeMux()

	// Serve static files
	subFS, _ := fs.Sub(staticFS, "static")
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	// API Endpoint
	mux.HandleFunc("/api/trace", handleTrace)

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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
