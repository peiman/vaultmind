package hookscripts_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A project installed with the knowledge profile has a knowledge base, not an
// agent's identity. At compaction it is asked what the segment learned about
// the code — in the vault it configured, even one scaffolded with arcs/ — and
// nothing about a desk, an identity or what changed in the agent. Found by
// running a fresh knowledge-profile install as a stranger: it got the whole
// persona speech and a journal/ written into its docs vault, and no code step.
func TestPrecompact_AKnowledgeProjectIsAskedOnlyAboutTheCode(t *testing.T) {
	project := t.TempDir()
	kb := filepath.Join(project, "docs-kb")
	require.NoError(t, os.MkdirAll(filepath.Join(kb, "arcs"), 0o750), "init's scaffold has arcs/; the profile still decides")
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".claude"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(project, ".claude", "vaultmind-profile"), []byte("knowledge\n"), 0o600))
	env := []string{
		"PATH=/usr/bin:/bin",
		"HOME=" + t.TempDir(),
		"CLAUDE_PROJECT_DIR=" + project,
		"VAULTMIND_VAULT=" + kb,
	}

	msg := precompactMessage(t, env)
	assert.Contains(t, msg, learnedStep)
	assert.Contains(t, msg, "add a note to "+kb, "the configured vault is the knowledge vault")
	assert.Contains(t, msg, "vaultmind ask", "search before writing")
	for _, persona := range []string{"desk entry", "identity-who-i-am", "CHANGED IN YOU", "journal/"} {
		assert.NotContains(t, msg, persona)
	}
}

// The full profile — this project's own — keeps the desk and identity steps.
func TestPrecompact_TheFullProfileKeepsTheDesk(t *testing.T) {
	e := newPrecompactEnv(t, true)
	msg := precompactMessage(t, e.env)
	assert.Contains(t, msg, "desk entry")
	assert.Contains(t, msg, "identity-who-i-am")
}
