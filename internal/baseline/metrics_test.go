package baseline_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/baseline"
	"github.com/stretchr/testify/assert"
)

// HitAtK: k notes returned, expected set of note IDs; returns 1 if any
// expected ID appears in the top k, else 0. Float return keeps the
// aggregation simple (mean across queries).
//
// The contract is strict-intersection: a query "hits" if *any* curated
// expected ID appears at rank ≤ k. Any other semantics (all-expected,
// strict-order) would mask partial-recall wins that are the point of
// keyword/hybrid retrieval in practice.

func TestHitAtK_ExpectedInTopK(t *testing.T) {
	results := []string{"n1", "n2", "n3"}
	expected := []string{"n2"}
	assert.Equal(t, 1.0, baseline.HitAtK(results, expected, 3))
	assert.Equal(t, 1.0, baseline.HitAtK(results, expected, 2), "exact rank still counts")
}

func TestHitAtK_ExpectedBeyondK(t *testing.T) {
	results := []string{"n1", "n2", "n3"}
	expected := []string{"n3"}
	assert.Equal(t, 0.0, baseline.HitAtK(results, expected, 2),
		"expected at rank 3 must NOT count for k=2")
}

func TestHitAtK_NoOverlap(t *testing.T) {
	results := []string{"a", "b", "c"}
	expected := []string{"x", "y"}
	assert.Equal(t, 0.0, baseline.HitAtK(results, expected, 3))
}

func TestHitAtK_AnyMatchCounts(t *testing.T) {
	// 1 of 3 expected IDs appears in top-k; this must count as a hit.
	// Regression guard against a naive "all expected must match" impl.
	results := []string{"n1", "n2"}
	expected := []string{"n2", "n99", "n42"}
	assert.Equal(t, 1.0, baseline.HitAtK(results, expected, 2))
}

func TestHitAtK_EmptyResultsIsMiss(t *testing.T) {
	assert.Equal(t, 0.0, baseline.HitAtK([]string{}, []string{"n1"}, 5))
}

func TestHitAtK_EmptyExpectedIsMiss(t *testing.T) {
	// A query with an empty expected-set can't hit anything. We return 0
	// rather than NaN so downstream aggregation (mean) stays well-defined.
	assert.Equal(t, 0.0, baseline.HitAtK([]string{"a", "b"}, []string{}, 5))
}

func TestHitAtK_KLargerThanResults(t *testing.T) {
	// k=10 but only 2 results: we scan all of them; no out-of-bounds.
	results := []string{"n1", "n2"}
	expected := []string{"n2"}
	assert.Equal(t, 1.0, baseline.HitAtK(results, expected, 10))
}

// MRR (Mean Reciprocal Rank) for a single query: 1/rank of the *first*
// expected ID found in the results, or 0 if none. "Mean" happens at the
// aggregate layer — a single query's MRR is its reciprocal rank.
//
// The "first expected" tiebreak is deliberate: if two expected IDs both
// match, we want the *best* rank to register. Losing this tiebreak would
// underreport high-quality retrievals.

func TestMRR_FirstExpectedAtRank1(t *testing.T) {
	results := []string{"n1", "n2"}
	expected := []string{"n1"}
	assert.Equal(t, 1.0, baseline.ReciprocalRank(results, expected))
}

func TestMRR_ExpectedAtRank2(t *testing.T) {
	results := []string{"x", "n1"}
	expected := []string{"n1"}
	assert.InDelta(t, 0.5, baseline.ReciprocalRank(results, expected), 1e-9)
}

func TestMRR_NoMatch(t *testing.T) {
	assert.Equal(t, 0.0, baseline.ReciprocalRank([]string{"a", "b"}, []string{"x"}))
}

func TestMRR_MultipleExpectedUsesBestRank(t *testing.T) {
	// Expected IDs are n3 and n1 in that declaration order; results
	// have n1 at rank 1 and n3 at rank 3. MRR must use the *best* rank (1),
	// not the first-declared rank (3) — otherwise ordering of the expected
	// set would silently change the metric.
	results := []string{"n1", "other", "n3"}
	expected := []string{"n3", "n1"}
	assert.Equal(t, 1.0, baseline.ReciprocalRank(results, expected),
		"MRR must use the best rank among expected matches, not first-declared")
}

func TestMRR_EmptyResultsIsZero(t *testing.T) {
	assert.Equal(t, 0.0, baseline.ReciprocalRank([]string{}, []string{"n1"}))
}
