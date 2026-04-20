package experiment

import (
	"os"
)

// DetectCaller resolves the caller identity for a new experiment session.
// Precedence:
//  1. VAULTMIND_CALLER env var — explicit, wins unconditionally. Hooks and
//     scripts set this ("workhorse-persona-hook", "vaultmind-persona-hook").
//  2. CLAUDE_PROJECT_DIR set — we were invoked from a Claude Code session,
//     probably by the agent running bash. Label as "claude-code" so we can
//     distinguish from raw CLI use.
//  3. Default to "cli" — a human at a terminal, or an unknown script.
//
// The companion caller_meta captures $USER, $HOSTNAME, and CLAUDE_PROJECT_DIR
// so we can later answer "whose laptop" and "which project context" without
// having to encode them into the caller label itself.
func DetectCaller() (string, map[string]any) {
	caller := os.Getenv("VAULTMIND_CALLER")
	if caller == "" {
		if os.Getenv("CLAUDE_PROJECT_DIR") != "" {
			caller = "claude-code"
		} else {
			caller = "cli"
		}
	}

	meta := make(map[string]any, 3)
	if user := os.Getenv("USER"); user != "" {
		meta["user"] = user
	}
	if host, err := os.Hostname(); err == nil && host != "" {
		meta["host"] = host
	}
	if projectDir := os.Getenv("CLAUDE_PROJECT_DIR"); projectDir != "" {
		meta["claude_project_dir"] = projectDir
	}
	return caller, meta
}
