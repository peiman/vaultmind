package query

import (
	"testing"

	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
)

// computeTopHitConfidence returns "" when fewer than 2 hits exist —
// no comparison possible. The empty value tells downstream renderers to
// hide the confidence line entirely rather than display a misleading
// "weak" / "strong" with zero data behind it.
func TestComputeTopHitConfidence_FewerThanTwoHits(t *testing.T) {
	assert.Equal(t, "", computeTopHitConfidenceRRFGap(nil), "nil hits → empty confidence")
	assert.Equal(t, "", computeTopHitConfidenceRRFGap([]retrieval.ScoredResult{}), "zero hits → empty confidence")
	assert.Equal(t, "", computeTopHitConfidenceRRFGap([]retrieval.ScoredResult{
		{ID: "a", Score: 0.5},
	}), "single hit → empty confidence (no comparison possible)")
}

// Non-positive top-1 score makes the relative-gap denominator ill-defined.
// Return "" rather than guessing a tier from numerically meaningless input.
func TestComputeTopHitConfidence_NonPositiveTopScore(t *testing.T) {
	assert.Equal(t, "", computeTopHitConfidenceRRFGap([]retrieval.ScoredResult{
		{ID: "a", Score: 0},
		{ID: "b", Score: -1},
	}), "zero top score → empty confidence")
	assert.Equal(t, "", computeTopHitConfidenceRRFGap([]retrieval.ScoredResult{
		{ID: "a", Score: -0.5},
		{ID: "b", Score: -0.6},
	}), "negative top score → empty confidence")
}

// Strong tier: top-1 dominates by 5%+ relative gap. Empirically the
// canonical-match queries on identity + research vaults sit here (e.g.
// "Hebbian learning" → top-1 is concept-hebbian-learning at 8.62% gap).
// The agent should treat top-1 as the answer.
func TestComputeTopHitConfidence_Strong(t *testing.T) {
	hits := []retrieval.ScoredResult{
		{ID: "a", Score: 0.10},
		{ID: "b", Score: 0.094}, // 6% relative gap — above 5% threshold
	}
	assert.Equal(t, ConfidenceStrong, computeTopHitConfidenceRRFGap(hits))

	// Exactly at the 5% threshold — should be strong (>=)
	hits2 := []retrieval.ScoredResult{
		{ID: "a", Score: 1.0},
		{ID: "b", Score: 0.95}, // exactly 5%
	}
	assert.Equal(t, ConfidenceStrong, computeTopHitConfidenceRRFGap(hits2))
}

// Moderate tier: 1.5–5% relative gap — top-1 leads but candidates exist.
// Empirically this is where most real-but-non-canonical queries land
// (re-probed 2026-04-30: REM sleep 3.01%, ACT-R 2.77%, place cells 2.72%,
// spreading activation 1.97%). The agent should consider top-2/3 too.
func TestComputeTopHitConfidence_Moderate(t *testing.T) {
	hits := []retrieval.ScoredResult{
		{ID: "a", Score: 0.10},
		{ID: "b", Score: 0.097}, // 3% gap
	}
	assert.Equal(t, ConfidenceModerate, computeTopHitConfidenceRRFGap(hits))

	// Exactly at the 1.5% threshold — should be moderate (>=).
	hits2 := []retrieval.ScoredResult{
		{ID: "a", Score: 1.0},
		{ID: "b", Score: 0.985}, // 1.5%
	}
	assert.Equal(t, ConfidenceModerate, computeTopHitConfidenceRRFGap(hits2))
}

// Weak tier: 0.5–1.5% relative gap — top-1 barely ahead but real signal
// still exists. Re-probed examples: synaptic plasticity 1.15%, vaultmind
// self 0.67%. Above no_match (where the gap signal stops being meaningful).
func TestComputeTopHitConfidence_Weak(t *testing.T) {
	hits := []retrieval.ScoredResult{
		{ID: "a", Score: 0.01622},
		{ID: "b", Score: 0.01604}, // 1.10% gap — synaptic plasticity area
	}
	assert.Equal(t, ConfidenceWeak, computeTopHitConfidenceRRFGap(hits))

	hits2 := []retrieval.ScoredResult{
		{ID: "a", Score: 0.01639},
		{ID: "b", Score: 0.01628}, // 0.67% — vaultmind self area
	}
	assert.Equal(t, ConfidenceWeak, computeTopHitConfidenceRRFGap(hits2))
}

// no_match tier (added 2026-04-30 after fresh-session evaluation):
// gap < 0.5% means top results are essentially tied — no clear winner.
// Empirically this catches both nonsense queries (cake-is-a-lie 0%,
// what's-the-weather 0.7%) AND ill-formed real queries that the vault
// can't disambiguate ("what is plasticity" 0.02%). The agent should
// treat the result list as candidates, not commit to top-1.
func TestComputeTopHitConfidence_NoMatch(t *testing.T) {
	hits := []retrieval.ScoredResult{
		{ID: "a", Score: 0.5},
		{ID: "b", Score: 0.499}, // 0.2% gap — effectively tied
	}
	assert.Equal(t, ConfidenceNoMatch, computeTopHitConfidenceRRFGap(hits))

	// Tied scores → 0% gap → no_match (was "weak" pre-retune).
	hits2 := []retrieval.ScoredResult{
		{ID: "a", Score: 0.5},
		{ID: "b", Score: 0.5},
	}
	assert.Equal(t, ConfidenceNoMatch, computeTopHitConfidenceRRFGap(hits2))
}

// Regression-pin: the exact gap percentages from the 2026-04-30 re-probes
// must classify under the retuned thresholds. The probes spanned 19
// queries (canonical, real, paraphrase, gibberish) and produced the gap
// distribution that drove the 5% / 1.5% / 0.5% / no_match scheme.
//
// Re-probed 2026-05-04 against the post-retraction substrate (vm_updated
// stripped, embeddings regenerated bge-m3-uniform). 8 of 9 probe gaps
// reproduced to the second decimal; one shifted within tier. Thresholds
// held without code change. See `vaultmind-identity/references/
// tophit-reprobe-2026-05-04.md` for the full result table and the
// generalizable insight ("uniform-content additions/removals don't
// require threshold re-calibration").
func TestComputeTopHitConfidence_ProbedQueries_2026_04_30(t *testing.T) {
	tests := []struct {
		name string
		top1 float64
		top2 float64
		want string
	}{
		// "Hebbian learning" — canonical match → strong (5.66%)
		{"hebbian-canonical", 0.01639, 0.01546, ConfidenceStrong},
		// "memory consolidation" — good match → moderate (4.1%)
		{"memory-consolidation-good", 0.01639, 0.01572, ConfidenceModerate},
		// "spreading activation" — was-falsely-weak, now moderate (1.97%)
		{"spreading-activation-fixed", 0.01626, 0.01594, ConfidenceModerate},
		// "synaptic plasticity" — real but close → weak (1.15%)
		{"synaptic-plasticity-close", 0.01639, 0.01620, ConfidenceWeak},
		// "what is plasticity" — multiple ties, no winner → no_match (0.02%)
		{"plasticity-no-winner", 0.01622, 0.016216, ConfidenceNoMatch},
		// "the cake is a lie" — pure nonsense, no winner → no_match (0%)
		{"nonsense-perfect-tie", 0.0156, 0.0156, ConfidenceNoMatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := []retrieval.ScoredResult{
				{ID: "a", Score: tt.top1},
				{ID: "b", Score: tt.top2},
			}
			assert.Equal(t, tt.want, computeTopHitConfidenceRRFGap(hits))
		})
	}
}
