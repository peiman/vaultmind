package commands_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexMetadata_Fields(t *testing.T) {
	meta := commands.IndexMetadata
	assert.Equal(t, "index", meta.Use)
	assert.NotEmpty(t, meta.Short)
	assert.NotEmpty(t, meta.Long)
	assert.Equal(t, "app.index", meta.ConfigPrefix)
}

func TestIndexOptions_ContainsVaultFlag(t *testing.T) {
	opts := commands.IndexOptions()
	require.NotEmpty(t, opts)

	var keys []string
	for _, o := range opts {
		keys = append(keys, o.Key)
	}
	assert.Contains(t, keys, "app.index.vault")
}

func TestIndexOptions_JSONDefaultFalse(t *testing.T) {
	for _, opt := range commands.IndexOptions() {
		if opt.Key == "app.index.json" {
			assert.Equal(t, false, opt.DefaultValue)
			return
		}
	}
	t.Fatal("app.index.json option not found")
}
