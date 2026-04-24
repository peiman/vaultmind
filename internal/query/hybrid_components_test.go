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

func TestHybridRetriever_PopulatesPerComponentScores(t *testing.T) {
	retA := &staticRetriever{
		results: []retrieval.ScoredResult{
			{ID: "note1", Score: 1.0},
			{ID: "note2", Score: 0.5},
		},
		total: 2,
	}
	retB := &staticRetriever{
		results: []retrieval.ScoredResult{
			{ID: "note2", Score: 1.0},
			{ID: "note1", Score: 0.5},
		},
		total: 2,
	}

	hybrid := &query.HybridRetriever{
		Retrievers: []retrieval.NamedRetriever{
			{Name: "fts", Retriever: retA},
			{Name: "dense", Retriever: retB},
		},
		K: 60,
	}

	results, _, err := hybrid.Search(context.Background(), "q", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.Len(t, results, 2)

	byID := map[string]retrieval.ScoredResult{results[0].ID: results[0], results[1].ID: results[1]}

	// note1: rank 1 in fts, rank 2 in dense → components {fts: 1/61, dense: 1/62}
	require.NotNil(t, byID["note1"].Components)
	assert.InDelta(t, 1.0/61.0, byID["note1"].Components["fts"], 1e-9)
	assert.InDelta(t, 1.0/62.0, byID["note1"].Components["dense"], 1e-9)

	// note2: rank 2 in fts, rank 1 in dense → components {fts: 1/62, dense: 1/61}
	require.NotNil(t, byID["note2"].Components)
	assert.InDelta(t, 1.0/62.0, byID["note2"].Components["fts"], 1e-9)
	assert.InDelta(t, 1.0/61.0, byID["note2"].Components["dense"], 1e-9)

	// Under mean-of-present RRF, Score is the mean of the per-lane raw
	// contributions (not their sum). Keeping components in raw 1/(K+rank)
	// units preserves the diagnostic signal — "what did each lane say" —
	// while Score reflects the fusion-of-lanes-that-scored-this-note.
	for _, id := range []string{"note1", "note2"} {
		sum := 0.0
		for _, v := range byID[id].Components {
			sum += v
		}
		n := float64(len(byID[id].Components))
		assert.InDelta(t, byID[id].Score, sum/n, 1e-9,
			"Score should be the mean of per-lane components for %s", id)
	}
}

func TestHybridRetriever_NoComponentsWhenRetrieverAbsent(t *testing.T) {
	// When a note only appears in one retriever's list, the component map
	// contains only that retriever's contribution — absent retrievers don't
	// leak zero entries.
	retA := &staticRetriever{
		results: []retrieval.ScoredResult{{ID: "only-fts", Score: 1.0}},
		total:   1,
	}
	retB := &staticRetriever{results: nil, total: 0}

	hybrid := &query.HybridRetriever{
		Retrievers: []retrieval.NamedRetriever{
			{Name: "fts", Retriever: retA},
			{Name: "dense", Retriever: retB},
		},
	}

	results, _, err := hybrid.Search(context.Background(), "q", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.Len(t, results, 1)

	require.Contains(t, results[0].Components, "fts")
	assert.NotContains(t, results[0].Components, "dense", "absent retriever should not appear in components")
}
