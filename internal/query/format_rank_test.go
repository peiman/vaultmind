package query

import (
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each hit line leads with its rank, not the fused RRF score. That score
// printed as 0.02 for nearly every hit — it carries order only, and read as a
// relevance number it said everything was equally weak. Relevance is in the
// header; the raw score stays in --json and --explain.
func TestAskHits_LeadWithTheRankNotTheFusedScore(t *testing.T) {
	var b strings.Builder
	hits := []retrieval.ScoredResult{
		{ID: "concept-a", Title: "Ay", Score: 0.0203},
		{ID: "concept-b", Title: "Bee", Score: 0.0198},
	}
	require.NoError(t, writeAskHits(&b, hits, formatOpts{}))

	out := b.String()
	assert.Contains(t, out, "   1.  concept-a")
	assert.Contains(t, out, "   2.  concept-b")
	assert.NotContains(t, out, "0.02", "the fused score is not shown as if it were relevance")
}
