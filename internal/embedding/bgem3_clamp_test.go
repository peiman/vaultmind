package embedding

import (
	"testing"

	"github.com/knights-analytics/hugot/backends"
	"github.com/stretchr/testify/assert"
)

// TestSafeTokenizerLimit guards the #39 fix VALUE in CI (no model needed). The
// bge-m3 tokenizer must never be left at hugot's default (max_position_embeddings
// = 8194), which is two tokens past the usable positional ceiling and hangs the
// ORT forward. A positive budget within the ceiling is honored; everything else —
// including the "0 = no truncation" default this model cannot satisfy — falls back
// to the safe budget.
func TestSafeTokenizerLimit(t *testing.T) {
	assert.Equal(t, BGEM3MaxTokens, safeTokenizerLimit(0), "0 (no-truncation) is unsafe for bge-m3 → clamp to the budget")
	assert.Equal(t, BGEM3MaxTokens, safeTokenizerLimit(-1), "negative → clamp to the budget")
	assert.Equal(t, BGEM3MaxTokens, safeTokenizerLimit(8194), "hugot's max_position_embeddings default → clamp down")
	assert.Equal(t, BGEM3MaxTokens, safeTokenizerLimit(BGEM3MaxTokens), "exactly the budget → the budget")
	assert.Equal(t, 4096, safeTokenizerLimit(4096), "a smaller positive budget is honored")
}

// TestClampTokenizer exercises the field-setting itself model-free — deleting or
// mis-computing the clamp fails here in normal CI, not only behind the 2.2GB model
// gate (the gated integration test covers only its wiring into a real pipeline).
func TestClampTokenizer(t *testing.T) {
	tok := &backends.Tokenizer{MaxAllowedTokens: 8194} // hugot's bge-m3 default, 2 past usable
	clampTokenizer(tok, BGEM3MaxTokens)
	assert.Equal(t, BGEM3MaxTokens, tok.MaxAllowedTokens, "clamped to the safe budget")

	unsafe := &backends.Tokenizer{MaxAllowedTokens: 8194}
	clampTokenizer(unsafe, 0) // a caller passing "no truncation" would re-open the #39 hang
	assert.Equal(t, BGEM3MaxTokens, unsafe.MaxAllowedTokens, "0 falls back to the budget, not left at the unsafe default")

	assert.NotPanics(t, func() { clampTokenizer(nil, BGEM3MaxTokens) }, "nil tokenizer is a safe no-op")
}
