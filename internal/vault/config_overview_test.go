package vault_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A folder overview is a note of type overview, valid in every vault that
// defines types without anyone editing its config.
func TestLoadConfig_OverviewIsABuiltInType(t *testing.T) {
	cfg, err := vault.LoadConfig(writeConfig(t, "types:\n  concept:\n    required: [title]\n"))
	require.NoError(t, err)
	def, ok := cfg.Types["overview"]
	require.True(t, ok)
	assert.Equal(t, []string{"title"}, def.Required)
}

// A vault's own overview definition is kept as written.
func TestLoadConfig_AVaultsOwnOverviewTypeWins(t *testing.T) {
	cfg, err := vault.LoadConfig(writeConfig(t, "types:\n  overview:\n    required: [title, status]\n"))
	require.NoError(t, err)
	assert.Equal(t, []string{"title", "status"}, cfg.Types["overview"].Required)
}

// A vault that defines no types stays unvalidated: adding one would make
// every other type unknown.
func TestLoadConfig_NoTypesStaysNoTypes(t *testing.T) {
	cfg, err := vault.LoadConfig(writeConfig(t, "vault:\n  exclude: [\".git\"]\n"))
	require.NoError(t, err)
	assert.Empty(t, cfg.Types)
}
