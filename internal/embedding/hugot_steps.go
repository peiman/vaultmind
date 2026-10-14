package embedding

import (
	"context"

	"github.com/gomlx/go-huggingface/tokenizers/api"
	"github.com/knights-analytics/hugot/backends"
	"github.com/knights-analytics/hugot/pipelines"
)

// hugot 0.7.8 made FeatureExtractionPipeline.Preprocess and .Forward private
// (#148). Both embedders need them: BGE-M3 skips hugot's mean-pooling to read
// raw per-token hidden states for its sparse and ColBERT heads, and both count
// real tokens before the forward pass. The private methods are thin wrappers
// over public backends functions, so these call the same functions directly —
// the steps hugot runs, not a reimplementation of them.

// preprocessFor returns a tokenize-into-batch step for p, matching what
// hugot's own preprocess does, plus the byte spans the query bound reads.
func preprocessFor(p *pipelines.FeatureExtractionPipeline) func(*backends.PipelineBatch, []string) error {
	return func(batch *backends.PipelineBatch, inputs []string) error {
		tokenizeInputs(batch, p.Model.Tokenizer, inputs)
		return backends.CreateInputTensors(batch, p.Model)
	}
}

// tokenizeInputs is the one tokenize step. It asks for byte spans, then
// fills the batch. CreateInputTensors stays with the caller: a span read
// does not need tensors.
func tokenizeInputs(batch *backends.PipelineBatch, tok *backends.Tokenizer, inputs []string) {
	enableContentTokenSpans(tok)
	backends.TokenizeInputs(batch, tok, inputs)
}

// enableContentTokenSpans asks the tokenizer for byte spans and the
// special-token mask. hugot's loader leaves both off, and
// EncodeWithAnnotations then drops the spans it computed. TokenSpans would
// return nothing and the query bound would count whitespace words, so a
// paragraph with no spaces never engages it. With replaces the option set,
// so AddSpecialTokens and the clamped MaxLen are set again here. One place
// decides this (Principle 7).
func enableContentTokenSpans(tok *backends.Tokenizer) {
	if tok == nil || tok.GoTokenizer == nil || tok.GoTokenizer.Tokenizer == nil {
		return
	}
	_ = tok.GoTokenizer.Tokenizer.With(api.EncodeOptions{
		AddSpecialTokens:         true,
		MaxLen:                   tok.MaxAllowedTokens,
		IncludeSpans:             true,
		IncludeSpecialTokensMask: true,
	})
}

// forwardOn runs p's model on an already-preprocessed batch.
func forwardOn(ctx context.Context, p *pipelines.FeatureExtractionPipeline, batch *backends.PipelineBatch) error {
	return backends.RunSessionOnBatch(ctx, batch, p.BasePipeline)
}
