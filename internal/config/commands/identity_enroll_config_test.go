package commands_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIdentityEnrollMetadata_Fields pins the command metadata.
func TestIdentityEnrollMetadata_Fields(t *testing.T) {
	meta := commands.IdentityEnrollMetadata
	assert.Equal(t, "enroll", meta.Use)
	assert.NotEmpty(t, meta.Short)
	assert.NotEmpty(t, meta.Long)
	assert.Equal(t, "app.identityenroll", meta.ConfigPrefix)
}

// TestIdentityEnrollMetadata_FlagOverridesMapToOptions proves every flag override
// targets an existing option key and a distinct flag name (no typos / collisions).
func TestIdentityEnrollMetadata_FlagOverridesMapToOptions(t *testing.T) {
	opts := commands.IdentityEnrollOptions()
	optKeys := map[string]bool{}
	for _, o := range opts {
		optKeys[o.Key] = true
	}

	seenFlags := map[string]bool{}
	for key, flag := range commands.IdentityEnrollMetadata.FlagOverrides {
		assert.Truef(t, optKeys[key], "flag override key %q has no matching option", key)
		assert.NotEmpty(t, flag)
		assert.Falsef(t, seenFlags[flag], "duplicate flag name %q", flag)
		seenFlags[flag] = true
	}
	// Every option must have a flag override (each is user-facing).
	assert.Len(t, commands.IdentityEnrollMetadata.FlagOverrides, len(opts))
}

// TestIdentityEnrollOptions_ShapeAndDefaults asserts each option's key/type and
// the required defaults, covering every option entry.
func TestIdentityEnrollOptions_ShapeAndDefaults(t *testing.T) {
	opts := commands.IdentityEnrollOptions()
	require.Len(t, opts, 8)

	byKey := map[string]struct {
		def any
		typ string
	}{}
	for _, o := range opts {
		assert.NotEmpty(t, o.Description, "option %q must describe itself", o.Key)
		byKey[o.Key] = struct {
			def any
			typ string
		}{o.DefaultValue, o.Type}
	}

	wantStrings := []string{
		"app.identityenroll.invite",
		"app.identityenroll.display_name",
		"app.identityenroll.slug",
		"app.identityenroll.pubkey",
		"app.identityenroll.transport_pubkey",
		"app.identityenroll.transport_endpoint",
		"app.identityenroll.signer_socket",
	}
	for _, k := range wantStrings {
		got, ok := byKey[k]
		require.Truef(t, ok, "missing option %q", k)
		assert.Equal(t, "string", got.typ, "option %q type", k)
		assert.Equal(t, "", got.def, "option %q default", k)
	}

	yes, ok := byKey["app.identityenroll.yes"]
	require.True(t, ok, "missing --yes option")
	assert.Equal(t, "bool", yes.typ)
	assert.Equal(t, false, yes.def)
}
