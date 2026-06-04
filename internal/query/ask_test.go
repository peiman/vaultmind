package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/noisefloor"
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

// TestAsk_WithNoiseFloor_OverridesRRFConfidence pins the central behavior of
// the calibration slice: when HasNoiseFloor is set and similarities exist, the
// top-hit confidence is derived from R = top_cosine - N (not the RRF gap), and
// the honest fields are populated. mockEmbedder returns {1,0,0}; the
// spreading-activation note stores {1,0,0} so its cosine is 1.0 → R = 0.55 →
// strong, well clear of the noise floor.
func TestAsk_WithNoiseFloor_OverridesRRFConfidence(t *testing.T) {
	db := buildRetrieverTestDB(t)
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)

	// Find the actual FTS top hit and give it a strong-match embedding so the
	// query (mockEmbedder returns {1,0,0}) scores cosine 1.0 against it — robust
	// to whichever note the shared test vault ranks first.
	hits, _, err := retriever.Search(context.Background(), "spreading activation", 5, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	require.NoError(t, index.StoreEmbedding(db, hits[0].ID, []float32{1, 0, 0}))

	embedder := &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3}
	const n = 0.45
	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{
		Query:         "spreading activation",
		Budget:        4000,
		MaxItems:      5,
		SearchLimit:   5,
		Embedder:      embedder,
		NoiseFloor:    n,
		HasNoiseFloor: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.TopHits)

	assert.True(t, result.NoiseFloorApplied, "noise-floor confidence must be applied")
	assert.InDelta(t, n, result.NoiseFloor, 1e-9)
	assert.InDelta(t, 1.0, result.TopHitCosine, 1e-6, "top hit was given a perfect-match embedding")
	assert.InDelta(t, 1.0-n, result.RelevanceR, 1e-6, "RelevanceR is the raw cosine margin top_cosine − N")
	// Mirror the production derivation: z = (cosine − N)/σ, σ from the embedder
	// default (clamped), floor clamped to the embedder ceiling.
	_, wantLabel := noisefloor.Relevance(result.TopHitCosine, result.NoiseFloor, result.NoiseFloorSigma, noisefloor.DefaultNoiseFloor(3))
	assert.Equal(t, wantLabel, result.TopHitConfidence,
		"confidence must be the noise-floor label, not the RRF-gap label")
	assert.Equal(t, noisefloor.ConfidenceStrong, result.TopHitConfidence, "perfect-match cosine 1.0 → strong")
}

// TestAsk_HasNoiseFloor_NoEmbedder_FallsBackToRRF: HasNoiseFloor set but no
// embedder means no similarities, so the noise-floor branch must not fire —
// NoiseFloorApplied stays false and the RRF-gap fallback label is used.
func TestAsk_HasNoiseFloor_NoEmbedder_FallsBackToRRF(t *testing.T) {
	db := buildRetrieverTestDB(t)
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)

	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{
		Query:         "spreading activation",
		Budget:        4000,
		MaxItems:      5,
		SearchLimit:   5,
		NoiseFloor:    0.45,
		HasNoiseFloor: true, // but Embedder is nil → no similarities
	})
	require.NoError(t, err)
	assert.False(t, result.NoiseFloorApplied, "no embedder → noise-floor branch must not fire")
	assert.Zero(t, result.TopHitCosine)
}

// TestAsk_SuppressOnNoMatch_SkipsContextAndAccess pins the recall floor: when
// the top hit is at/below the noise floor (no_match) and the caller opts into
// suppression, Ask returns before the context pack + access fan-out — so the
// recall hook neither injects noise nor reinforces an irrelevant note.
func TestAsk_SuppressOnNoMatch_SkipsContextAndAccess(t *testing.T) {
	db := buildRetrieverTestDB(t)
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)

	// Give the FTS top hit an embedding ORTHOGONAL to the query vector {1,0,0}
	// so its cosine is 0 → R = 0 - 0.45 < 0 → no_match.
	hits, _, err := retriever.Search(context.Background(), "spreading activation", 5, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	require.NoError(t, index.StoreEmbedding(db, hits[0].ID, []float32{0, 1, 0}))

	embedder := &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3}
	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{
		Query:             "spreading activation",
		Budget:            4000,
		MaxItems:          5,
		SearchLimit:       5,
		Embedder:          embedder,
		NoiseFloor:        0.45,
		HasNoiseFloor:     true,
		SuppressOnNoMatch: true,
	})
	require.NoError(t, err)
	assert.Equal(t, noisefloor.ConfidenceNoMatch, result.TopHitConfidence, "orthogonal top hit → no_match")
	assert.Nil(t, result.Context, "no_match + suppress → context pack (and its access fan-out) skipped")
}

// Suppression must NOT fire when the top hit clears the floor — a relevant
// recall still packs context.
func TestAsk_SuppressOnNoMatch_RelevantStillPacks(t *testing.T) {
	db := buildRetrieverTestDB(t)
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)

	hits, _, err := retriever.Search(context.Background(), "spreading activation", 5, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	require.NoError(t, index.StoreEmbedding(db, hits[0].ID, []float32{1, 0, 0})) // cosine 1.0 → strong

	embedder := &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3}
	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{
		Query:             "spreading activation",
		Budget:            4000,
		MaxItems:          5,
		SearchLimit:       5,
		Embedder:          embedder,
		NoiseFloor:        0.45,
		HasNoiseFloor:     true,
		SuppressOnNoMatch: true,
	})
	require.NoError(t, err)
	assert.NotEqual(t, noisefloor.ConfidenceNoMatch, result.TopHitConfidence)
	assert.NotNil(t, result.Context, "relevant top hit → context pack still runs")
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
