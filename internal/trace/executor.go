package trace

import (
	"bufio"
	"io"
	"os"
	"os/exec"
)

// RunTrace executes the shell trace command and returns the stderr pipe.
func RunTrace(shell Shell) (io.ReadCloser, error) {
	cmd := exec.Command("sh", "-c", shell.GetTraceCommand())
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "PS4="+shell.GetPS4())

	// We only care about stderr for the trace
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// We don't wait for the command here because we need to stream the output.
	// The caller is responsible for reading stderr until EOF.
	// Note: This leaves the process running until it exits or stderr is closed.
	// Since the command is `exit`, it should finish quickly after dumping init logs.

	// However, exec.Command doesn't make it easy to wait *after* returning the pipe.
	// We might need a wrapper logic if we want to ensure cleanup, but for now
	// let's rely on the read loop ending.

	return stderr, nil
}

// RunTraceSync is a helper to run and collect all output (for testing/debugging)
func RunTraceSync(shell Shell) ([]string, error) {
	stderr, err := RunTrace(shell)
	if err != nil {
		return nil, err
	}
	defer stderr.Close()

	var lines []string
	scanner := bufio.NewScanner(stderr)
	// Increase buffer size in case of very long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
