package query_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRetriever_Keyword(t *testing.T) {
	db := buildRetrieverTestDB(t)
	ret, cleanup, err := query.BuildRetriever("keyword", db)
	require.NoError(t, err)
	assert.Nil(t, cleanup, "keyword mode needs no cleanup")
	assert.IsType(t, &query.FTSRetriever{}, ret)
}

func TestBuildRetriever_EmptyModeDefaultsToKeyword(t *testing.T) {
	db := buildRetrieverTestDB(t)
	ret, cleanup, err := query.BuildRetriever("", db)
	require.NoError(t, err)
	assert.Nil(t, cleanup)
	assert.IsType(t, &query.FTSRetriever{}, ret)
}

func TestBuildRetriever_UnknownMode(t *testing.T) {
	db := buildRetrieverTestDB(t)
	_, _, err := query.BuildRetriever("bogus", db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown search mode")
	assert.Contains(t, err.Error(), "bogus")
}

func TestBuildRetriever_SemanticNoEmbeddings(t *testing.T) {
	db := buildRetrieverTestDB(t)
	_, _, err := query.BuildRetriever("semantic", db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no embeddings found")
}

func TestBuildRetriever_HybridNoEmbeddings(t *testing.T) {
	db := buildRetrieverTestDB(t)
	_, _, err := query.BuildRetriever("hybrid", db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no embeddings found")
}

func TestBuildRetriever_SemanticWithEmbeddings(t *testing.T) {
	db := buildRetrieverTestDB(t)
	// Store a dummy embedding so HasEmbeddings returns true
	row, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row)
	require.NoError(t, index.StoreEmbedding(db, row.ID, []float32{0.1, 0.2, 0.3}))

	ret, cleanup, err := query.BuildRetriever("semantic", db)
	// This will fail because NewHugotEmbedder tries to download the model.
	// That's expected — we're testing the flow, not the embedder init.
	// If it errors, it should be about creating the embedder, not about embeddings.
	if err != nil {
		assert.NotContains(t, err.Error(), "no embeddings found",
			"should pass the embeddings check and fail on embedder init instead")
		return
	}
	assert.IsType(t, &query.EmbeddingRetriever{}, ret)
	if cleanup != nil {
		cleanup()
	}
}
