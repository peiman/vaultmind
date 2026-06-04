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

var _ retrieval.Retriever = (*query.ColBERTRetriever)(nil)

func TestColBERTRetriever_Search(t *testing.T) {
	db := buildRetrieverTestDB(t)

	row1, _ := db.QueryNoteByPath("concepts/spreading-activation.md")
	row2, _ := db.QueryNoteByPath("concepts/episodic-memory.md")
	require.NotNil(t, row1)
	require.NotNil(t, row2)

	// row1: tokens that match query well
	require.NoError(t, index.StoreColBERTEmbedding(db, row1.ID, [][]float32{
		{1, 0}, {0, 1},
	}))
	// row2: tokens that partially match
	require.NoError(t, index.StoreColBERTEmbedding(db, row2.ID, [][]float32{
		{0.7, 0.7}, {-0.7, 0.7},
	}))

	embedFunc := func(_ context.Context, _ string) ([][]float32, error) {
		return [][]float32{{1, 0}, {0, 1}}, nil
	}
	retriever := &query.ColBERTRetriever{DB: db, EmbedColBERT: embedFunc, Dims: 2}

	results, total, err := retriever.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, results, 2)
	assert.Equal(t, row1.ID, results[0].ID, "row1 should rank first (exact token matches)")
	assert.Greater(t, results[0].Score, results[1].Score)
}

func TestColBERTRetriever_NoEmbeddings(t *testing.T) {
	db := buildRetrieverTestDB(t)
	embedFunc := func(_ context.Context, _ string) ([][]float32, error) {
		return [][]float32{{1, 0}}, nil
	}
	retriever := &query.ColBERTRetriever{DB: db, EmbedColBERT: embedFunc, Dims: 2}

	results, total, err := retriever.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, results)
}
