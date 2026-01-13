package trace

import (
	"bufio"
	"io"
	"os"
	"os/exec"
)

// Define the baseline path here.
// Export it so main.go or the analyzer can see it if needed.
// This is a subtle point about how your tool works:
// The Tool Creates a Sandbox: In executor.go, your tool explicitly strips the user's existing PATH and forces PATH=/usr/bin:/bin.... It does this to create a "clean slate" so it can see exactly how the config files reconstruct the path.
// It IS Hardcoded (by design): Currently, your executor.go does hardcode these paths (line 38 of the file you sent).
// Is this bad?
// Yes: If you run this on a system where /bin doesn't exist (like some distinct NixOS setups or Windows), the shell might fail to find basic commands like rm or mkdir.
// No: For 99% of macOS and Linux systems, these paths are standard.
// Better approach for the Executor:
// Instead of hardcoding /usr/bin..., the executor could technically capture the system default path (confstr _CS_PATH on POSIX), but that is hard to get reliably from Go without CGO.
const SandboxInitialPath = "/usr/bin:/bin:/usr/sbin:/sbin"

// RunTrace executes the shell trace command and returns the stderr pipe.
func RunTrace(shell Shell) (io.ReadCloser, error) {
	cmd := exec.Command("sh", "-c", shell.GetTraceCommand())
	// Sanitize Environment:
	// We want to trace how the PATH is constructed from configuration files,
	// so we remove the inherited PATH to prevent the first script from being
	// incorrectly attributed with all existing entries.
	var env []string
	for _, e := range os.Environ() {
		// Filter out PATH, keeping others (TERM, USER, etc.)
		if len(e) >= 5 && e[:5] == "PATH=" {
			continue
		}
		env = append(env, e)
	}
	// Set a minimal standard PATH to ensure basic shell tools (like rm, mkdir, zsh itself) work.
	// This forces the shell startup scripts to reconstruct the full user PATH.
	env = append(env, "PATH="+SandboxInitialPath)

	cmd.Env = env
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
