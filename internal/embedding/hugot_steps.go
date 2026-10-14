package embedding

import (
	"context"

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
// hugot's own preprocess does.
func preprocessFor(p *pipelines.FeatureExtractionPipeline) func(*backends.PipelineBatch, []string) error {
	return func(batch *backends.PipelineBatch, inputs []string) error {
		backends.TokenizeInputs(batch, p.Model.Tokenizer, inputs)
		return backends.CreateInputTensors(batch, p.Model)
	}
}

// forwardOn runs p's model on an already-preprocessed batch.
func forwardOn(ctx context.Context, p *pipelines.FeatureExtractionPipeline, batch *backends.PipelineBatch) error {
	return backends.RunSessionOnBatch(ctx, batch, p.BasePipeline)
}
