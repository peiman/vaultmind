package query_test

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// candidateLane records what it was asked to score.
type candidateLane struct {
	among     []string
	fullCalls int
	results   []retrieval.ScoredResult
}

func (c *candidateLane) Search(context.Context, string, int, int, index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	c.fullCalls++
	return c.results, len(c.results), nil
}

func (c *candidateLane) SearchAmong(_ context.Context, _ string, _ int, _ index.SearchFilters, ids []string) ([]retrieval.ScoredResult, int, error) {
	c.among = append([]string(nil), ids...)
	sort.Strings(c.among)
	var out []retrieval.ScoredResult
	for _, r := range c.results {
		for _, id := range ids {
			if r.ID == id {
				out = append(out, r)
			}
		}
	}
	return out, len(out), nil
}

// A lane that can score a given set (ColBERT) scores only what the other lanes
// found: on a 416-note vault that is 163 notes instead of all, and on 32
// labelled real queries it was no worse (Hit@1 27 -> 28, MRR 0.888 -> 0.903).
func TestHybridRetriever_ACandidateLaneScoresOnlyWhatTheOthersFound(t *testing.T) {
	colbert := &candidateLane{results: []retrieval.ScoredResult{{ID: "b"}, {ID: "z-only-colbert"}}}
	h := &query.HybridRetriever{Retrievers: []retrieval.NamedRetriever{
		{Name: "fts", Retriever: &staticRetriever{results: []retrieval.ScoredResult{{ID: "a"}, {ID: "b"}}}},
		{Name: "dense", Retriever: &staticRetriever{results: []retrieval.ScoredResult{{ID: "b"}, {ID: "c"}}}},
		{Name: "colbert", Retriever: colbert},
	}}

	got, _, err := h.Search(context.Background(), "q", 10, 0, index.SearchFilters{})
	require.NoError(t, err)

	assert.Equal(t, []string{"a", "b", "c"}, colbert.among, "the union of the other lanes, each id once")
	assert.Zero(t, colbert.fullCalls)
	ids := map[string]map[string]float64{}
	for _, r := range got {
		ids[r.ID] = r.Components
	}
	assert.NotContains(t, ids, "z-only-colbert", "a note no other lane found is not scored")
	assert.Contains(t, ids["b"], "colbert", "the candidate lane still votes in the fusion")
}

// With nothing from the other lanes there is nothing to narrow to: the lane
// searches everything rather than returning nothing.
func TestHybridRetriever_ACandidateLaneSearchesAllWhenTheOthersFoundNothing(t *testing.T) {
	colbert := &candidateLane{results: []retrieval.ScoredResult{{ID: "x"}}}
	h := &query.HybridRetriever{Retrievers: []retrieval.NamedRetriever{
		{Name: "fts", Retriever: &staticRetriever{}},
		{Name: "colbert", Retriever: colbert},
	}}

	got, _, err := h.Search(context.Background(), "q", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 1, colbert.fullCalls)
	require.Len(t, got, 1)
	assert.Equal(t, "x", got[0].ID)
}

// The candidate lane scores the union of each other lane's top 20, not of
// everything they fetched (100 each). Reading and scoring ColBERT vectors was
// two thirds of a warm ask; on 32 labelled real queries the cut kept Hit@5
// (30/32) and raised MRR (0.887 -> 0.910), long-doc page Hit@3 and delivery
// held, and a three-vault ask got 23% faster (2026-10-05).
func TestHybridRetriever_ACandidateLaneScoresEachLanesTop20(t *testing.T) {
	ranked := func(prefix string, n int) []retrieval.ScoredResult {
		out := make([]retrieval.ScoredResult, n)
		for i := range out {
			out[i] = retrieval.ScoredResult{ID: fmt.Sprintf("%s%02d", prefix, i)}
		}
		return out
	}
	colbert := &candidateLane{}
	h := &query.HybridRetriever{Retrievers: []retrieval.NamedRetriever{
		{Name: "fts", Retriever: &staticRetriever{results: ranked("f", 30)}},
		{Name: "dense", Retriever: &staticRetriever{results: ranked("d", 30)}},
		{Name: "colbert", Retriever: colbert},
	}}

	_, _, err := h.Search(context.Background(), "q", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.Len(t, colbert.among, 40, "20 from each lane")
	assert.Contains(t, colbert.among, "f19")
	assert.NotContains(t, colbert.among, "f20")
	assert.Contains(t, colbert.among, "d19")
	assert.NotContains(t, colbert.among, "d20")
}

// A caller asking for more than 20 results (search --limit 50, or paging with
// --offset) gets ColBERT's vote across the window it asked for: the cut is
// never below limit+offset.
func TestHybridRetriever_TheCandidateCutCoversTheRequestedWindow(t *testing.T) {
	ranked := func(prefix string, n int) []retrieval.ScoredResult {
		out := make([]retrieval.ScoredResult, n)
		for i := range out {
			out[i] = retrieval.ScoredResult{ID: fmt.Sprintf("%s%02d", prefix, i)}
		}
		return out
	}
	colbert := &candidateLane{}
	h := &query.HybridRetriever{Retrievers: []retrieval.NamedRetriever{
		{Name: "fts", Retriever: &staticRetriever{results: ranked("f", 60)}},
		{Name: "colbert", Retriever: colbert},
	}}

	_, _, err := h.Search(context.Background(), "q", 30, 10, index.SearchFilters{})
	require.NoError(t, err)
	assert.Len(t, colbert.among, 40, "limit 30 + offset 10")
}
