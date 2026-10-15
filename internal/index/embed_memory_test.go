package index

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BGE-M3 forward memory grows with the padded tokens in a batch. The planner
// keeps every batch inside one cap-length note's worth of padded tokens,
// while short notes still share a batch.

func estCost(lengths []int, idxs []int) int {
	most := 0
	for _, i := range idxs {
		if t := bgem3TokenEstimate(lengths[i]); t > most {
			most = t
		}
	}
	return len(idxs) * most
}

func TestPlanBGEM3Batches_KeepsEveryBatchInsideTheAttentionBudget(t *testing.T) {
	lens := []int{100, 50000, 3000, 200, 9000, 400, 400, 400, 400, 400, 400, 400, 400, 120, 6000}
	batches := planBGEM3Batches(lens)
	require.NotEmpty(t, batches)
	for _, b := range batches {
		assert.LessOrEqual(t, estCost(lens, b), bgem3EmbedTokens, "batch %v exceeds one cap-length note of padded tokens", b)
		assert.LessOrEqual(t, len(b), bgem3MaxBatch)
	}
}

func TestPlanBGEM3Batches_ACapLengthNoteRunsAlone(t *testing.T) {
	lens := []int{50000, 100, 100}
	var capBatch []int
	for _, b := range planBGEM3Batches(lens) {
		if slices.Contains(b, 0) {
			capBatch = b
		}
	}
	assert.Equal(t, []int{0}, capBatch, "a note at the token cap must run alone")
}

func TestPlanBGEM3Batches_ShortNotesStillShareABatch(t *testing.T) {
	lens := make([]int, 20)
	for i := range lens {
		lens[i] = 200
	}
	batches := planBGEM3Batches(lens)
	assert.Len(t, batches, 3, "20 short notes batch up to the count cap")
	assert.Len(t, batches[0], bgem3MaxBatch)
}

func TestPlanBGEM3Batches_CoversEveryNoteExactlyOnce(t *testing.T) {
	lens := []int{10, 70000, 300, 300, 5000, 0, 8191, 2}
	seen := map[int]int{}
	for _, b := range planBGEM3Batches(lens) {
		for _, i := range b {
			seen[i]++
		}
	}
	require.Len(t, seen, len(lens))
	for i := range lens {
		assert.Equal(t, 1, seen[i], "note %d", i)
	}
	assert.Nil(t, planBGEM3Batches(nil))
}

func TestBGEM3TokenEstimate_IsConservativeAndCapped(t *testing.T) {
	assert.Equal(t, 0, bgem3TokenEstimate(0))
	assert.Equal(t, 50, bgem3TokenEstimate(100), "two characters per token over-counts English")
	assert.Equal(t, bgem3EmbedTokens, bgem3TokenEstimate(1_000_000), "text past the cap is truncated to the cap")
}
