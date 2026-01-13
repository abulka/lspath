package model

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LineContext represents a line from a file with surrounding context
type LineContext struct {
	Before2    string // Two lines before the target
	Before1    string // Line before the target
	Target     string // The actual target line
	After1     string // Line after the target
	After2     string // Two lines after the target
	LineNumber int    // Line number of the target
	HasBefore2 bool   // Whether there's a second line before
	HasBefore1 bool   // Whether there's a line before
	HasAfter1  bool   // Whether there's a line after
	HasAfter2  bool   // Whether there's a second line after
	ErrorMsg   string // Error message if file couldn't be read
}

// GetLineContext reads a file and returns the target line with surrounding context
func GetLineContext(filePath string, lineNumber int) LineContext {
	result := LineContext{
		LineNumber: lineNumber,
	}

	// Expand tilde in file path
	if strings.HasPrefix(filePath, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			filePath = strings.Replace(filePath, "~", home, 1)
		}
	}

	file, err := os.Open(filePath)
	if err != nil {
		result.ErrorMsg = fmt.Sprintf("Could not read file: %v", err)
		return result
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	currentLine := 0
	lines := []string{}

	// Read the file line by line
	for scanner.Scan() {
		currentLine++
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		result.ErrorMsg = fmt.Sprintf("Error reading file: %v", err)
		return result
	}

	// Check if line number is valid
	if lineNumber < 1 || lineNumber > len(lines) {
		result.ErrorMsg = fmt.Sprintf("Line %d out of range (file has %d lines)", lineNumber, len(lines))
		return result
	}

	// Get the target line (convert to 0-indexed)
	result.Target = lines[lineNumber-1]

	// Get the lines before if they exist
	if lineNumber > 2 {
		result.Before2 = lines[lineNumber-3]
		result.HasBefore2 = true
	}
	if lineNumber > 1 {
		result.Before1 = lines[lineNumber-2]
		result.HasBefore1 = true
	}

	// Get the lines after if they exist
	if lineNumber < len(lines) {
		result.After1 = lines[lineNumber]
		result.HasAfter1 = true
	}
	if lineNumber+1 < len(lines) {
		result.After2 = lines[lineNumber+1]
		result.HasAfter2 = true
	}

	return result
}
