package query_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check
var _ retrieval.Retriever = (*query.HybridRetriever)(nil)

// staticRetriever returns a fixed set of results.
type staticRetriever struct {
	results []retrieval.ScoredResult
	total   int
}

func (r *staticRetriever) Search(_ context.Context, _ string, limit, offset int, _ index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	results := r.results
	if offset >= len(results) {
		return nil, r.total, nil
	}
	results = results[offset:]
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, r.total, nil
}

func TestHybridRetriever_TwoRetrievers(t *testing.T) {
	// Retriever A ranks: note1, note2, note3
	retA := &staticRetriever{
		results: []retrieval.ScoredResult{
			{ID: "note1", Title: "Note 1", Score: 1.0},
			{ID: "note2", Title: "Note 2", Score: 0.5},
			{ID: "note3", Title: "Note 3", Score: 0.1},
		},
		total: 3,
	}
	// Retriever B ranks: note2, note3, note1
	retB := &staticRetriever{
		results: []retrieval.ScoredResult{
			{ID: "note2", Title: "Note 2", Score: 1.0},
			{ID: "note3", Title: "Note 3", Score: 0.5},
			{ID: "note1", Title: "Note 1", Score: 0.1},
		},
		total: 3,
	}

	hybrid := &query.HybridRetriever{
		Retrievers: []retrieval.NamedRetriever{{Name: "a", Retriever: retA}, {Name: "b", Retriever: retB}},
		K:          60,
	}

	results, total, err := hybrid.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, results, 3)

	// RRF math:
	// note1: rank 1 in A + rank 3 in B = 1/61 + 1/63 = 0.01639 + 0.01587 = 0.03226
	// note2: rank 2 in A + rank 1 in B = 1/62 + 1/61 = 0.01613 + 0.01639 = 0.03252
	// note3: rank 3 in A + rank 2 in B = 1/63 + 1/62 = 0.01587 + 0.01613 = 0.03200
	// Order: note2 > note1 > note3
	assert.Equal(t, "note2", results[0].ID, "note2 should rank first")
	assert.Equal(t, "note1", results[1].ID, "note1 should rank second")
	assert.Equal(t, "note3", results[2].ID, "note3 should rank third")
}

func TestHybridRetriever_SingleRetriever(t *testing.T) {
	ret := &staticRetriever{
		results: []retrieval.ScoredResult{
			{ID: "a", Title: "A", Score: 1.0},
			{ID: "b", Title: "B", Score: 0.5},
		},
		total: 2,
	}

	hybrid := &query.HybridRetriever{
		Retrievers: []retrieval.NamedRetriever{{Name: "ret", Retriever: ret}},
		K:          60,
	}

	results, total, err := hybrid.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Equal(t, "a", results[0].ID)
	assert.Equal(t, "b", results[1].ID)
}

func TestHybridRetriever_EmptyRetriever(t *testing.T) {
	retA := &staticRetriever{
		results: []retrieval.ScoredResult{
			{ID: "note1", Title: "Note 1", Score: 1.0},
		},
		total: 1,
	}
	retB := &staticRetriever{results: nil, total: 0}

	hybrid := &query.HybridRetriever{
		Retrievers: []retrieval.NamedRetriever{{Name: "a", Retriever: retA}, {Name: "b", Retriever: retB}},
		K:          60,
	}

	results, total, err := hybrid.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "note1", results[0].ID)
}

func TestHybridRetriever_DisjointResults(t *testing.T) {
	retA := &staticRetriever{
		results: []retrieval.ScoredResult{{ID: "a", Title: "A", Score: 1.0}},
		total:   1,
	}
	retB := &staticRetriever{
		results: []retrieval.ScoredResult{{ID: "b", Title: "B", Score: 1.0}},
		total:   1,
	}

	hybrid := &query.HybridRetriever{
		Retrievers: []retrieval.NamedRetriever{{Name: "a", Retriever: retA}, {Name: "b", Retriever: retB}},
		K:          60,
	}

	results, total, err := hybrid.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, results, 2)
	ids := map[string]bool{results[0].ID: true, results[1].ID: true}
	assert.True(t, ids["a"])
	assert.True(t, ids["b"])
}

func TestHybridRetriever_LimitAndOffset(t *testing.T) {
	results := make([]retrieval.ScoredResult, 5)
	for i := range results {
		results[i] = retrieval.ScoredResult{ID: fmt.Sprintf("n%d", i), Title: fmt.Sprintf("N%d", i), Score: float64(5 - i)}
	}

	ret := &staticRetriever{results: results, total: 5}
	hybrid := &query.HybridRetriever{
		Retrievers: []retrieval.NamedRetriever{{Name: "ret", Retriever: ret}},
		K:          60,
	}

	res, total, err := hybrid.Search(context.Background(), "test", 2, 1, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, res, 2)
}

// errorRetriever always returns an error.
type errorRetriever struct{}

func (r *errorRetriever) Search(_ context.Context, _ string, _, _ int, _ index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	return nil, 0, fmt.Errorf("retriever error")
}

func TestHybridRetriever_ErrorPropagation(t *testing.T) {
	ret := &staticRetriever{
		results: []retrieval.ScoredResult{{ID: "a", Title: "A", Score: 1.0}},
		total:   1,
	}

	hybrid := &query.HybridRetriever{
		Retrievers: []retrieval.NamedRetriever{{Name: "ok", Retriever: ret}, {Name: "boom", Retriever: &errorRetriever{}}},
		K:          60,
	}

	_, _, err := hybrid.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	assert.Error(t, err)
}

func TestHybridRetriever_DefaultK(t *testing.T) {
	ret := &staticRetriever{
		results: []retrieval.ScoredResult{
			{ID: "a", Title: "A", Score: 1.0},
		},
		total: 1,
	}

	hybrid := &query.HybridRetriever{
		Retrievers: []retrieval.NamedRetriever{{Name: "ret", Retriever: ret}},
		// K is 0, should default to 60
	}

	results, total, err := hybrid.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, results, 1)
	assert.Equal(t, "a", results[0].ID)
	// With K=60, single result at rank 0: score = 1/(60+0+1) = 1/61
	assert.InDelta(t, 1.0/61.0, results[0].Score, 1e-10)
}

func TestHybridRetriever_NoRetrievers(t *testing.T) {
	hybrid := &query.HybridRetriever{
		Retrievers: []retrieval.NamedRetriever{},
		K:          60,
	}

	results, total, err := hybrid.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, results)
}

func TestHybridRetriever_OffsetBeyondResults(t *testing.T) {
	ret := &staticRetriever{
		results: []retrieval.ScoredResult{
			{ID: "a", Title: "A", Score: 1.0},
		},
		total: 1,
	}

	hybrid := &query.HybridRetriever{
		Retrievers: []retrieval.NamedRetriever{{Name: "ret", Retriever: ret}},
		K:          60,
	}

	results, total, err := hybrid.Search(context.Background(), "test", 10, 100, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Empty(t, results)
}
