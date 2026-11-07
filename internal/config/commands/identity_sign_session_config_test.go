package commands_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdentitySignSessionMetadata_Fields(t *testing.T) {
	meta := commands.IdentitySignSessionMetadata
	assert.Equal(t, "sign-session", meta.Use)
	assert.NotEmpty(t, meta.Short)
	assert.NotEmpty(t, meta.Long)
	assert.Equal(t, "app.identitysignsession", meta.ConfigPrefix)
	assert.NotContains(t, meta.Long, "--slug")
	assert.NotContains(t, meta.Long, "--key-epoch")
}

func TestIdentitySignSessionMetadata_FlagOverridesMapToOptions(t *testing.T) {
	opts := commands.IdentitySignSessionOptions()
	optKeys := map[string]bool{}
	for _, o := range opts {
		optKeys[o.Key] = true
		assert.Equal(t, "string", o.Type)
		assert.Equal(t, "", o.DefaultValue)
		assert.NotEmpty(t, o.Description)
	}
	assert.Equal(t, map[string]bool{
		"app.identitysignsession.challenge":     true,
		"app.identitysignsession.network_id":    true,
		"app.identitysignsession.signer_socket": true,
	}, optKeys)

	seen := map[string]bool{}
	for key, flag := range commands.IdentitySignSessionMetadata.FlagOverrides {
		assert.Truef(t, optKeys[key], "flag override key %q has no matching option", key)
		assert.Falsef(t, seen[flag], "duplicate flag name %q", flag)
		seen[flag] = true
	}
	assert.Len(t, commands.IdentitySignSessionMetadata.FlagOverrides, len(opts))
	require.Equal(t, "challenge", commands.IdentitySignSessionMetadata.FlagOverrides["app.identitysignsession.challenge"])
	require.Equal(t, "network-id", commands.IdentitySignSessionMetadata.FlagOverrides["app.identitysignsession.network_id"])
	require.Equal(t, "signer-socket", commands.IdentitySignSessionMetadata.FlagOverrides["app.identitysignsession.signer_socket"])
}
