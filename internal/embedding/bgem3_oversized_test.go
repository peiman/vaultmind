package embedding

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/knights-analytics/hugot/backends"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBGEM3_OversizedNoteEmbedsWithoutOOB is the #39 regression. hugot clamps the
// tokenizer at the model's max_position_embeddings (8194 for bge-m3), but
// XLM-RoBERTa derives position IDs with a padding_idx+1 offset, so a full-length
// sequence indexes PAST the position-embedding table and the ORT forward wedges on
// the position_embeddings Gather node (patterns.md, ~12k tokens, hung exactly
// there). NewBGEM3Embedder now clamps the tokenizer's own limit to the token
// budget, safely below the positional ceiling.
//
// This drives an oversized note through the REAL tokenizer and forward — the path
// a model-free test cannot reach, which is the gap that let an earlier stderr-only
// fix look complete while the compute hang survived.
func TestBGEM3_OversizedNoteEmbedsWithoutOOB(t *testing.T) {
	if os.Getenv("VAULTMIND_TEST_BGEM3") == "" {
		t.Skip("set VAULTMIND_TEST_BGEM3=1 (loads the BGE-M3 model + tokenizer)")
	}
	e, err := NewBGEM3Embedder(BGEM3Config())
	require.NoError(t, err)
	defer func() { _ = e.Close() }()

	// The clamp itself: the tokenizer's hard limit must sit at our budget, not the
	// model's positional ceiling (which is what OOBs).
	require.NotNil(t, e.pipeline.Model)
	require.NotNil(t, e.pipeline.Model.Tokenizer)
	assert.LessOrEqualf(t, e.pipeline.Model.Tokenizer.MaxAllowedTokens, BGEM3MaxTokens,
		"tokenizer MaxAllowedTokens must be clamped to <= %d, below max_position_embeddings", BGEM3MaxTokens)

	// A note far longer than the limit. Tokenize it DIRECTLY (bypassing the char/
	// token guard) so the tokenizer's own clamp is what must hold. Self-validating:
	// an over-limit note must come back capped exactly at the budget — a note under
	// the limit would fail this equality, so the test can't pass trivially.
	huge := strings.Repeat("the quick brown fox jumps over the lazy dog while nimble foxes ponder syntax trees. ", 4000)
	rawBatch := backends.NewBatch(1)
	require.NoError(t, e.pipeline.Preprocess(rawBatch, []string{huge}))
	defer func() { _ = rawBatch.Destroy() }()
	got := len(rawBatch.Input[0].TokenIDs)
	require.Equalf(t, BGEM3MaxTokens, got,
		"tokenizer must clamp an oversized input to exactly %d tokens (got %d); above the positional limit the ORT forward OOBs", BGEM3MaxTokens, got)

	// Drive the ORT forward at exactly the clamped boundary — the #39 surface.
	// Without the clamp this batch is 8194 tokens and the position_embeddings Gather
	// indexes out of bounds (hang on some ORT builds, error on others); at the
	// clamped length it must run cleanly. The char guard cannot reach this on the
	// EmbedFull path (it pre-cuts English prose well below the boundary), so it is
	// asserted directly on the tokenized batch.
	require.NoErrorf(t, e.pipeline.Forward(rawBatch),
		"forward must run at the clamped boundary (%d tokens) without indexing past the position embeddings (#39)", got)

	// And the full public path embeds an oversized note without error.
	out, err := e.EmbedFull(context.Background(), huge)
	require.NoError(t, err, "oversized note must embed via EmbedFull (#39)")
	require.NotNil(t, out)
	assert.Len(t, out.Dense, BGEM3Dims)
}
