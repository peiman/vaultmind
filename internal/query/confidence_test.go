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
	assert.Equal(t, "", computeTopHitConfidence(nil), "nil hits → empty confidence")
	assert.Equal(t, "", computeTopHitConfidence([]retrieval.ScoredResult{}), "zero hits → empty confidence")
	assert.Equal(t, "", computeTopHitConfidence([]retrieval.ScoredResult{
		{ID: "a", Score: 0.5},
	}), "single hit → empty confidence (no comparison possible)")
}

// Non-positive top-1 score makes the relative-gap denominator ill-defined.
// Return "" rather than guessing a tier from numerically meaningless input.
func TestComputeTopHitConfidence_NonPositiveTopScore(t *testing.T) {
	assert.Equal(t, "", computeTopHitConfidence([]retrieval.ScoredResult{
		{ID: "a", Score: 0},
		{ID: "b", Score: -1},
	}), "zero top score → empty confidence")
	assert.Equal(t, "", computeTopHitConfidence([]retrieval.ScoredResult{
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
	assert.Equal(t, ConfidenceStrong, computeTopHitConfidence(hits))

	// Exactly at the 5% threshold — should be strong (>=)
	hits2 := []retrieval.ScoredResult{
		{ID: "a", Score: 1.0},
		{ID: "b", Score: 0.95}, // exactly 5%
	}
	assert.Equal(t, ConfidenceStrong, computeTopHitConfidence(hits2))
}

// Moderate tier: 2-5% relative gap — top-1 leads but not decisively.
// The agent should consider the next hit too but probably trust top-1.
func TestComputeTopHitConfidence_Moderate(t *testing.T) {
	hits := []retrieval.ScoredResult{
		{ID: "a", Score: 0.10},
		{ID: "b", Score: 0.097}, // 3% gap
	}
	assert.Equal(t, ConfidenceModerate, computeTopHitConfidence(hits))
}

// Weak tier: <2% relative gap — top-1 might be coincidental.
// Empirically this matches the failure-mode queries on the baselines
// (research rank-6 miss at 1.60%, identity rank-2 case at 1.94%). The
// agent should treat top-N as candidates rather than committing to top-1.
func TestComputeTopHitConfidence_Weak(t *testing.T) {
	hits := []retrieval.ScoredResult{
		{ID: "a", Score: 0.01622},
		{ID: "b", Score: 0.01596}, // 1.60% — the actual research rank-6 case
	}
	assert.Equal(t, ConfidenceWeak, computeTopHitConfidence(hits))

	hits2 := []retrieval.ScoredResult{
		{ID: "a", Score: 0.01613},
		{ID: "b", Score: 0.01582}, // 1.92% — the actual identity rank-2 case
	}
	assert.Equal(t, ConfidenceWeak, computeTopHitConfidence(hits2))
}

// Tied scores → 0% gap → weak. The agent definitely shouldn't commit to
// top-1 when top-2 has the same score.
func TestComputeTopHitConfidence_TiedScores(t *testing.T) {
	hits := []retrieval.ScoredResult{
		{ID: "a", Score: 0.5},
		{ID: "b", Score: 0.5},
	}
	assert.Equal(t, ConfidenceWeak, computeTopHitConfidence(hits))
}

// Regression-pin: the exact gap percentages from the 2026-04-29 probes
// must classify as expected. If the thresholds change later, this test
// must be updated explicitly — that's the point.
func TestComputeTopHitConfidence_ProbedQueries_2026_04_29(t *testing.T) {
	tests := []struct {
		name string
		top1 float64
		top2 float64
		want string
	}{
		// "Hebbian learning" on research vault → canonical match
		{"hebbian-canonical", 0.01639, 0.01498, ConfidenceStrong},
		// "structured schema versus pure embeddings" → rank-6 miss
		{"structured-rank6-miss", 0.01622, 0.01596, ConfidenceWeak},
		// "what is the roadmap I committed to" → identity rank-2 case
		{"roadmap-paraphrase-rank2", 0.01613, 0.01582, ConfidenceWeak},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := []retrieval.ScoredResult{
				{ID: "a", Score: tt.top1},
				{ID: "b", Score: tt.top2},
			}
			assert.Equal(t, tt.want, computeTopHitConfidence(hits))
		})
	}
}
