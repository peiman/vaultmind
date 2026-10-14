package embedding

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gomlx/go-huggingface/tokenizers/api"
	"github.com/gomlx/go-huggingface/tokenizers/hftokenizer"
	"github.com/knights-analytics/hugot/backends"
	"github.com/knights-analytics/hugot/pipelines"
	"github.com/stretchr/testify/require"
)

func TestEnableContentTokenSpans_CJKRunIsManyTokens(t *testing.T) {
	raw := readBGEM3Tokenizer(t)
	shared, err := hftokenizer.NewFromContent(nil, raw)
	require.NoError(t, err)
	// The loader's option set: specials on, spans off.
	require.NoError(t, shared.With(api.EncodeOptions{AddSpecialTokens: true, MaxLen: 8192}))
	spanTok, err := querySpanTokenizerFromBytes(raw, 8192)
	require.NoError(t, err)

	text := strings.Repeat("日本語の文章", 40)
	require.Equal(t, 1, len(strings.Fields(text)))
	bare := shared.EncodeWithAnnotations(text)
	require.Empty(t, bare.Spans)
	require.Empty(t, bare.SpecialTokensMask)

	e := &BGEM3Embedder{spanTok: spanTok}
	spans, err := e.TokenSpans(text, 0)
	require.NoError(t, err)
	require.Greater(t, len(spans), len(strings.Fields(text)))
	for _, sp := range spans {
		require.Greater(t, sp[1], sp[0])
		require.LessOrEqual(t, sp[1], len(text))
		require.True(t, strings.HasPrefix(text, text[:sp[1]]))
	}
	// The shared tokenizer is the index and short-query tokenizer. A span
	// read must not replace its options: IncludeSpans and
	// IncludeSpecialTokensMask stay off, as on main.
	after := shared.EncodeWithAnnotations("a note about the index")
	require.Empty(t, after.Spans)
	require.Empty(t, after.SpecialTokensMask)
}

// TestSharedPreprocess_OptionsMatchMain is the index-time byte-identity
// check. The shared preprocess must pass the loader's options through:
// AddSpecialTokens on, IncludeSpans off, IncludeSpecialTokensMask off.
// Token ids for a document then match a fresh tokenizer, and the batch
// carries an empty special mask and empty offsets — the inputs
// SparsePositionWeights saw on main.
func TestSharedPreprocess_OptionsMatchMain(t *testing.T) {
	raw := readBGEM3Tokenizer(t)
	doc := "A short note about retrieval and the index format."

	shared := tokenizerWithMainOptions(t, raw)
	fresh := tokenizerWithMainOptions(t, raw)

	p := &pipelines.FeatureExtractionPipeline{
		BasePipeline: &backends.BasePipeline{
			Model: &backends.Model{
				Tokenizer: shared,
			},
		},
	}
	batch := backends.NewBatch(1)
	t.Cleanup(func() { _ = batch.Destroy() })
	// CreateInputTensors fails with no backend. TokenizeInputs has already
	// filled the batch, which is the output this check compares.
	err := preprocessFor(p)(batch, []string{doc})
	require.Error(t, err)
	require.Contains(t, err.Error(), "pipeline backend is not configured")
	require.Len(t, batch.Input, 1)

	mainBatch := backends.NewBatch(1)
	t.Cleanup(func() { _ = mainBatch.Destroy() })
	backends.TokenizeInputs(mainBatch, fresh, []string{doc})
	require.Len(t, mainBatch.Input, 1)

	require.Equal(t, mainBatch.Input[0].TokenIDs, batch.Input[0].TokenIDs)
	require.Equal(t, mainBatch.Input[0].Tokens, batch.Input[0].Tokens)
	require.Empty(t, batch.Input[0].SpecialTokensMask)
	require.Empty(t, batch.Input[0].Offsets)
	require.Empty(t, mainBatch.Input[0].SpecialTokensMask)
	require.Empty(t, mainBatch.Input[0].Offsets)

	ann := shared.GoTokenizer.Tokenizer.EncodeWithAnnotations(doc)
	require.Empty(t, ann.Spans)
	require.Empty(t, ann.SpecialTokensMask)
}

// TestTokenSpans_PreCutBoundsTheRead: the query-only span read must not
// tokenize a multi-megabyte input whole. The window is the forward bound
// passed in, times querySpanCharsPerToken, not the model cap on the embedder.
// Ignoring the argument and sizing from maxTokens fails this test: the
// embedder's cap is BGEM3MaxTokens, and the bound under test is 8.
func TestTokenSpans_PreCutBoundsTheRead(t *testing.T) {
	require.Equal(t, 8, querySpanCharsPerToken)
	raw := readBGEM3Tokenizer(t)
	spanTok, err := querySpanTokenizerFromBytes(raw, 8192)
	require.NoError(t, err)
	text := strings.Repeat("a", 4000)
	const bound = 8
	e := &BGEM3Embedder{maxTokens: BGEM3MaxTokens, spanTok: spanTok}
	spans, err := e.TokenSpans(text, bound)
	require.NoError(t, err)
	require.NotEmpty(t, spans)
	limit := bound * querySpanCharsPerToken
	for _, sp := range spans {
		require.LessOrEqual(t, sp[1], limit)
	}
}

func TestNewSpanEmbedder_ReadsSpansWithoutTheModel(t *testing.T) {
	raw := readBGEM3Tokenizer(t)
	e, err := NewSpanEmbedder(raw, 0)
	require.NoError(t, err)
	require.Equal(t, BGEM3MaxTokens, e.maxTokens)
	spans, err := e.TokenSpans("hello world", 0)
	require.NoError(t, err)
	require.NotEmpty(t, spans)
	_, err = NewSpanEmbedder([]byte("not-json"), BGEM3MaxTokens)
	require.Error(t, err)
}

func TestQuerySpanWindow_CutsByCharacters(t *testing.T) {
	long := strings.Repeat("a", 4000)
	require.Equal(t, "", querySpanWindow("", 8))
	require.Equal(t, long, querySpanWindow(long, 0))
	require.Equal(t, long, querySpanWindow(long, -1))
	got := querySpanWindow(long, 8)
	require.Equal(t, 8*querySpanCharsPerToken, len(got))
	require.True(t, strings.HasPrefix(long, got))
}

// TestSharedTokenizer_SpanReadLeavesOptions loads the real pipeline and
// checks that a query span read leaves the shared tokenizer's options where
// the loader put them. Gated like the other model tests.
func TestSharedTokenizer_SpanReadLeavesOptions(t *testing.T) {
	if os.Getenv("VAULTMIND_TEST_BGEM3") == "" {
		t.Skip("set VAULTMIND_TEST_BGEM3=1 (loads the BGE-M3 model + tokenizer)")
	}
	e, err := NewBGEM3Embedder(context.Background(), BGEM3Config())
	require.NoError(t, err)
	defer func() { _ = e.Close() }()

	doc := "A short note about retrieval and the index format."
	tok := e.pipeline.Model.Tokenizer.GoTokenizer.Tokenizer
	before := tok.EncodeWithAnnotations(doc)
	require.Empty(t, before.Spans)
	require.Empty(t, before.SpecialTokensMask)

	spans, err := e.TokenSpans(strings.Repeat("日本語の文章", 40), 0)
	require.NoError(t, err)
	require.NotEmpty(t, spans)

	after := tok.EncodeWithAnnotations(doc)
	require.Equal(t, before.IDs, after.IDs)
	require.Empty(t, after.Spans)
	require.Empty(t, after.SpecialTokensMask)

	batch := backends.NewBatch(1)
	t.Cleanup(func() { _ = batch.Destroy() })
	require.NoError(t, preprocessFor(e.pipeline)(batch, []string{doc}))
	require.Empty(t, batch.Input[0].SpecialTokensMask)
	require.Empty(t, batch.Input[0].Offsets)
}

func readBGEM3Tokenizer(t *testing.T) []byte {
	t.Helper()
	raw, err := ReadBGEM3Tokenizer()
	if err != nil {
		msg := TokenizerFetchHint() + ": " + err.Error()
		if TokenizerMissIsFatal(os.Getenv("CI")) {
			t.Fatal(msg)
		}
		t.Skip(msg)
	}
	return raw
}

func tokenizerWithMainOptions(t *testing.T, raw []byte) *backends.Tokenizer {
	t.Helper()
	tk, err := hftokenizer.NewFromContent(nil, raw)
	require.NoError(t, err)
	// Main's loader: specials on, spans off, special-token mask off.
	// MaxLen is the model's max_position_embeddings (8194); the test uses
	// the same 8192 the CJK fixture already pinned, which does not change
	// ids for a short document. hftokenizer does not apply MaxLen.
	require.NoError(t, tk.With(api.EncodeOptions{AddSpecialTokens: true, MaxLen: 8192}))
	return &backends.Tokenizer{
		GoTokenizer:      &backends.GoTokenizer{Tokenizer: tk},
		MaxAllowedTokens: 8192,
	}
}
