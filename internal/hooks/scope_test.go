package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserScope_LegacyHint(t *testing.T) {
	home := t.TempDir()
	assert.False(t, UserScopeInstalled(""))
	assert.False(t, UserScopeInstalled(home))
	settings := filepath.Join(home, ".claude", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(settings), 0o750))
	require.NoError(t, os.WriteFile(settings, []byte(`{"hooks":{"SessionStart":[{"hooks":[{"command":"bash /old/scripts/load-persona.sh"}]}]}}`), 0o600))
	assert.True(t, UserScopeInstalled(home), "wiring alone still indicates an install, even if scripts were deleted")
}

func TestUserScope_MergeErrorsPreserveSettings(t *testing.T) {
	for _, agent := range []Agent{AgentCursor, AgentCodex} {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
			path := CursorHooksPath(home)
			if agent == AgentCodex {
				path = UserCodexHooksPath()
			}
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
			require.NoError(t, os.WriteFile(path, []byte("not JSON"), 0o600))
			_, err := MergeUserAgentHooks(home, "", nil, ProfileKnowledge, agent, false)
			require.ErrorContains(t, err, "merging into")
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, "not JSON", string(after))
			require.NoError(t, os.Remove(path))
			require.NoError(t, os.Mkdir(path, 0o750))
			_, err = MergeUserAgentHooks(home, "", nil, ProfileKnowledge, agent, false)
			require.ErrorContains(t, err, "reading")
		})
	}
}

func TestUserScope_ProfileErrors(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, ".vaultmind", profileFilename)
	require.NoError(t, os.MkdirAll(filepath.Dir(profile), 0o750))
	require.NoError(t, os.WriteFile(profile, []byte("typo"), 0o600))
	_, err := InstallProfileForScope(home, ScopeUser)
	require.ErrorContains(t, err, "unknown vaultmind profile")
	_, err = StatusForScope(home, ScopeUser)
	require.ErrorContains(t, err, "unknown vaultmind profile")
}

func TestUserScope_CursorStanza(t *testing.T) {
	home := t.TempDir()
	stanza, err := UserAgentStanza(home, "/vault", nil, ProfileKnowledge, AgentCursor)
	require.NoError(t, err)
	assert.Contains(t, stanza, filepath.Join(home, ".vaultmind", "scripts", cursorAdapterScript))
	assert.NotContains(t, stanza, "export VAULTMIND_PROJECT_DIR=")
}
