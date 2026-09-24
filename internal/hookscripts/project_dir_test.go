package hookscripts

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The scripts serve Claude Code AND Codex. Claude Code sets CLAUDE_PROJECT_DIR
// for every hook; Codex sets nothing, so its hooks export the agent-neutral
// VAULTMIND_PROJECT_DIR. Any line that reads CLAUDE_PROJECT_DIR must consult
// VAULTMIND_PROJECT_DIR first on the same line — otherwise a Codex hook would
// fall through to the working directory, or to a Claude path it never had.
func TestScripts_ReadTheNeutralProjectDirFirst(t *testing.T) {
	var checked int
	for _, name := range Names() {
		body, ok := Get(name)
		require.True(t, ok)
		for i, line := range strings.Split(string(body), "\n") {
			code := strings.TrimSpace(line)
			if strings.HasPrefix(code, "#") || !strings.Contains(code, "CLAUDE_PROJECT_DIR") {
				continue
			}
			checked++
			v := strings.Index(code, "VAULTMIND_PROJECT_DIR")
			c := strings.Index(code, "CLAUDE_PROJECT_DIR")
			assert.Truef(t, v >= 0 && v < c, "%s:%d reads CLAUDE_PROJECT_DIR without VAULTMIND_PROJECT_DIR first:\n  %s", name, i+1, code)
		}
	}
	assert.Positive(t, checked, "the guard must have something to check")
}

// Every hook call into vaultmind that names its caller must also forward the
// conversation id from the hook payload. Without it the call lands in a
// time-guessed session, apart from the agent's own reads — measured
// 2026-09-24: 0 of 654 recall-hook sessions shared an id with the agent's
// reads, which starved every usage-learning arm of the plasticity replay.
func TestScripts_HookCallsForwardTheSessionID(t *testing.T) {
	var checked int
	for _, name := range Names() {
		body, ok := Get(name)
		require.True(t, ok)
		for i, line := range strings.Split(string(body), "\n") {
			code := strings.TrimSpace(line)
			if strings.HasPrefix(code, "#") || !strings.Contains(code, "VAULTMIND_CALLER=") {
				continue
			}
			checked++
			assert.Containsf(t, code, "VAULTMIND_USER_SESSION_ID=",
				"%s:%d calls vaultmind without forwarding the session id:\n  %s", name, i+1, code)
		}
	}
	assert.Positive(t, checked)
}
