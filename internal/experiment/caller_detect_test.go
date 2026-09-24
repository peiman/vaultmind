package experiment

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectCaller_ExplicitEnvVarWins(t *testing.T) {
	t.Setenv("VAULTMIND_CALLER", "companion-persona-hook")
	t.Setenv("CLAUDE_PROJECT_DIR", "/should/be/ignored/when/explicit")

	caller, _ := DetectCaller()
	assert.Equal(t, "companion-persona-hook", caller)
}

func TestDetectCaller_FallsBackToClaudeCodeWhenProjectDirSet(t *testing.T) {
	t.Setenv("VAULTMIND_CALLER", "")
	t.Setenv("CLAUDE_PROJECT_DIR", "/Users/me/dev/vaultmind")

	caller, meta := DetectCaller()
	assert.Equal(t, "claude-code", caller)
	assert.Equal(t, "/Users/me/dev/vaultmind", meta["claude_project_dir"])
}

func TestDetectCaller_DefaultsToCLI(t *testing.T) {
	t.Setenv("VAULTMIND_CALLER", "")
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	caller, _ := DetectCaller()
	assert.Equal(t, "cli", caller)
}

func TestDetectCaller_MetaCapturesUserAndHost(t *testing.T) {
	t.Setenv("VAULTMIND_CALLER", "cli")
	t.Setenv("USER", "someuser")

	_, meta := DetectCaller()
	assert.Equal(t, "someuser", meta["user"])
	// hostname is os-provided; just check it's populated.
	require.Contains(t, meta, "host")
	assert.NotEmpty(t, meta["host"])
}

func TestDetectCaller_MetaOmitsMissingFields(t *testing.T) {
	t.Setenv("VAULTMIND_CALLER", "cli")
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	_, meta := DetectCaller()
	_, hasPD := meta["claude_project_dir"]
	assert.False(t, hasPD, "empty CLAUDE_PROJECT_DIR should not appear in meta")
}

func TestDetectCaller_CarriesTheHarnessSessionIDWhenPresent(t *testing.T) {
	t.Setenv("VAULTMIND_USER_SESSION_ID", "conv-from-harness")
	_, meta := DetectCaller()
	require.Equal(t, "conv-from-harness", meta[MetaUserSessionID],
		"the real conversation id must reach the session row, not be re-derived from timing")
}

// clearHarnessSessionIDs isolates a test from the shell running it: under
// Claude Code or Codex the agent's own shell carries a real session id.
func clearHarnessSessionIDs(t *testing.T) {
	t.Helper()
	for _, k := range []string{EnvUserSessionID, EnvClaudeCodeSessionID, EnvCodexSessionID} {
		t.Setenv(k, "")
	}
}

func TestDetectCaller_OmitsTheKeyWhenUnset(t *testing.T) {
	clearHarnessSessionIDs(t)
	_, meta := DetectCaller()
	_, present := meta[MetaUserSessionID]
	require.False(t, present, "absent means fall back to the heuristic, not group under an empty string")
}

// The agent's own CLI calls (a `note get` after recall showed a pointer) must
// land in the SAME user session as the hooks' searches, or "shown, then used"
// cannot be observed at all. Measured 2026-09-24: 0 of 654 recall-hook sessions
// shared an id with the agent's reads; every usage-learning arm of the
// plasticity replay was starved by it. Claude Code and Codex both expose the
// conversation id to the agent's shell.
func TestDetectCaller_UsesClaudeCodesSessionIDFromTheShell(t *testing.T) {
	clearHarnessSessionIDs(t)
	t.Setenv(EnvClaudeCodeSessionID, "cc-conv")
	_, meta := DetectCaller()
	assert.Equal(t, "cc-conv", meta[MetaUserSessionID])
}

func TestDetectCaller_UsesCodexsSessionIDFromTheShell(t *testing.T) {
	clearHarnessSessionIDs(t)
	t.Setenv(EnvCodexSessionID, "codex-root")
	_, meta := DetectCaller()
	assert.Equal(t, "codex-root", meta[MetaUserSessionID])
}

// A hook forwards the id from its payload explicitly; that wins over whatever
// the process environment happens to carry.
func TestDetectCaller_AForwardedIDWinsOverTheShells(t *testing.T) {
	clearHarnessSessionIDs(t)
	t.Setenv(EnvUserSessionID, "from-payload")
	t.Setenv(EnvClaudeCodeSessionID, "cc-conv")
	t.Setenv(EnvCodexSessionID, "codex-root")
	_, meta := DetectCaller()
	assert.Equal(t, "from-payload", meta[MetaUserSessionID])
}
