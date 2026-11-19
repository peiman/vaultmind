package commands_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdentitySignHealthMetadata_Fields(t *testing.T) {
	meta := commands.IdentitySignHealthMetadata
	assert.Equal(t, "sign-health", meta.Use)
	assert.NotEmpty(t, meta.Short)
	assert.NotEmpty(t, meta.Long)
	assert.Equal(t, "app.identitysignhealth", meta.ConfigPrefix)
}

func TestIdentitySignHealthOptions_Shape(t *testing.T) {
	opts := commands.IdentitySignHealthOptions()
	require.Len(t, opts, 1)
	assert.Equal(t, "app.identitysignhealth.max_age", opts[0].Key)
	assert.Equal(t, "0", opts[0].DefaultValue)
	assert.Equal(t, "string", opts[0].Type)
	assert.Equal(t, "max-age", commands.IdentitySignHealthMetadata.FlagOverrides[opts[0].Key])
}
