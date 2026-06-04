package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRetriever returns a fixed list of ScoredResult with descending RRF
// scores, mimicking what a 4-way HybridRetriever would emit. Used to drive
// the reranker without spinning up real retrievers.
type stubRetriever struct {
	results []retrieval.ScoredResult
}

func (s *stubRetriever) Search(_ context.Context, _ string, limit, _ int, _ index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	if limit > 0 && len(s.results) > limit {
		return s.results[:limit], len(s.results), nil
	}
	return s.results, len(s.results), nil
}

// stubScorer is a test-only ActivationScoreFn that returns the given map.
// Decouples the reranker tests from experiment.ComputeBatchScores so the
// test surface is self-contained.
func stubScorer(scores map[string]float64) query.ActivationScoreFn {
	return func(ids []string) (map[string]float64, error) {
		out := make(map[string]float64, len(ids))
		for _, id := range ids {
			if v, ok := scores[id]; ok {
				out[id] = v
			}
		}
		return out, nil
	}
}

func mkResult(id string, score float64) retrieval.ScoredResult {
	return retrieval.ScoredResult{ID: id, Score: score, IsDomain: true}
}

// TestActivationReranker_NoAccessedNotes — when the candidate set has
// zero accessed notes (cold-start vault, or any query whose top-N is all
// untouched), the reranker MUST return the base retriever's order
// unchanged. Activation contributes 0; final = α * rrf_norm.
func TestActivationReranker_NoAccessedNotes(t *testing.T) {
	base := &stubRetriever{results: []retrieval.ScoredResult{
		mkResult("a", 0.05), mkResult("b", 0.04), mkResult("c", 0.03),
	}}
	rer := &query.ActivationReranker{
		Base:   base,
		Score:  stubScorer(map[string]float64{}), // no accessed notes
		Alpha:  0.7,
		Beta:   0.3,
		FetchN: 10,
	}
	results, _, err := rer.Search(context.Background(), "q", 3, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "a", results[0].ID, "rank 1 must match 4-way when no activation signal")
	assert.Equal(t, "b", results[1].ID)
	assert.Equal(t, "c", results[2].ID)
}

// TestActivationReranker_ActivationLiftsWithinCandidates — a candidate
// with strong activation rises within the candidate set. Critical: the
// candidate must already be in the 4-way top-N — activation cannot
// introduce notes from outside (that's the structural fix from slice 5b”).
func TestActivationReranker_ActivationLiftsWithinCandidates(t *testing.T) {
	// Base order: a, b, c with close RRF scores.
	base := &stubRetriever{results: []retrieval.ScoredResult{
		mkResult("a", 0.020), mkResult("b", 0.018), mkResult("c", 0.016),
	}}
	// b has dominant activation; a, c untouched.
	rer := &query.ActivationReranker{
		Base:   base,
		Score:  stubScorer(map[string]float64{"b": 1.0}),
		Alpha:  0.5,
		Beta:   0.5,
		FetchN: 10,
	}
	results, _, err := rer.Search(context.Background(), "q", 3, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "b", results[0].ID, "b should be lifted to rank 1 by activation under α=β=0.5")
}

// TestActivationReranker_ActivationCannotIntroduceCandidates — a note
// not in the base retriever's top-N must not appear in the rerank output.
// This is the load-bearing structural property: drown-out is impossible
// because activation operates only on candidates that survived 4-way.
func TestActivationReranker_ActivationCannotIntroduceCandidates(t *testing.T) {
	base := &stubRetriever{results: []retrieval.ScoredResult{
		mkResult("a", 0.05), mkResult("b", 0.04),
	}}
	// "z" has dominant activation but is NOT in the base result set.
	rer := &query.ActivationReranker{
		Base:   base,
		Score:  stubScorer(map[string]float64{"z": 100.0, "a": 0.1, "b": 0.1}),
		Alpha:  0.0,
		Beta:   1.0, // pure activation
		FetchN: 10,
	}
	results, _, err := rer.Search(context.Background(), "q", 5, 0, index.SearchFilters{})
	require.NoError(t, err)
	for _, r := range results {
		assert.NotEqual(t, "z", r.ID, "activation must not introduce notes outside the base candidate set")
	}
}

// TestActivationReranker_AlphaOne_PreservesBaseOrder — α=1.0 reduces
// the rerank to identity-on-base. Pins the contract that disabling
// activation (β=0) gives back the unmodified 4-way ranking.
func TestActivationReranker_AlphaOne_PreservesBaseOrder(t *testing.T) {
	base := &stubRetriever{results: []retrieval.ScoredResult{
		mkResult("a", 0.05), mkResult("b", 0.04), mkResult("c", 0.03),
	}}
	rer := &query.ActivationReranker{
		Base:   base,
		Score:  stubScorer(map[string]float64{"c": 100.0}),
		Alpha:  1.0,
		Beta:   0.0,
		FetchN: 10,
	}
	results, _, err := rer.Search(context.Background(), "q", 3, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, []string{"a", "b", "c"}, []string{results[0].ID, results[1].ID, results[2].ID})
}

// TestActivationReranker_BetaOne_OrdersByActivation — β=1.0 orders the
// candidate set by pure activation score. Confirms the weight extremes
// are predictable.
func TestActivationReranker_BetaOne_OrdersByActivation(t *testing.T) {
	base := &stubRetriever{results: []retrieval.ScoredResult{
		mkResult("a", 0.05), mkResult("b", 0.04), mkResult("c", 0.03),
	}}
	rer := &query.ActivationReranker{
		Base:   base,
		Score:  stubScorer(map[string]float64{"a": 0.1, "b": 1.0, "c": 0.5}),
		Alpha:  0.0,
		Beta:   1.0,
		FetchN: 10,
	}
	results, _, err := rer.Search(context.Background(), "q", 3, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.Len(t, results, 3)
	// b has highest activation, c next, a lowest.
	assert.Equal(t, "b", results[0].ID)
	assert.Equal(t, "c", results[1].ID)
	assert.Equal(t, "a", results[2].ID)
}

// TestActivationReranker_FetchNExpandsCandidates — when limit (K) is
// smaller than FetchN, the reranker fetches FetchN candidates from base
// and reranks them all, returning the top K. This is what gives
// activation room to lift candidates from rank N+1 to top-K.
func TestActivationReranker_FetchNExpandsCandidates(t *testing.T) {
	// Base returns 5 results in descending RRF order.
	base := &stubRetriever{results: []retrieval.ScoredResult{
		mkResult("a", 0.05), mkResult("b", 0.04), mkResult("c", 0.03),
		mkResult("d", 0.02), mkResult("e", 0.01),
	}}
	// "e" (last in base) has dominant activation. Should rank into top-3.
	rer := &query.ActivationReranker{
		Base:   base,
		Score:  stubScorer(map[string]float64{"e": 1.0}),
		Alpha:  0.0,
		Beta:   1.0,
		FetchN: 5,
	}
	results, _, err := rer.Search(context.Background(), "q", 3, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "e", results[0].ID, "FetchN=5 with K=3 must rerank all 5 and return top-3 by combined score")
}

// TestActivationReranker_DefaultsAreReasonable — zero-value Alpha/Beta/
// FetchN fall back to sane defaults so a caller that constructs the
// reranker without explicit knobs still gets the documented behavior
// (RRF-dominant with a soft activation lift).
func TestActivationReranker_DefaultsAreReasonable(t *testing.T) {
	base := &stubRetriever{results: []retrieval.ScoredResult{
		mkResult("a", 0.05), mkResult("b", 0.04),
	}}
	rer := &query.ActivationReranker{
		Base:  base,
		Score: stubScorer(map[string]float64{}),
		// Alpha, Beta, FetchN intentionally zero-value
	}
	results, _, err := rer.Search(context.Background(), "q", 2, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "a", results[0].ID, "with no activation and zero-value weights, defaults must preserve base order")
}

// TestActivationReranker_EmptyBase — base returning zero results is
// passed through; reranker doesn't fabricate candidates. Trivial guard
// against div-by-zero in normalization.
func TestActivationReranker_EmptyBase(t *testing.T) {
	base := &stubRetriever{results: []retrieval.ScoredResult{}}
	rer := &query.ActivationReranker{
		Base:   base,
		Score:  stubScorer(map[string]float64{"a": 1.0}),
		Alpha:  0.5,
		Beta:   0.5,
		FetchN: 10,
	}
	results, total, err := rer.Search(context.Background(), "q", 3, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.Equal(t, 0, total)
}

// _ silence unused import — experiment used elsewhere in the package.
var _ = experiment.DefaultActivationParams
