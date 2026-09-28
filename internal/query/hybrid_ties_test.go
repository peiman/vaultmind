package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/require"
)

// Notes at the same rank in different lanes get exactly the same RRF score.
// Their order came from ranging over a map, so two runs of the same query on
// the same vault disagreed below the top (#208) — and a ranking that changes
// run to run cannot be used to check that a change left rankings alone.
// Ties now break by note id.
func TestHybridRetriever_TiedScoresOrderByID(t *testing.T) {
	ids := func(rs ...string) []retrieval.ScoredResult {
		out := make([]retrieval.ScoredResult, len(rs))
		for i, id := range rs {
			out[i] = retrieval.ScoredResult{ID: id}
		}
		return out
	}
	h := &query.HybridRetriever{Retrievers: []retrieval.NamedRetriever{
		{Name: "fts", Retriever: &staticRetriever{results: ids("e", "c", "a", "g", "i")}},
		{Name: "dense", Retriever: &staticRetriever{results: ids("f", "d", "b", "h", "j")}},
	}}
	want := []string{"e", "f", "c", "d", "a", "b", "g", "h", "i", "j"}

	for run := range 30 {
		got, _, err := h.Search(context.Background(), "q", 10, 0, index.SearchFilters{})
		require.NoError(t, err)
		order := make([]string, len(got))
		for i, r := range got {
			order[i] = r.ID
		}
		require.Equal(t, want, order, "run %d", run)
	}
}
