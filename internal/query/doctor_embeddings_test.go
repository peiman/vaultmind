package query_test

import (
	"fmt"
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

	result, err := query.Doctor(db, "/vault", nil)
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

	result, err := query.Doctor(db, "/vault", nil)
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

	// BGE-M3 requires all three modalities in lockstep (migration 006).
	// INSERT the full row so the parity trigger passes.
	vec := make([]float32, 1024)
	_, err = db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, title, is_domain, embedding, sparse_embedding, colbert_embedding)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"n1", "n1.md", "h", 0, "T", true,
		index.EncodeEmbedding(vec),
		index.EncodeSparseEmbedding(map[int32]float32{0: 1.0}),
		index.EncodeColBERTEmbedding([][]float32{vec}),
	)
	require.NoError(t, err)

	result, err := query.Doctor(db, "/vault", nil)
	require.NoError(t, err)
	require.NotNil(t, result.Embeddings)
	assert.Equal(t, "bge-m3", result.Embeddings.Model)
	assert.True(t, result.Embeddings.SemanticReady)
}

// In a mixed-state vault (some notes MiniLM, others BGE-M3), doctor must
// surface the per-model breakdown as Model="mixed" with MixedModel populated
// — not silently classify the vault as one model based on whichever row
// SQLite scans first. See vaultmind#22 dig.
func TestDoctor_MixedModelState_SurfacedExplicitly(t *testing.T) {
	dbPath := t.TempDir() + "/idx.db"
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Two MiniLM rows.
	miniVec := make([]float32, 384)
	for i := 0; i < 2; i++ {
		_, err = db.Exec(
			`INSERT INTO notes (id, path, hash, mtime, title, is_domain, embedding)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("m%d", i), fmt.Sprintf("m%d.md", i), "h", 0, "T", true,
			index.EncodeEmbedding(miniVec),
		)
		require.NoError(t, err)
	}
	// One BGE-M3 row (must include sparse + colbert per the parity trigger).
	bgeVec := make([]float32, 1024)
	_, err = db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, title, is_domain, embedding, sparse_embedding, colbert_embedding)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"b0", "b0.md", "h", 0, "T", true,
		index.EncodeEmbedding(bgeVec),
		index.EncodeSparseEmbedding(map[int32]float32{0: 1.0}),
		index.EncodeColBERTEmbedding([][]float32{bgeVec}),
	)
	require.NoError(t, err)

	result, err := query.Doctor(db, "/vault", nil)
	require.NoError(t, err)
	require.NotNil(t, result.Embeddings)
	assert.Equal(t, "mixed", result.Embeddings.Model,
		"vault with both MiniLM and BGE-M3 rows must classify as 'mixed', not one or the other")
	require.Len(t, result.Embeddings.MixedModel, 2,
		"MixedModel must carry a breakdown entry per distinct model")
	// Counts ordered descending by count → MiniLM first (2 vs 1).
	assert.Equal(t, "minilm", result.Embeddings.MixedModel[0].Model)
	assert.Equal(t, 2, result.Embeddings.MixedModel[0].Count)
	assert.Equal(t, "bge-m3", result.Embeddings.MixedModel[1].Model)
	assert.Equal(t, 1, result.Embeddings.MixedModel[1].Count)
	assert.True(t, result.Embeddings.HasModalityImbalance,
		"mixed-state with MiniLM rows lacking sparse/colbert is an imbalance — operator should know")
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

	result, err := query.Doctor(db, "/vault", nil)
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

	result, err := query.Doctor(db, "/vault", nil)
	require.NoError(t, err)
	require.NotNil(t, result.Embeddings)
	assert.Equal(t, "bge-m3", result.Embeddings.Model)
	assert.False(t, result.Embeddings.HasModalityImbalance,
		"full coverage must not be flagged as imbalance")
}

// HasModalityImbalance must be TRUE when a BGE-M3 vault has dense embeddings
// but any note is missing sparse or colbert. This is the 2026-04-24 incident:
// 8 newly-added notes had dense but not sparse/colbert, silently compressing
// hybrid RRF ranking.
//
// As of migration 006 the schema trigger prevents this state from being
// written in the first place — this doctor field is now defense-in-depth
// (handles databases in the wild that pre-date migration 006, or that were
// restored from an older backup). The test temporarily drops the trigger so
// it can stage the failure state, verifies detection, then restores the
// schema invariant. Bypassing the trigger in a test is the only supported
// way to produce the state the doctor warning guards against.
func TestDoctor_BGEM3_PartialCoverage_FlagsImbalance(t *testing.T) {
	dbPath := t.TempDir() + "/idx.db"
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`DROP TRIGGER IF EXISTS bgem3_modality_parity_insert`)
	require.NoError(t, err)
	_, err = db.Exec(`DROP TRIGGER IF EXISTS bgem3_modality_parity_update`)
	require.NoError(t, err)

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
	// Note 2: dense only — the failure mode. Only stage-able with the
	// trigger dropped above.
	_, err = db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, title, is_domain, embedding) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"n2", "n2.md", "h2", 0, "T2", true, index.EncodeEmbedding(dense),
	)
	require.NoError(t, err)

	result, err := query.Doctor(db, "/vault", nil)
	require.NoError(t, err)
	require.NotNil(t, result.Embeddings)
	assert.Equal(t, "bge-m3", result.Embeddings.Model)
	assert.Equal(t, 2, result.Embeddings.DenseCount)
	assert.Equal(t, 1, result.Embeddings.SparseCount)
	assert.Equal(t, 1, result.Embeddings.ColBERTCount)
	assert.True(t, result.Embeddings.HasModalityImbalance,
		"dense=2 but sparse=colbert=1 under BGE-M3 must flag imbalance")
}
