package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check
var _ query.Retriever = (*query.EmbeddingRetriever)(nil)

// mockEmbedder returns a fixed vector for any input.
type mockEmbedder struct {
	vec  []float32
	dims int
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return m.vec, nil
}
func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = m.vec
	}
	return result, nil
}
func (m *mockEmbedder) Dims() int    { return m.dims }
func (m *mockEmbedder) Close() error { return nil }

func TestEmbeddingRetriever_Search(t *testing.T) {
	db := buildRetrieverTestDB(t)

	// Find two existing notes from the indexed vault
	row1, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row1)
	row2, err := db.QueryNoteByPath("concepts/episodic-memory.md")
	require.NoError(t, err)
	require.NotNil(t, row2)

	// row1 gets vector close to query, row2 gets orthogonal vector
	require.NoError(t, index.StoreEmbedding(db, row1.ID, []float32{1, 0, 0}))
	require.NoError(t, index.StoreEmbedding(db, row2.ID, []float32{0, 1, 0}))

	embedder := &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3}
	retriever := &query.EmbeddingRetriever{DB: db, Embedder: embedder}

	results, total, err := retriever.Search(context.Background(), "spreading activation", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, results, 2)
	assert.Equal(t, row1.ID, results[0].ID)
	assert.InDelta(t, 1.0, results[0].Score, 1e-6)
	assert.Equal(t, row2.ID, results[1].ID)
	assert.InDelta(t, 0.0, results[1].Score, 1e-6)
}

func TestEmbeddingRetriever_SearchWithLimit(t *testing.T) {
	db := buildRetrieverTestDB(t)

	row1, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row1)
	row2, err := db.QueryNoteByPath("concepts/episodic-memory.md")
	require.NoError(t, err)
	require.NotNil(t, row2)

	require.NoError(t, index.StoreEmbedding(db, row1.ID, []float32{1, 0}))
	require.NoError(t, index.StoreEmbedding(db, row2.ID, []float32{0, 1}))

	embedder := &mockEmbedder{vec: []float32{1, 0}, dims: 2}
	retriever := &query.EmbeddingRetriever{DB: db, Embedder: embedder}

	results, total, err := retriever.Search(context.Background(), "test", 1, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 2, total, "total should reflect all embeddings")
	assert.Len(t, results, 1, "limit=1 should return only 1 result")
}

func TestEmbeddingRetriever_SearchNoEmbeddings(t *testing.T) {
	db := buildRetrieverTestDB(t)

	embedder := &mockEmbedder{vec: []float32{1, 0}, dims: 2}
	retriever := &query.EmbeddingRetriever{DB: db, Embedder: embedder}

	results, total, err := retriever.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, results)
}

func TestEmbeddingRetriever_SearchWithTypeFilter(t *testing.T) {
	db := buildRetrieverTestDB(t)

	row1, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row1)

	require.NoError(t, index.StoreEmbedding(db, row1.ID, []float32{1, 0}))

	embedder := &mockEmbedder{vec: []float32{1, 0}, dims: 2}
	retriever := &query.EmbeddingRetriever{DB: db, Embedder: embedder}

	// Filter by a type that doesn't match
	results, _, err := retriever.Search(context.Background(), "test", 10, 0, index.SearchFilters{Type: "nonexistent"})
	require.NoError(t, err)
	assert.Empty(t, results)

	// Filter by correct type
	results, _, err = retriever.Search(context.Background(), "test", 10, 0, index.SearchFilters{Type: "concept"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
}
