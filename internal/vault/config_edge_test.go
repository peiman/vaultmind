package vault_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LoadConfig must fall back to the default exclude list when the user
// provides a config that has an empty vault.exclude field. Losing the
// defaults would walk into .obsidian/.git/.trash directories — a user
// surprise that silently inflates the index with junk.
//
// An explicit `exclude: []` YAML overrides the pre-populated defaults with
// an empty slice — that's the post-unmarshal branch (line 82) we need to
// cover. An absent `vault:` section leaves the defaults in place, which
// doesn't exercise that branch.
func TestLoadConfig_ExplicitEmptyExcludeFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".vaultmind")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(`
vault:
  exclude: []
types:
  concept:
    required: [title]
`), 0o644))

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.Vault.Exclude,
		"explicit empty exclude must still fall back to the default list")
	want := map[string]bool{".git": true, ".obsidian": true, ".trash": true, "node_modules": true}
	got := map[string]bool{}
	for _, e := range cfg.Vault.Exclude {
		got[e] = true
	}
	for k := range want {
		assert.True(t, got[k], "default excludes must include %q after fallback", k)
	}
}

// LoadConfig must return a structured "reading config" error for
// permission / IO failures distinct from "not exist". Callers can't
// retry or branch meaningfully without knowing which.
func TestLoadConfig_UnreadablePathReturnsReadError(t *testing.T) {
	// Create a vault config where the config file is a directory (not a
	// regular file) — reading it will fail with EISDIR on most systems.
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".vaultmind")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "config.yaml"), 0o755))

	_, err := vault.LoadConfig(dir)
	// On macOS/Linux, os.ReadFile on a directory returns EISDIR. That's
	// neither os.IsNotExist nor a YAML error — it must surface via the
	// "reading config" branch.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading config",
		"IO failure distinct from 'not exist' must use the reading-config error path")
}
