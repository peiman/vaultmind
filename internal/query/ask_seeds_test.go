package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seededAsk gives the first three hits for "activation" cosines of 1.0, 0.8 and
// 0.5 against the query. With N = 0.45 and σ = 0.1 that is z = 5.5, 3.5 and 0.5:
// the second hit clears the weak band, the third does not.
func seededAsk(t *testing.T, hasFloor bool) (*query.AskResult, []string) {
	t.Helper()
	db := buildRetrieverTestDB(t)
	retriever := &query.FTSRetriever{DB: db}
	hits, _, err := retriever.Search(context.Background(), "activation", 5, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(hits), 3, "fixture needs three hits for this query")
	vecs := [][]float32{{1, 0, 0}, {0.8, 0.6, 0}, {0.5, 0.866, 0}}
	ids := make([]string, len(vecs))
	for i, v := range vecs {
		ids[i] = hits[i].ID
		require.NoError(t, index.StoreEmbedding(db, hits[i].ID, v))
	}

	result, err := query.Ask(context.Background(), retriever, graph.NewResolver(db), db, query.AskConfig{
		Query:           "activation",
		Budget:          8000,
		MaxItems:        8,
		SearchLimit:     5,
		Embedder:        &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3},
		NoiseFloor:      0.45,
		NoiseFloorSigma: 0.1,
		HasNoiseFloor:   hasFloor,
	})
	require.NoError(t, err)
	require.NotNil(t, result.Context)
	return result, ids
}

func searchHitIDs(pack *memory.ContextPackResult) map[string]string {
	out := map[string]string{}
	for _, item := range pack.Context {
		if item.EdgeType == memory.EdgeTypeSearchHit {
			out[item.ID] = item.Confidence
		}
	}
	return out
}

func TestAsk_PacksLaterHitsThatClearTheWeakBand(t *testing.T) {
	result, ids := seededAsk(t, true)
	seeded := searchHitIDs(result.Context)

	assert.Equal(t, ids[0], result.Context.TargetID)
	assert.Equal(t, "strong", seeded[ids[1]], "z = 3.5 → packed as a search hit, labelled by its own band")
	assert.NotContains(t, seeded, ids[2], "z = 0.5 is weak → not seeded")
	assert.Len(t, seeded, 1)
}

func TestAsk_NoNoiseFloorMeansNoSeeds(t *testing.T) {
	result, _ := seededAsk(t, false)
	assert.Empty(t, searchHitIDs(result.Context), "without a floor there is no per-hit relevance to gate on")
}
