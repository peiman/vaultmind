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
// multi-megabyte query. Text past the window is not tokenized, and the token
// count the forward bound sees is the count inside the window.
const querySpanCharsPerToken = 8

// querySpanWindow returns the prefix the query-only tokenizer may see.
// maxTokens <= 0 leaves the text unchanged: a caller that has not set a
// model budget (the CJK unit fixture) still reads the whole string.
func querySpanWindow(text string, maxTokens int) string {
	if maxTokens <= 0 || text == "" {
		return text
	}
	return truncateToChars(text, maxTokens*querySpanCharsPerToken)
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

// queryAnnotations tokenizes the span window on the query-only tokenizer.
// Offsets are byte ranges into that window, which is a prefix of text, so
// they stay valid on text. The special mask is for the forward-bound cut
// only; sparse weights never see it.
func (e *BGEM3Embedder) queryAnnotations(text string) ([][2]uint, []uint32, error) {
	if e == nil || e.spanTok == nil {
		return nil, nil, fmt.Errorf("tokenizer unavailable")
	}
	window := querySpanWindow(text, e.maxTokens)
	batch := backends.NewBatch(1)
	defer func() { _ = batch.Destroy() }()
	backends.TokenizeInputs(batch, e.spanTok, []string{window})
	if len(batch.Input) == 0 {
		return nil, nil, fmt.Errorf("tokenizer returned no input")
	}
	return batch.Input[0].Offsets, batch.Input[0].SpecialTokensMask, nil
}
