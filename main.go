package main

import (
	"encoding/json"
	"fmt"
	"os"

	"lspath/internal/model"
	"lspath/internal/trace"
	"lspath/internal/tui"
	"lspath/internal/web"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/pflag"
	"github.com/tcnksm/go-latest"
)

func checkUpdate(currentVer string) {
	githubTag := &latest.GithubTag{
		Owner:      "abulka",
		Repository: "lspath",
	}

	res, err := latest.Check(githubTag, currentVer)
	if err != nil {
		return // Silently fail
	}

	if res.Outdated {
		fmt.Printf("\n✨ A new version is available: %s (you have %s)\n", res.Current, currentVer)
		fmt.Println("👉 Download it from https://github.com/abulka/lspath/releases")
	} else if pflag.Lookup("update").Changed {
		fmt.Printf("✅ You are using the latest version: %s\n", currentVer)
	}
}

func main() {
	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lspath [options]\n\n")
		fmt.Fprintf(os.Stderr, "lspath is a tool for analyzing and debugging your system PATH.\n")
		fmt.Fprintf(os.Stderr, "It shows your actual PATH with full attribution from shell config files.\n")
		fmt.Fprintf(os.Stderr, "Session-specific entries (e.g., virtual environments) are clearly marked.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		pflag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  lspath              # Start TUI mode (unified view)\n")
		fmt.Fprintf(os.Stderr, "  lspath --report     # Print diagnostic report to stdout\n")
		fmt.Fprintf(os.Stderr, "  lspath -r -o r.txt  # Save report to file\n")
		fmt.Fprintf(os.Stderr, "  lspath --json       # Output analysis as JSON\n")
	}

	jsonFlag := pflag.BoolP("json", "j", false, "Output raw analysis data as JSON")
	reportFlag := pflag.BoolP("report", "r", false, "Generate a detailed diagnostic report (CLI mode)")
	outputFlag := pflag.StringP("output", "o", "", "Save report to the specified file (combined with --report)")
	verboseFlag := pflag.BoolP("verbose", "v", false, "Include detailed internal model data in the report")
	webFlag := pflag.BoolP("web", "w", false, "Start Web Mode on http://localhost:8080")
	versionFlag := pflag.BoolP("version", "V", false, "Print version information")
	updateFlag := pflag.BoolP("update", "u", false, "Check for latest version (not implemented)")
	helpFlag := pflag.BoolP("help", "h", false, "Show this help message")
	pflag.Parse()

	if *helpFlag {
		pflag.Usage()
		return
	}

	if *versionFlag {
		fmt.Printf("lspath version %s\n", model.Version)
		return
	}

	if *updateFlag {
		checkUpdate(model.Version)
		return
	}

	if *webFlag {
		web.StartServer()
		return
	}

	if *reportFlag {
		runReportMode(*outputFlag, *verboseFlag)
		return
	}

	if *jsonFlag {
		runJsonMode()
		return
	}

	// Default: TUI
	runTuiMode()
}

func runReportMode(outputFile string, verbose bool) {
	sessionPath := os.Getenv("PATH")

	// Run shell trace to find config file sources
	shell := trace.DetectShell(os.Getenv("SHELL"))
	stderr, err := trace.RunTrace(shell, trace.SandboxInitialPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running trace: %v\n", err)
		os.Exit(1)
	}

	parser := trace.NewParser(shell)
	events, errs := parser.Parse(stderr)
	var allEvents []model.TraceEvent
	for ev := range events {
		allEvents = append(allEvents, ev)
	}
	go func() {
		for range errs {
		}
	}()

	// Unified analysis: merge trace results with session PATH
	analyzer := trace.NewAnalyzer()
	result := analyzer.AnalyzeUnified(sessionPath, allEvents)

	report := trace.GenerateReport(result, verbose)

	if outputFile != "" {
		err := os.WriteFile(outputFile, []byte(report), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing report to %s: %v\n", outputFile, err)
			os.Exit(1)
		}
		fmt.Printf("Report saved to %s\n", outputFile)
	} else {
		fmt.Println(report)
	}
}

func runJsonMode() {
	sessionPath := os.Getenv("PATH")

	shell := trace.DetectShell(os.Getenv("SHELL"))
	stderr, err := trace.RunTrace(shell, trace.SandboxInitialPath)
	if err != nil {
		panic(err)
	}

	parser := trace.NewParser(shell)
	events, errs := parser.Parse(stderr)

	var allEvents []model.TraceEvent
	for ev := range events {
		allEvents = append(allEvents, ev)
	}

	go func() {
		for range errs {
		}
	}()

	analyzer := trace.NewAnalyzer()
	result := analyzer.AnalyzeUnified(sessionPath, allEvents)

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
