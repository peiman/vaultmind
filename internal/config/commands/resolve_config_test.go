package commands_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/stretchr/testify/assert"
)

func TestResolveMetadata_Fields(t *testing.T) {
	meta := commands.ResolveMetadata
	assert.Contains(t, meta.Use, "resolve")
	assert.NotEmpty(t, meta.Short)
	assert.Equal(t, "app.resolve", meta.ConfigPrefix)
}
