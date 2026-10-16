package embedding

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A BGE-M3 batch may hold at most BGEM3MaxBatch texts and at most one
// cap-length note's worth of padded tokens. v0.10.13 sent batches of 32 long
// notes and one process passed 100 GB.

func TestBGEM3BatchFits(t *testing.T) {
	assert.True(t, BGEM3BatchFits(1, 1_000_000), "one note always fits: the cap truncates it")
	assert.True(t, BGEM3BatchFits(BGEM3MaxBatch, 200), "eight short notes share a batch")
	assert.False(t, BGEM3BatchFits(BGEM3MaxBatch+1, 10), "the count cap holds for tiny notes")
	assert.False(t, BGEM3BatchFits(2, 2*BGEM3EmbedMaxTokens), "two cap-length notes never share a batch")
	assert.False(t, BGEM3BatchFits(0, 0), "an empty batch is not a batch")
}

func TestBGEM3TokenEstimate_IsConservativeAndCapped(t *testing.T) {
	assert.Equal(t, 0, BGEM3TokenEstimate(0))
	assert.Equal(t, 50, BGEM3TokenEstimate(100), "two characters per token over-counts English")
	assert.Equal(t, BGEM3EmbedMaxTokens, BGEM3TokenEstimate(1_000_000), "text past the cap is truncated to the cap")
}

// Every BGE-M3 batch reaches the model through EmbedFullBatch. Refusing an
// over-budget batch there bounds memory for every caller, including ones not
// written yet. The check runs before the model is touched: a zero-value
// embedder has no pipeline, so reaching it would panic.
func TestBGEM3Embedder_RefusesABatchOverTheMemoryBudget(t *testing.T) {
	cases := overBudgetBatches()
	require.NotEmpty(t, cases)
	for name, texts := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := (&BGEM3Embedder{}).EmbedFullBatch(context.Background(), texts)
			require.ErrorIs(t, err, ErrBatchOverMemoryBudget)
		})
	}
}

func TestSidecarBGEM3Embedder_RefusesABatchOverTheMemoryBudget(t *testing.T) {
	cases := overBudgetBatches()
	require.NotEmpty(t, cases)
	for name, texts := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := (&SidecarBGEM3Embedder{}).EmbedFullBatch(context.Background(), texts)
			require.ErrorIs(t, err, ErrBatchOverMemoryBudget)
		})
	}
}

func overBudgetBatches() map[string][]string {
	many := make([]string, BGEM3MaxBatch+1)
	for i := range many {
		many[i] = "short"
	}
	long := strings.Repeat("a", 2*BGEM3EmbedMaxTokens)
	return map[string][]string{
		"too many notes":             many,
		"two cap-length notes":       {long, long},
		"one long note among shorts": {long, "short"},
	}
}
