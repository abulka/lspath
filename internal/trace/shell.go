package trace

import (
	"strings"
)

// Shell defines the interface for shell-specific tracing commands.
type Shell interface {
	GetTraceCommand() string
	GetPS4() string
	Name() string
}

// ZshShell implements Shell for Zsh.
type ZshShell struct{}

func (s *ZshShell) GetTraceCommand() string {
	return "zsh -xli -c exit"
}

func (s *ZshShell) GetPS4() string {
	// Format: + file:line>command
	return "+ %x:%I>"
}

func (s *ZshShell) Name() string {
	return "zsh"
}

// BashShell implements Shell for Bash.
type BashShell struct{}

func (s *BashShell) GetTraceCommand() string {
	return "bash -xli -c exit"
}

func (s *BashShell) GetPS4() string {
	// Format: +file:line>command
	return "+${BASH_SOURCE}:${LINENO}>"
}

func (s *BashShell) Name() string {
	return "bash"
}

// DetectShell attempts to identify the user's shell or defaults to Zsh.
// DetectShell attempts to identify the user's shell or defaults to Zsh.
func DetectShell(shellPath string) Shell {
	// Check for "bash" in the path or name
	if strings.Contains(shellPath, "bash") {
		return &BashShell{}
	}
	// Default to Zsh as it's the specific request target, and macOS default.
	return &ZshShell{}
}
