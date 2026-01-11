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
	// Pattern: .*?\+ ?(.*?):(\d+)>(.*)
	// Matches:
	// + file:10>command
	// ...garbage...+ file:10>command
	return &Parser{
		re: regexp.MustCompile(`.*\+(?: )?([^:]+):(\d+)>(.*)`),
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
				// Identify if this is a PATH assignment
				var isPathChange bool
				var value string

				// 1. Direct Assignment: PATH='...' or export PATH='...'
				// Regex to capture value inside optional quotes.
				// Handles: PATH=val, PATH='val', export PATH="val"
				// Note: cmd is the rest of the trace line.
				// We look for "PATH=" pattern.

				// Find start of "PATH="
				idx := strings.Index(cmd, "PATH=")
				if idx != -1 {
					// Safety check: Needs to be start of string or preceded by space/export
					valid := false
					if idx == 0 {
						valid = true
					} else if idx > 0 && (cmd[idx-1] == ' ' || strings.HasSuffix(cmd[:idx], "export ")) {
						// Ensure it's not SOMEOTHERPATH=
						if idx > 0 && cmd[idx-1] != ' ' {
							// potential suffix match like MYPATH=
							// Check character before
							// If it was "export PATH=", preceding char is space.
							// parsing "match whole word PATH" is tricky without regex.
							// simpler: check if character before P is space or delimiter.
							r := cmd[idx-1]
							if r == ' ' || r == ';' {
								valid = true
							}
						} else {
							valid = true
						}
					}

					if valid {
						// Extract everything after PATH=
						// Value might be quoted.
						rhs := cmd[idx+5:]
						value = cleanPathValue(rhs)
						isPathChange = true
					}
				}

				if isPathChange {
					pathChange = value
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
