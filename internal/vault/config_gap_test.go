package vault_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A gap is a note type. The index and the vault cannot import each other, so
// this test is the lock that the two spellings stay the same string.
func TestGapType_MatchesTheIndex(t *testing.T) {
	assert.Equal(t, index.TypeGap, vault.GapType)
}

// A vault that already defines types gets gap in memory. The file is not
// rewritten. A gap is not citable evidence.
func TestLoadConfig_GapIsABuiltInType(t *testing.T) {
	dir := writeConfig(t, "types:\n  concept:\n    required: [title]\n")
	before, err := os.ReadFile(filepath.Join(dir, ".vaultmind", "config.yaml"))
	require.NoError(t, err)
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	after, err := os.ReadFile(filepath.Join(dir, ".vaultmind", "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, before, after, "registering gap must not rewrite config.yaml")

	def, ok := cfg.Types[vault.GapType]
	require.True(t, ok)
	assert.Equal(t, []string{"title", "question"}, def.Required)
	assert.Equal(t, []string{"open", "closed"}, def.Statuses)
	require.NotNil(t, def.Authoritative)
	assert.False(t, *def.Authoritative)
	assert.Empty(t, def.Template)
	assert.NotContains(t, def.Statuses, "draft")
}

// A vault's own gap definition is kept as written.
func TestLoadConfig_AVaultsOwnGapTypeWins(t *testing.T) {
	cfg, err := vault.LoadConfig(writeConfig(t, "types:\n  gap:\n    required: [title]\n    statuses: [open]\n"))
	require.NoError(t, err)
	assert.Equal(t, []string{"title"}, cfg.Types["gap"].Required)
	assert.Equal(t, []string{"open"}, cfg.Types["gap"].Statuses)
}

// A vault that defines no types stays unvalidated. note gap then names the fix.
func TestLoadConfig_NoTypesDoesNotInventGap(t *testing.T) {
	cfg, err := vault.LoadConfig(writeConfig(t, "vault:\n  exclude: [\".git\"]\n"))
	require.NoError(t, err)
	assert.Empty(t, cfg.Types)
}
