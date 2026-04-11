package query_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float32{1, 2, 3}
	sim := query.CosineSimilarity(a, a)
	assert.InDelta(t, 1.0, sim, 1e-6)
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	sim := query.CosineSimilarity(a, b)
	assert.InDelta(t, 0.0, sim, 1e-6)
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{-1, -2, -3}
	sim := query.CosineSimilarity(a, b)
	assert.InDelta(t, -1.0, sim, 1e-6)
}

func TestCosineSimilarity_KnownValue(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{1, 1}
	// cos(45°) = 1/√2 ≈ 0.7071
	sim := query.CosineSimilarity(a, b)
	assert.InDelta(t, 1.0/math.Sqrt(2), sim, 1e-6)
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	sim := query.CosineSimilarity(a, b)
	assert.Equal(t, 0.0, sim)
}

func TestNoteSimilarities_NilEmbedder(t *testing.T) {
	sims, err := query.NoteSimilarities(context.Background(), "test", nil, nil)
	require.NoError(t, err)
	assert.Nil(t, sims)
}

func TestNoteSimilarities_HappyPath(t *testing.T) {
	db := buildRetrieverTestDB(t)

	// Store embeddings for two known notes
	row1, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row1)
	row2, err := db.QueryNoteByPath("concepts/episodic-memory.md")
	require.NoError(t, err)
	require.NotNil(t, row2)

	// row1 aligned with query vector, row2 orthogonal
	require.NoError(t, index.StoreEmbedding(db, row1.ID, []float32{1, 0, 0}))
	require.NoError(t, index.StoreEmbedding(db, row2.ID, []float32{0, 1, 0}))

	// mockEmbedder returns the query vector [1,0,0]
	embedder := &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3}

	sims, err := query.NoteSimilarities(context.Background(), "spreading activation", embedder, db)
	require.NoError(t, err)
	require.NotNil(t, sims)

	// row1 should be highly similar (cosine ≈ 1.0)
	assert.InDelta(t, 1.0, sims[row1.ID], 1e-6, "aligned note should have similarity ≈ 1.0")
	// row2 should be orthogonal (cosine ≈ 0.0)
	assert.InDelta(t, 0.0, sims[row2.ID], 1e-6, "orthogonal note should have similarity ≈ 0.0")
	// All notes with embeddings should be in the map
	assert.Len(t, sims, 2)
}

// errorEmbedder always returns an error from Embed.
type errorEmbedder struct{}

func (e *errorEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("embedding model unavailable")
}
func (e *errorEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, errors.New("embedding model unavailable")
}
func (e *errorEmbedder) Dims() int    { return 0 }
func (e *errorEmbedder) Close() error { return nil }

func TestNoteSimilarities_EmbedError(t *testing.T) {
	db := buildRetrieverTestDB(t)
	embedder := &errorEmbedder{}

	sims, err := query.NoteSimilarities(context.Background(), "test query", embedder, db)
	require.Error(t, err)
	assert.Nil(t, sims)
	assert.Contains(t, err.Error(), "embedding model unavailable")
}
