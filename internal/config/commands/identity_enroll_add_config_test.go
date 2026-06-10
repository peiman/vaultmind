package commands_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIdentityEnrollAddMetadata_Fields pins the command metadata.
func TestIdentityEnrollAddMetadata_Fields(t *testing.T) {
	meta := commands.IdentityEnrollAddMetadata
	assert.Equal(t, "enroll-add", meta.Use)
	assert.NotEmpty(t, meta.Short)
	assert.NotEmpty(t, meta.Long)
	assert.Equal(t, "app.identityenrolladd", meta.ConfigPrefix)
}

// TestIdentityEnrollAddMetadata_FlagOverridesMapToOptions proves every flag
// override targets an existing option key and a distinct flag name (no typos /
// collisions), and that every option is user-facing (has a flag override).
func TestIdentityEnrollAddMetadata_FlagOverridesMapToOptions(t *testing.T) {
	opts := commands.IdentityEnrollAddOptions()
	optKeys := map[string]bool{}
	for _, o := range opts {
		optKeys[o.Key] = true
	}

	seenFlags := map[string]bool{}
	for key, flag := range commands.IdentityEnrollAddMetadata.FlagOverrides {
		assert.Truef(t, optKeys[key], "flag override key %q has no matching option", key)
		assert.NotEmpty(t, flag)
		assert.Falsef(t, seenFlags[flag], "duplicate flag name %q", flag)
		seenFlags[flag] = true
	}
	assert.Len(t, commands.IdentityEnrollAddMetadata.FlagOverrides, len(opts))
}

// TestIdentityEnrollAddOptions_ShapeAndDefaults asserts each option's key/type
// and default, covering every option entry.
func TestIdentityEnrollAddOptions_ShapeAndDefaults(t *testing.T) {
	opts := commands.IdentityEnrollAddOptions()
	require.Len(t, opts, 6)

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
		"app.identityenrolladd.request",
		"app.identityenrolladd.registry",
		"app.identityenrolladd.root_pubkey",
		"app.identityenrolladd.network_id",
		"app.identityenrolladd.validity_seconds",
		"app.identityenrolladd.origin_daemon",
	}
	for _, k := range wantStrings {
		got, ok := byKey[k]
		require.Truef(t, ok, "missing option %q", k)
		assert.Equal(t, "string", got.typ, "option %q type", k)
		assert.Equal(t, "", got.def, "option %q default", k)
	}
}
