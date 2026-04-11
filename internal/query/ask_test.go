package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsk_ReturnsHitsAndContext(t *testing.T) {
	db := buildIndexedDB(t)
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)

	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{
		Query:       "memory",
		Budget:      4000,
		MaxItems:    8,
		SearchLimit: 5,
	})

	require.NoError(t, err)
	assert.Equal(t, "memory", result.Query)
	assert.NotEmpty(t, result.TopHits)
	assert.LessOrEqual(t, len(result.TopHits), 5)
	// Context-pack from the top hit should be populated
	assert.NotNil(t, result.Context)
}

func TestAsk_EmptyQuery(t *testing.T) {
	db := buildIndexedDB(t)
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)

	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{
		Query:       "",
		Budget:      4000,
		MaxItems:    8,
		SearchLimit: 5,
	})

	require.NoError(t, err)
	assert.Equal(t, "", result.Query)
	assert.Empty(t, result.TopHits)
	assert.Nil(t, result.Context)
}

func TestAsk_NoHitsGivesNilContext(t *testing.T) {
	db := buildIndexedDB(t)
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)

	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{
		Query:       "xyzzy_nonexistent_7329",
		Budget:      4000,
		MaxItems:    8,
		SearchLimit: 5,
	})

	require.NoError(t, err)
	assert.Empty(t, result.TopHits)
	assert.Nil(t, result.Context)
}

func TestAsk_LimitsHitsToSearchLimit(t *testing.T) {
	db := buildIndexedDB(t)
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)

	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{
		Query:       "memory",
		Budget:      4000,
		MaxItems:    8,
		SearchLimit: 3,
	})

	require.NoError(t, err)
	assert.LessOrEqual(t, len(result.TopHits), 3)
}

func TestAsk_WithEmbedder_ComputesSimilarities(t *testing.T) {
	db := buildRetrieverTestDB(t)

	// Store embeddings for notes so the embedding retriever has data
	row1, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row1)
	row2, err := db.QueryNoteByPath("concepts/episodic-memory.md")
	require.NoError(t, err)
	require.NotNil(t, row2)
	require.NoError(t, index.StoreEmbedding(db, row1.ID, []float32{1, 0, 0}))
	require.NoError(t, index.StoreEmbedding(db, row2.ID, []float32{0, 1, 0}))

	embedder := &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3}
	retriever := &query.FTSRetriever{DB: db} // FTS for search, embedder for similarities
	resolver := graph.NewResolver(db)

	var activationFuncCalled bool
	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{
		Query:       "spreading activation",
		Budget:      4000,
		MaxItems:    5,
		SearchLimit: 5,
		Embedder:    embedder,
		ActivationFunc: func(sims map[string]float64) map[string]float64 {
			activationFuncCalled = true
			assert.NotEmpty(t, sims, "similarities should be non-empty")
			// Return a simple score map to verify it's used
			scores := make(map[string]float64, len(sims))
			for id, sim := range sims {
				scores[id] = sim * 10.0 // arbitrary transformation
			}
			return scores
		},
	})

	require.NoError(t, err)
	assert.NotEmpty(t, result.TopHits)
	assert.NotNil(t, result.Similarities, "similarities should be populated when embedder is provided")
	assert.InDelta(t, 1.0, result.Similarities[row1.ID], 1e-6)
	assert.InDelta(t, 0.0, result.Similarities[row2.ID], 1e-6)
	assert.True(t, activationFuncCalled, "ActivationFunc should be called when similarities are available")
}

func TestAsk_WithEmbedder_NoActivationFunc(t *testing.T) {
	db := buildRetrieverTestDB(t)

	row1, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NoError(t, index.StoreEmbedding(db, row1.ID, []float32{1, 0, 0}))

	embedder := &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3}
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)

	// Embedder provided but no ActivationFunc — similarities computed, no recompute
	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{
		Query:       "spreading activation",
		Budget:      4000,
		MaxItems:    5,
		SearchLimit: 5,
		Embedder:    embedder,
	})

	require.NoError(t, err)
	assert.NotNil(t, result.Similarities, "similarities should still be computed even without ActivationFunc")
}
