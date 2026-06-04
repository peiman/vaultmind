package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ retrieval.Retriever = (*query.SparseRetriever)(nil)

func TestSparseRetriever_Search(t *testing.T) {
	db := buildRetrieverTestDB(t)

	row1, _ := db.QueryNoteByPath("concepts/spreading-activation.md")
	row2, _ := db.QueryNoteByPath("concepts/episodic-memory.md")
	require.NotNil(t, row1)
	require.NotNil(t, row2)

	// row1 has terms overlapping with query, row2 does not
	require.NoError(t, index.StoreSparseEmbedding(db, row1.ID, map[int32]float32{100: 1.0, 200: 0.5}))
	require.NoError(t, index.StoreSparseEmbedding(db, row2.ID, map[int32]float32{300: 1.0, 400: 0.5}))

	embedFunc := func(_ context.Context, _ string) (map[int32]float32, error) {
		return map[int32]float32{100: 0.8, 200: 0.3}, nil
	}
	retriever := &query.SparseRetriever{DB: db, EmbedSparse: embedFunc}

	results, total, err := retriever.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, results, 2)
	assert.Equal(t, row1.ID, results[0].ID, "row1 should rank first (overlapping terms)")
	assert.Greater(t, results[0].Score, results[1].Score)
}

func TestSparseRetriever_NoEmbeddings(t *testing.T) {
	db := buildRetrieverTestDB(t)
	embedFunc := func(_ context.Context, _ string) (map[int32]float32, error) {
		return map[int32]float32{100: 1.0}, nil
	}
	retriever := &query.SparseRetriever{DB: db, EmbedSparse: embedFunc}

	results, total, err := retriever.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, results)
}
