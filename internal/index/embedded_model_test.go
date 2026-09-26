package index_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// EmbeddedModel names the model a vault's embeddings were made with, so a write
// can embed the new note the same way instead of mixing models in one index.

func embeddedModelDB(t *testing.T, rows ...string) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	require.NotEmpty(t, rows, "a vault with no notes proves nothing")
	for i, set := range rows {
		_, err := db.Exec(`INSERT INTO notes (id, path, hash, mtime) VALUES (?, ?, 'h', 0)`,
			string(rune('a'+i)), string(rune('a'+i))+".md")
		require.NoError(t, err)
		if set != "" {
			_, err = db.Exec(`UPDATE notes SET `+set+` WHERE id = ?`, string(rune('a'+i)))
			require.NoError(t, err)
		}
	}
	return dbPath
}

func TestEmbeddedModel_NeverEmbeddedIsEmpty(t *testing.T) {
	model, err := index.EmbeddedModel(embeddedModelDB(t, "", ""))
	require.NoError(t, err)
	assert.Empty(t, model, "a vault never embedded is not embedded on write either")
}

func TestEmbeddedModel_DenseOnlyIsMiniLM(t *testing.T) {
	model, err := index.EmbeddedModel(embeddedModelDB(t, "embedding = x'00'", ""))
	require.NoError(t, err)
	assert.Equal(t, embedding.ModelMiniLM, model)
}

func TestEmbeddedModel_SparseOrColBERTIsBGEM3(t *testing.T) {
	model, err := index.EmbeddedModel(embeddedModelDB(t,
		"embedding = x'00', sparse_embedding = x'00', colbert_embedding = x'00'", ""))
	require.NoError(t, err)
	assert.Equal(t, embedding.ModelBGEM3, model)
}
