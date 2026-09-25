package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A knowledge-vault project installed with --profile knowledge got only the
// knowledge scripts written, but the settings merge wired every canonical
// hook — persona loading and episode capture included — pointing at scripts
// the profile had just declined to write. Found upgrading a copy of focalc
// (2026-09-25). The Codex path already filtered by profile; Claude's did not.
func TestProvision_WiresOnlyTheProfilesHooks(t *testing.T) {
	dir := t.TempDir()
	_, err := Provision(InstallConfig{ProjectDir: dir, Profile: ProfileKnowledge}, true, false, false)
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)
	s := string(raw)
	for _, want := range []string{hookHealthScript, hookUserPromptSubmitScript, hookPreToolUseScript, hookReachScript} {
		assert.Contains(t, s, want)
	}
	for _, not := range []string{hookSessionStartScript, hookSessionEndScript, hookPreCompactScript} {
		assert.NotContains(t, s, not, "a knowledge profile must not wire %s", not)
	}
}
