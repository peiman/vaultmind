package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectCaller_ExplicitEnvVarWins(t *testing.T) {
	t.Setenv("VAULTMIND_CALLER", "workhorse-persona-hook")
	t.Setenv("CLAUDE_PROJECT_DIR", "/should/be/ignored/when/explicit")

	caller, _ := detectCaller()
	assert.Equal(t, "workhorse-persona-hook", caller)
}

func TestDetectCaller_FallsBackToClaudeCodeWhenProjectDirSet(t *testing.T) {
	t.Setenv("VAULTMIND_CALLER", "")
	t.Setenv("CLAUDE_PROJECT_DIR", "/Users/peiman/dev/vaultmind")

	caller, meta := detectCaller()
	assert.Equal(t, "claude-code", caller)
	assert.Equal(t, "/Users/peiman/dev/vaultmind", meta["claude_project_dir"])
}

func TestDetectCaller_DefaultsToCLI(t *testing.T) {
	t.Setenv("VAULTMIND_CALLER", "")
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	caller, _ := detectCaller()
	assert.Equal(t, "cli", caller)
}

func TestDetectCaller_MetaCapturesUserAndHost(t *testing.T) {
	t.Setenv("VAULTMIND_CALLER", "cli")
	t.Setenv("USER", "siavoush")

	_, meta := detectCaller()
	assert.Equal(t, "siavoush", meta["user"])
	// hostname is os-provided; just check it's populated.
	require.Contains(t, meta, "host")
	assert.NotEmpty(t, meta["host"])
}

func TestDetectCaller_MetaOmitsMissingFields(t *testing.T) {
	t.Setenv("VAULTMIND_CALLER", "cli")
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	_, meta := detectCaller()
	_, hasPD := meta["claude_project_dir"]
	assert.False(t, hasPD, "empty CLAUDE_PROJECT_DIR should not appear in meta")
}
