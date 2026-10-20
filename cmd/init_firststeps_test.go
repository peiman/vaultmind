package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/onboard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// init's next steps are the README Quickstart commands, in the same order,
// for the vault just made.
func TestInit_NextStepsMatchTheQuickstart(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	vault := filepath.Join(t.TempDir(), "kb")
	out, _, err := runRootCmd(t, "init", vault)
	require.NoError(t, err)
	text := out.String()

	prev := -1
	for _, step := range onboard.FirstSteps(vault) {
		i := strings.Index(text, "  "+step)
		require.Greater(t, i, prev, step)
		prev = i
	}
	assert.Contains(t, text, onboard.ShellExpandSentence)
	assert.NotContains(t, text, "init --wire-hooks")
	assert.Contains(t, text, "--agent codex --merge")
	assert.NotContains(t, text, "who am I")
}

func TestInit_PersonaNextSteps(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	vault := filepath.Join(t.TempDir(), "persona")
	out, _, err := runRootCmd(t, "init", vault, "--profile", "persona")
	require.NoError(t, err)
	text := out.String()
	assert.NotContains(t, text, "vaultmind import")
	assert.NotContains(t, text, "shell expands")
	assert.NotContains(t, text, "init --wire-hooks")
	prev := -1
	for _, step := range []string{
		"vaultmind index --embed --vault " + vault,
		"vaultmind ask \"who am I\" --vault " + vault,
		"vaultmind hooks install . --vault " + vault + " --profile persona --merge",
	} {
		i := strings.Index(text, "  "+step)
		require.Greater(t, i, prev, step)
		prev = i
	}
}

func TestInit_FullNextStepsPreserveTheHookProfile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	vault := filepath.Join(t.TempDir(), "full")
	out, _, err := runRootCmd(t, "init", vault, "--profile", "full")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "vaultmind hooks install . --vault "+vault+" --profile full --merge")
	assert.NotContains(t, out.String(), "--profile persona")
}
