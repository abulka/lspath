package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"lspath/internal/model"
	"lspath/internal/trace"
	"lspath/internal/tui"
	"lspath/internal/web"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	jsonFlag := flag.Bool("json", false, "Output analysis as JSON")
	webFlag := flag.Bool("w", false, "Start Web Mode (not implemented)")
	flag.Parse()

	if *webFlag {
		web.StartServer()
		return
	}

	if *jsonFlag {
		runJsonMode()
		return
	}

	// Default: TUI
	runTuiMode()
}

func runJsonMode() {
	shell := trace.DetectShell(os.Getenv("SHELL"))
	// In JSON mode, we block and run trace sync
	stderr, err := trace.RunTrace(shell)
	if err != nil {
		panic(err)
	}

	parser := trace.NewParser(shell)
	events, errs := parser.Parse(stderr)

	var allEvents []model.TraceEvent
	for ev := range events {
		allEvents = append(allEvents, ev)
	}

	// Drain errors
	go func() {
		for range errs {
		}
	}()

	analyzer := trace.NewAnalyzer()
	result := analyzer.Analyze(allEvents)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(result)
}

func runTuiMode() {
	m := tui.InitialModel()
	p := tea.NewProgram(&m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
