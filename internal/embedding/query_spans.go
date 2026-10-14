package embedding

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gomlx/go-huggingface/tokenizers/api"
	"github.com/gomlx/go-huggingface/tokenizers/hftokenizer"
	"github.com/knights-analytics/hugot/backends"
)

// querySpanCharsPerToken is the character estimate for the query-only span
// read. fitTextsWithinTokenLimit pre-cuts at approxCharsPerToken (4) and
// then measures real tokens; this read does not loop, so the estimate is
// wider. Eight characters per token covers English prose and still bounds a
// multi-megabyte query. Text past the window is not tokenized. The forward
// bound sizes that window; a model-cap window can count a low-density head
// as short enough that the bound never applies.
const querySpanCharsPerToken = 8

// QuerySpanWindow returns the prefix a span read may see for forwardTokens.
// forwardTokens <= 0 leaves the text unchanged. The width is
// querySpanCharsPerToken bytes per token, broken on a word when one is near
// the cut. Callers compare a bounded embed against this prefix.
func QuerySpanWindow(text string, forwardTokens int) string {
	if forwardTokens <= 0 || text == "" {
		return text
	}
	return truncateToChars(text, forwardTokens*querySpanCharsPerToken)
}

// querySpanWindow is QuerySpanWindow. TokenSpans passes the forward bound in
// use. queryAnnotations passes the model cap: AnalyzeQuery runs on text
// ShapeQuery has already cut, and the cap is only a safety bound on that prefix.
func querySpanWindow(text string, maxTokens int) string {
	return QuerySpanWindow(text, maxTokens)
}

// NewSpanEmbedder builds an embedder that can only read token spans. It does
// not load the ONNX model. modelMax is the model cap stored on the embedder.
// TokenSpans sizes its window from the forward bound it is given. A test
// passes BGEM3MaxTokens so a mutation back to that cap is the real model limit.
func NewSpanEmbedder(tokenizerJSON []byte, modelMax int) (*BGEM3Embedder, error) {
	if modelMax <= 0 {
		modelMax = BGEM3MaxTokens
	}
	tok, err := querySpanTokenizerFromBytes(tokenizerJSON, modelMax)
	if err != nil {
		return nil, fmt.Errorf("building span tokenizer: %w", err)
	}
	return &BGEM3Embedder{spanTok: tok, maxTokens: modelMax}, nil
}

// newQuerySpanTokenizer loads a second tokenizer from the model directory.
// It is not the pipeline tokenizer. Span options live only here, so an index
// or a short query never sees them (Principle 7 — one place sets them).
func newQuerySpanTokenizer(modelDir string, maxLen int) (*backends.Tokenizer, error) {
	// nosemgrep: go-path-traversal -- modelDir is the cache directory DownloadBGEM3 just verified, joined with a fixed filename
	raw, err := os.ReadFile(filepath.Join(modelDir, "tokenizer.json")) //nolint:gosec // same
	if err != nil {
		return nil, err
	}
	return querySpanTokenizerFromBytes(raw, maxLen)
}

// querySpanTokenizerFromBytes builds the query-only tokenizer. With replaces
// the option set, so IncludeSpans and IncludeSpecialTokensMask are set on
// this instance alone.
func querySpanTokenizerFromBytes(raw []byte, maxLen int) (*backends.Tokenizer, error) {
	tk, err := hftokenizer.NewFromContent(nil, raw)
	if err != nil {
		return nil, err
	}
	if maxLen <= 0 {
		maxLen = BGEM3MaxTokens
	}
	if err := tk.With(api.EncodeOptions{
		AddSpecialTokens:         true,
		MaxLen:                   maxLen,
		IncludeSpans:             true,
		IncludeSpecialTokensMask: true,
	}); err != nil {
		return nil, err
	}
	return &backends.Tokenizer{
		GoTokenizer:      &backends.GoTokenizer{Tokenizer: tk},
		MaxAllowedTokens: maxLen,
	}, nil
}

// tokenizeSpanWindow tokenizes one prefix on the query-only tokenizer.
// Offsets are byte ranges into that prefix.
func tokenizeSpanWindow(tok *backends.Tokenizer, window string) ([][2]uint, []uint32, error) {
	if tok == nil {
		return nil, nil, fmt.Errorf("tokenizer unavailable")
	}
	batch := backends.NewBatch(1)
	defer func() { _ = batch.Destroy() }()
	backends.TokenizeInputs(batch, tok, []string{window})
	if len(batch.Input) == 0 {
		return nil, nil, fmt.Errorf("tokenizer returned no input")
	}
	return batch.Input[0].Offsets, batch.Input[0].SpecialTokensMask, nil
}

// queryAnnotations tokenizes the model-cap window on the query-only tokenizer.
// AnalyzeQuery runs on text ShapeQuery has already cut, so this window is a
// safety bound, not the forward bound. Offsets are byte ranges into that
// window, which is a prefix of text, so they stay valid on text. The special
// mask is for the forward-bound cut only; sparse weights never see it.
func (e *BGEM3Embedder) queryAnnotations(text string) ([][2]uint, []uint32, error) {
	if e == nil || e.spanTok == nil {
		return nil, nil, fmt.Errorf("tokenizer unavailable")
	}
	return tokenizeSpanWindow(e.spanTok, querySpanWindow(text, e.maxTokens))
}
