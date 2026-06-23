package embedding

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/knights-analytics/hugot/backends"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Pure unit tests (no model needed; always run) ---

func TestTruncateToChars(t *testing.T) {
	assert.Equal(t, "", truncateToChars("anything", 0), "zero cap returns empty")
	assert.Equal(t, "short", truncateToChars("short", 100), "under cap is unchanged")
	long := strings.Repeat("a", 200)
	assert.LessOrEqual(t, len(truncateToChars(long, 50)), 50, "over cap is capped")
	// Word-break: cuts at a space near the cap rather than mid-word.
	got := truncateToChars("alpha beta gamma delta", 12)
	assert.LessOrEqual(t, len(got), 12)
	assert.NotContains(t, got, "gamma", "tail past the cap is dropped")
}

// The #39 content class is dense / non-English; a byte-boundary cut must never
// split a multi-byte rune into invalid UTF-8.
func TestTruncateToChars_RuneSafe(t *testing.T) {
	cjk := strings.Repeat("中", 100) // 3 bytes each, no spaces to break on
	got := truncateToChars(cjk, 50) // 50 is not a multiple of 3
	assert.True(t, utf8.ValidString(got), "must not emit invalid UTF-8")
	assert.LessOrEqual(t, len(got), 50)
}

// shrinkTowardTokenBudget must ALWAYS reduce an over-budget input, and its halving
// fallback must guarantee convergence regardless of how wrong the measured ratio
// is — the property that stops #39 from ever hanging the forward pass.
func TestShrinkTowardTokenBudget_AlwaysShrinks(t *testing.T) {
	text := strings.Repeat("word ", 1000) // 5000 bytes
	got := shrinkTowardTokenBudget(text, 4000, 50, 0)
	assert.Less(t, len(got), len(text), "an over-budget input must get shorter")
}

func TestShrinkTowardTokenBudget_FallbackHalves(t *testing.T) {
	text := strings.Repeat("x", 1000)
	got := shrinkTowardTokenBudget(text, 999, 10, 5) // iter>=4 → halving fallback
	assert.InDelta(t, 500, len(got), 1, "the fallback halves the input")
}

// Convergence: repeatedly applying the shrink (worst case: the halving fallback)
// drives any input under the cap within the hardCap (16) iterations the real loop
// uses — so preprocessWithinTokenLimit can never spin forever.
func TestShrinkTowardTokenBudget_ConvergesWithinHardCap(t *testing.T) {
	text := strings.Repeat("x", 100000)
	const maxTokens = 10
	for iter := 0; iter < 16; iter++ {
		text = shrinkTowardTokenBudget(text, len(text), maxTokens, iter) // 1 char/token: worst case
		if len(text) <= maxTokens {
			return
		}
	}
	assert.LessOrEqual(t, len(text), maxTokens, "must converge within 16 iterations")
}

// --- Token-fit loop tests (model-free: the tokenizer is injected) ---

// fakeCountByLen treats each byte as one token — a deterministic stand-in for the
// real tokenizer so the fit loop is testable without the model.
func fakeCountByLen(texts []string) ([]int, error) {
	c := make([]int, len(texts))
	for i, s := range texts {
		c[i] = len(s)
	}
	return c, nil
}

func TestFitTextsWithinTokenLimit_UnderLimitUnchanged(t *testing.T) {
	got, err := fitTextsWithinTokenLimit([]string{"hi"}, 10, fakeCountByLen)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "hi", got[0], "an under-limit input is returned unchanged")
}

func TestFitTextsWithinTokenLimit_ShrinksOverLimit(t *testing.T) {
	got, err := fitTextsWithinTokenLimit([]string{strings.Repeat("a", 500)}, 10, fakeCountByLen)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(got[0]), 10, "an over-limit input is shrunk to the cap")
}

func TestFitTextsWithinTokenLimit_MixedBatch(t *testing.T) {
	got, err := fitTextsWithinTokenLimit([]string{"ok", strings.Repeat("b", 300)}, 10, fakeCountByLen)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "ok", got[0], "under-limit input untouched")
	assert.LessOrEqual(t, len(got[1]), 10, "over-limit input shrunk")
}

func TestFitTextsWithinTokenLimit_MaxTokensZeroSkips(t *testing.T) {
	called := false
	got, err := fitTextsWithinTokenLimit([]string{"anything"}, 0, func([]string) ([]int, error) { called = true; return nil, nil })
	require.NoError(t, err)
	assert.Equal(t, []string{"anything"}, got)
	assert.False(t, called, "maxTokens<=0 skips tokenization entirely")
}

func TestFitTextsWithinTokenLimit_CounterErrorPropagates(t *testing.T) {
	_, err := fitTextsWithinTokenLimit([]string{"x"}, 10, func([]string) ([]int, error) { return nil, assert.AnError })
	require.Error(t, err)
}

func TestFitTextsWithinTokenLimit_NonConvergenceErrorsNotHangs(t *testing.T) {
	// A counter that never reports within-limit must hit hardCap and ERROR — never
	// loop forever. This is the anti-hang guarantee at the loop level.
	stuck := func(ts []string) ([]int, error) {
		c := make([]int, len(ts))
		for i := range ts {
			c[i] = 9999
		}
		return c, nil
	}
	_, err := fitTextsWithinTokenLimit([]string{strings.Repeat("a", 100)}, 10, stuck)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not fit")
}

// preprocessWithinTokenLimit + tokenCounts, exercised with a FAKE preprocess (no
// model): proves the wrapper builds a within-cap batch from a mixed input.
func TestPreprocessWithinTokenLimit_FakePipeline(t *testing.T) {
	fake := func(b *backends.PipelineBatch, texts []string) error {
		b.Input = make([]backends.TokenizedInput, len(texts))
		for i, s := range texts {
			b.Input[i] = backends.TokenizedInput{TokenIDs: make([]uint32, len(s))} // 1 token/byte
		}
		return nil
	}
	e := &BGEM3Embedder{maxTokens: 10, preprocess: fake}
	batch, err := e.preprocessWithinTokenLimit([]string{"ok", strings.Repeat("a", 500)})
	require.NoError(t, err)
	defer func() { _ = batch.Destroy() }()
	require.Len(t, batch.Input, 2)
	assert.LessOrEqual(t, len(batch.Input[0].TokenIDs), 10)
	assert.LessOrEqual(t, len(batch.Input[1].TokenIDs), 10)
}

// --- ORT integration test (real tokenizer; gated by env + needs the model) ---

// On the ORT build the Rust tokenizer does not truncate, so dense content stays
// oversized past the char-estimate pre-cut (#39). This proves the token-accurate
// loop caps it via the REAL tokenizer. Self-validating: it first confirms the
// char-pre-cut input is genuinely over the cap, so it can never pass trivially.
func TestPreprocessWithinTokenLimit_CapsDenseInput(t *testing.T) {
	if os.Getenv("VAULTMIND_TEST_BGEM3") == "" {
		t.Skip("set VAULTMIND_TEST_BGEM3=1 (loads the BGE-M3 model + tokenizer)")
	}
	e, err := NewBGEM3Embedder(BGEM3Config())
	require.NoError(t, err)
	defer func() { _ = e.Close() }()

	e.maxTokens = 40
	// Dense, symbol-heavy content — the #39 class (code/markdown tokenizes well
	// below the assumed chars/token, so the char pre-cut leaves it oversized).
	dense := strings.Repeat("a1=b2;c3{d4}e5[f6]:g7,h8|i9/j0\\", 80)

	// Precondition: the char pre-cut must STILL be over the cap, or the test
	// isn't exercising the token-accurate loop (the actual fix).
	pre := TruncateForEmbedding(dense, e.maxTokens)
	rawBatch := backends.NewBatch(1)
	require.NoError(t, e.pipeline.Preprocess(rawBatch, []string{pre}))
	rawN := len(rawBatch.Input[0].TokenIDs)
	_ = rawBatch.Destroy()
	require.Greaterf(t, rawN, e.maxTokens,
		"precondition: char-pre-cut input must exceed the cap to exercise the loop (got %d tokens) — make `dense` denser", rawN)

	// The fix: the token-accurate loop brings it within the cap.
	batch, err := e.preprocessWithinTokenLimit([]string{dense})
	require.NoError(t, err)
	defer func() { _ = batch.Destroy() }()
	assert.LessOrEqualf(t, len(batch.Input[0].TokenIDs), e.maxTokens,
		"preprocessWithinTokenLimit must cap the tokenized input at maxTokens (#39 fix); got %d", len(batch.Input[0].TokenIDs))

	// Mixed batch (under + over): exercises the per-element loop — only the
	// over-limit input is shrunk, and every input ends within the cap.
	mixed, err := e.preprocessWithinTokenLimit([]string{"hello world", dense})
	require.NoError(t, err)
	defer func() { _ = mixed.Destroy() }()
	require.Len(t, mixed.Input, 2)
	assert.LessOrEqual(t, len(mixed.Input[0].TokenIDs), e.maxTokens, "short input stays within cap")
	assert.LessOrEqual(t, len(mixed.Input[1].TokenIDs), e.maxTokens, "dense input is capped")
}
