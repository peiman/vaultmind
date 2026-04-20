package query_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctor_ReportsNoEmbeddingsStatus(t *testing.T) {
	dbPath := t.TempDir() + "/idx.db"
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Seed a couple of notes, no embeddings.
	for i, id := range []string{"n1", "n2"} {
		_, err := db.Exec(
			`INSERT INTO notes (id, path, hash, mtime, title, is_domain) VALUES (?, ?, ?, ?, ?, ?)`,
			id, "n"+string(rune('0'+i))+".md", "hash", 0, "Title "+id, true,
		)
		require.NoError(t, err)
	}

	result, err := query.Doctor(db, "/vault")
	require.NoError(t, err)
	require.NotNil(t, result.Embeddings)
	assert.Equal(t, 2, result.Embeddings.TotalNotes)
	assert.Equal(t, 0, result.Embeddings.DenseCount)
	assert.Equal(t, 0, result.Embeddings.SparseCount)
	assert.Equal(t, 0, result.Embeddings.ColBERTCount)
	assert.Empty(t, result.Embeddings.Model)
	assert.False(t, result.Embeddings.SemanticReady, "no dense embeddings → no semantic retrieval")
}

func TestDoctor_ReportsDenseMiniLMEmbeddings(t *testing.T) {
	dbPath := t.TempDir() + "/idx.db"
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Insert note with a MiniLM-sized embedding (384 float32 = 1536 bytes).
	vec := make([]float32, 384)
	_, err = db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, title, is_domain, embedding) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"n1", "n1.md", "h", 0, "T", true, index.EncodeEmbedding(vec),
	)
	require.NoError(t, err)

	result, err := query.Doctor(db, "/vault")
	require.NoError(t, err)
	require.NotNil(t, result.Embeddings)
	assert.Equal(t, 1, result.Embeddings.DenseCount)
	assert.Equal(t, "minilm", result.Embeddings.Model)
	assert.True(t, result.Embeddings.SemanticReady)
}

func TestDoctor_ReportsBGEM3Embeddings(t *testing.T) {
	dbPath := t.TempDir() + "/idx.db"
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Insert note with a BGE-M3-sized embedding (1024 float32 = 4096 bytes).
	vec := make([]float32, 1024)
	_, err = db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, title, is_domain, embedding) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"n1", "n1.md", "h", 0, "T", true, index.EncodeEmbedding(vec),
	)
	require.NoError(t, err)

	result, err := query.Doctor(db, "/vault")
	require.NoError(t, err)
	require.NotNil(t, result.Embeddings)
	assert.Equal(t, "bge-m3", result.Embeddings.Model)
	assert.True(t, result.Embeddings.SemanticReady)
}
