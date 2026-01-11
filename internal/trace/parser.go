package trace

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"strings"

	"lspath/internal/model"
)

// Parser handles the parsing of shell trace output.
type Parser struct {
	re *regexp.Regexp
}

// NewParser creates a new Parser with the appropriate regex for the shell.
func NewParser(shell Shell) *Parser {
	// Pattern: ^\+* ?(.*?):(\d+)>(.*)
	// Matches:
	// + file:10>command
	// ++ file:10>command (nested)
	// Groups:
	// 1: File
	// 2: Line
	// 3: Command
	return &Parser{
		re: regexp.MustCompile(`^\+* ?(.*?):(\d+)>(.*)`),
	}
}

// Parse reads the trace stream and returns a channel of TraceEvents.
// It runs asynchronously.
func (p *Parser) Parse(r io.Reader) (chan model.TraceEvent, chan error) {
	events := make(chan model.TraceEvent)
	errs := make(chan error, 1) // Buffered to avoid blocking if receiver stops

	go func() {
		defer close(events)
		defer close(errs)

		scanner := bufio.NewScanner(r)
		// Large buffer for long PATH lines
		buf := make([]byte, 0, 1024*1024)
		scanner.Buffer(buf, 10*1024*1024) // 10MB max line, should be enough

		for scanner.Scan() {
			line := scanner.Text()
			matches := p.re.FindStringSubmatch(line)
			if len(matches) == 4 {
				file := matches[1]
				lineNumStr := matches[2]
				cmd := matches[3]
				lineNum, _ := strconv.Atoi(lineNumStr)

				// We are looking for PATH changes.
				// Heuristic: command starts with "PATH=" or "export PATH="
				// Or "typeset -x PATH=" etc.
				// Simple heuristic: contains "PATH="
				// The trace expands variables, so we see "PATH=/foo:/bar"

				pathChange := ""
				if strings.Contains(cmd, "PATH=") {
					// Check if it's an assignment.
					// Could be "export PATH=..." or just "PATH=..."
					// Also watch out for "INFOPATH="

					// Simple tokenization to be safer
					tokens := strings.Fields(cmd)
					isPathAssign := false
					for _, token := range tokens {
						if strings.HasPrefix(token, "PATH=") {
							isPathAssign = true
							// Extract the value
							parts := strings.SplitN(token, "=", 2)
							if len(parts) == 2 {
								pathChange = cleanPathValue(parts[1])
							}
							break
						}
					}
					// Handle "export PATH" separate from assignment
					if !isPathAssign && strings.HasPrefix(cmd, "export PATH") && !strings.Contains(cmd, "=") {
						// This is just 'export PATH' without assignment, ignore or track?
						// Usually follows an assignment. Ignoring for diffs, but maybe relevant for flow.
					}
				}

				event := model.TraceEvent{
					File:       file,
					Line:       lineNum,
					RawCommand: cmd,
					PathChange: pathChange,
				}
				events <- event
			}
		}
		if err := scanner.Err(); err != nil {
			errs <- err
		}
	}()

	return events, errs
}

func cleanPathValue(v string) string {
	// Remove quotes if present
	v = strings.TrimPrefix(v, "'")
	v = strings.TrimSuffix(v, "'")
	v = strings.TrimPrefix(v, "\"")
	v = strings.TrimSuffix(v, "\"")
	return v
}
