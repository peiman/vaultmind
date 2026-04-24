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

// HasModalityImbalance must be false when a vault is MiniLM-only: sparse and
// colbert don't apply to that model, so missing them is by design, not a bug.
func TestDoctor_MiniLM_NoImbalanceReported(t *testing.T) {
	dbPath := t.TempDir() + "/idx.db"
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	vec := make([]float32, 384) // MiniLM dims
	_, err = db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, title, is_domain, embedding) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"n1", "n1.md", "h", 0, "T", true, index.EncodeEmbedding(vec),
	)
	require.NoError(t, err)

	result, err := query.Doctor(db, "/vault")
	require.NoError(t, err)
	require.NotNil(t, result.Embeddings)
	assert.False(t, result.Embeddings.HasModalityImbalance,
		"MiniLM vaults legitimately lack sparse/colbert — must not flag")
}

// HasModalityImbalance must be false when every dense-embedded note also has
// sparse and colbert (full BGE-M3 coverage).
func TestDoctor_BGEM3_FullCoverage_NoImbalance(t *testing.T) {
	dbPath := t.TempDir() + "/idx.db"
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	dense := make([]float32, 1024)
	sparse := map[int32]float32{1: 0.5}
	colbert := [][]float32{make([]float32, 1024)}

	_, err = db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, title, is_domain, embedding, sparse_embedding, colbert_embedding)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"n1", "n1.md", "h", 0, "T", true,
		index.EncodeEmbedding(dense),
		index.EncodeSparseEmbedding(sparse),
		index.EncodeColBERTEmbedding(colbert),
	)
	require.NoError(t, err)

	result, err := query.Doctor(db, "/vault")
	require.NoError(t, err)
	require.NotNil(t, result.Embeddings)
	assert.Equal(t, "bge-m3", result.Embeddings.Model)
	assert.False(t, result.Embeddings.HasModalityImbalance,
		"full coverage must not be flagged as imbalance")
}

// HasModalityImbalance must be TRUE when a BGE-M3 vault has dense embeddings
// but any note is missing sparse or colbert. This is the 2026-04-24 incident:
// 8 newly-added notes had dense but not sparse/colbert, silently compressing
// hybrid RRF ranking. The whole point of this field is to surface that state
// before it ships as degraded recall.
func TestDoctor_BGEM3_PartialCoverage_FlagsImbalance(t *testing.T) {
	dbPath := t.TempDir() + "/idx.db"
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	dense := make([]float32, 1024)
	sparse := map[int32]float32{1: 0.5}
	colbert := [][]float32{make([]float32, 1024)}

	// Note 1: full coverage.
	_, err = db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, title, is_domain, embedding, sparse_embedding, colbert_embedding)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"n1", "n1.md", "h1", 0, "T1", true,
		index.EncodeEmbedding(dense),
		index.EncodeSparseEmbedding(sparse),
		index.EncodeColBERTEmbedding(colbert),
	)
	require.NoError(t, err)
	// Note 2: dense only — the failure mode.
	_, err = db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, title, is_domain, embedding) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"n2", "n2.md", "h2", 0, "T2", true, index.EncodeEmbedding(dense),
	)
	require.NoError(t, err)

	result, err := query.Doctor(db, "/vault")
	require.NoError(t, err)
	require.NotNil(t, result.Embeddings)
	assert.Equal(t, "bge-m3", result.Embeddings.Model)
	assert.Equal(t, 2, result.Embeddings.DenseCount)
	assert.Equal(t, 1, result.Embeddings.SparseCount)
	assert.Equal(t, 1, result.Embeddings.ColBERTCount)
	assert.True(t, result.Embeddings.HasModalityImbalance,
		"dense=2 but sparse=colbert=1 under BGE-M3 must flag imbalance")
}
