package embedding

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/backends"
	"github.com/knights-analytics/hugot/pipelines"
)

// BGEM3Output contains all three embedding types from a BGE-M3 forward pass.
type BGEM3Output struct {
	Dense   []float32         // [1024] CLS-pooled, L2-normalized
	Sparse  map[int32]float32 // vocab_id -> weight (non-zero only)
	ColBERT [][]float32       // [seq_len-1][1024] per-token, L2-normalized
}

// BGEM3Embedder produces dense, sparse, and ColBERT embeddings using BGE-M3.
type BGEM3Embedder struct {
	session   *hugot.Session
	pipeline  *pipelines.FeatureExtractionPipeline
	sparseW   []float32   // [1024] from sparse_linear.pt weight[0]
	sparseB   float32     // scalar bias
	colbertW  [][]float32 // [1024][1024] from colbert_linear.pt
	colbertB  []float32   // [1024] bias
	dims      int
	maxTokens int
	// preprocess tokenizes texts into a batch. In production it is
	// pipeline.Preprocess; tests substitute a fake so the token-fitting path
	// (#39) is exercisable without loading the 2.2GB model.
	preprocess func(*backends.PipelineBatch, []string) error
}

// NewBGEM3Embedder creates a BGE-M3 embedder with all three heads.
func NewBGEM3Embedder(cfg HugotConfig) (*BGEM3Embedder, error) {
	// Download all model files (ONNX + weights + tokenizer)
	modelDir, err := DownloadBGEM3(cfg.CacheDir)
	if err != nil {
		return nil, fmt.Errorf("downloading BGE-M3: %w", err)
	}

	// Create hugot session (ORT if built with -tags ORT, pure Go otherwise)
	session, err := newBGEM3Session()
	if err != nil {
		return nil, fmt.Errorf("creating hugot session: %w", err)
	}

	pipeline, err := hugot.NewPipeline(session, hugot.FeatureExtractionConfig{
		ModelPath: modelDir,
		Name:      "vaultmind-bgem3",
	})
	if err != nil {
		_ = session.Destroy()
		return nil, fmt.Errorf("creating BGE-M3 pipeline: %w", err)
	}

	// Load sparse head weights: Linear(1024, 1)
	sparseW, sparseB, err := LoadLinearWeights(filepath.Join(modelDir, "sparse_linear.pt"))
	if err != nil {
		_ = session.Destroy()
		return nil, fmt.Errorf("loading sparse weights: %w", err)
	}

	// Load ColBERT head weights: Linear(1024, 1024)
	colbertW, colbertB, err := LoadLinearWeights(filepath.Join(modelDir, "colbert_linear.pt"))
	if err != nil {
		_ = session.Destroy()
		return nil, fmt.Errorf("loading ColBERT weights: %w", err)
	}

	var sBias float32
	if len(sparseB) > 0 {
		sBias = sparseB[0]
	}

	return &BGEM3Embedder{
		session:    session,
		pipeline:   pipeline,
		sparseW:    sparseW[0], // [1][1024] -> [1024]
		sparseB:    sBias,
		colbertW:   colbertW,
		colbertB:   colbertB,
		dims:       cfg.Dims,
		maxTokens:  cfg.MaxTokens,
		preprocess: pipeline.Preprocess,
	}, nil
}

// Embed returns the dense embedding (Embedder interface compatibility).
func (e *BGEM3Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	out, err := e.EmbedFull(ctx, text)
	if err != nil {
		return nil, err
	}
	return out.Dense, nil
}

// EmbedBatch returns dense embeddings (Embedder interface compatibility).
func (e *BGEM3Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	outputs, err := e.EmbedFullBatch(ctx, texts)
	if err != nil {
		return nil, err
	}
	result := make([][]float32, len(outputs))
	for i, out := range outputs {
		result[i] = out.Dense
	}
	return result, nil
}

// Dims returns the embedding dimensionality (1024).
func (e *BGEM3Embedder) Dims() int { return e.dims }

// Close releases the hugot session.
func (e *BGEM3Embedder) Close() error {
	if e.session != nil {
		return e.session.Destroy()
	}
	return nil
}

// EmbedFull produces all three embedding types for a single text.
func (e *BGEM3Embedder) EmbedFull(ctx context.Context, text string) (*BGEM3Output, error) {
	outputs, err := e.EmbedFullBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("empty BGE-M3 output")
	}
	return outputs[0], nil
}

// EmbedFullBatch produces all three embedding types for multiple texts.
// Bypasses hugot's Postprocess to access raw per-token hidden states.
func (e *BGEM3Embedder) EmbedFullBatch(_ context.Context, texts []string) ([]*BGEM3Output, error) {
	// Tokenize into a batch, GUARANTEEING no input exceeds the model's token limit
	// before the (hang-prone) ONNX forward pass — see preprocessWithinTokenLimit
	// (#39). Skip Postprocess (mean-pool) so the heads see raw per-token states.
	batch, err := e.preprocessWithinTokenLimit(texts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = batch.Destroy() }()

	if err := e.pipeline.Forward(batch); err != nil {
		return nil, fmt.Errorf("forward pass: %w", err)
	}

	// batch.OutputValues[0] is the raw last_hidden_state: [batch][seq_len][1024]
	rawOutput := batch.OutputValues[0]
	hiddenStates, ok := rawOutput.([][][]float32)
	if !ok {
		return nil, fmt.Errorf("expected [][][]float32 from ONNX output, got %T", rawOutput)
	}

	// Apply three heads to each input
	outputs := make([]*BGEM3Output, len(texts))
	for i, hidden := range hiddenStates {
		tokenIDs := batch.Input[i].TokenIDs
		specialMask := batch.Input[i].SpecialTokensMask

		outputs[i] = &BGEM3Output{
			Dense:   DenseHead(hidden),
			Sparse:  SparseHead(hidden, tokenIDs, specialMask, e.sparseW, e.sparseB),
			ColBERT: ColBERTHead(hidden, e.colbertW, e.colbertB),
		}
	}

	return outputs, nil
}

// preprocessWithinTokenLimit tokenizes texts into a batch, guaranteeing no input
// exceeds maxTokens before the (hang-prone) ONNX forward pass. hugot's Rust
// tokenizer does NOT truncate to the model limit (only the pure-Go one sets
// MaxLen), so a note above max_position_embeddings would otherwise reach ORT as
// an oversized tensor and hang — vaultmind#39. A character estimate
// (TruncateForEmbedding) cannot guarantee the cap for dense content (code /
// markdown / non-English tokenize below the assumed chars/token), so we measure
// the ACTUAL token count from a cheap tokenization pass and shrink any over-limit
// text by its observed chars/token ratio, looping until every input fits. A
// halving fallback guarantees termination. The common case (already within limit)
// is a single Preprocess — no added cost.
func (e *BGEM3Embedder) preprocessWithinTokenLimit(texts []string) (*backends.PipelineBatch, error) {
	fitted, err := fitTextsWithinTokenLimit(texts, e.maxTokens, e.tokenCounts)
	if err != nil {
		return nil, err
	}
	batch := backends.NewBatch(len(fitted))
	if err := e.preprocess(batch, fitted); err != nil {
		_ = batch.Destroy()
		return nil, fmt.Errorf("preprocessing: %w", err)
	}
	return batch, nil
}

// tokenCounts tokenizes texts and returns each one's token count (via a throwaway
// batch). This is the model-bound step fitTextsWithinTokenLimit calls; injecting
// e.preprocess lets tests substitute a fake so the fit path needs no model.
func (e *BGEM3Embedder) tokenCounts(texts []string) ([]int, error) {
	batch := backends.NewBatch(len(texts))
	defer func() { _ = batch.Destroy() }()
	if err := e.preprocess(batch, texts); err != nil {
		return nil, fmt.Errorf("preprocessing: %w", err)
	}
	counts := make([]int, len(texts))
	for i := range texts {
		counts[i] = len(batch.Input[i].TokenIDs)
	}
	return counts, nil
}

// fitTextsWithinTokenLimit shrinks each text until its tokenized length (per
// countTokens) is <= maxTokens, looping with shrinkTowardTokenBudget's halving
// fallback so it always terminates. countTokens is injected, so the loop is
// model-free unit-testable. Returns the within-limit texts, or an error if it
// cannot converge within hardCap (a pathological maxTokens below the tokenizer's
// special-token floor).
func fitTextsWithinTokenLimit(texts []string, maxTokens int, countTokens func([]string) ([]int, error)) ([]string, error) {
	work := make([]string, len(texts))
	for i, t := range texts {
		work[i] = t
		if maxTokens > 0 {
			work[i] = TruncateForEmbedding(t, maxTokens) // cheap first cut by char estimate
		}
	}
	if maxTokens <= 0 {
		return work, nil
	}
	const hardCap = 16 // halving 16x reduces any input to ~0 — convergence guaranteed well before
	for iter := 0; ; iter++ {
		counts, err := countTokens(work)
		if err != nil {
			return nil, err
		}
		over := false
		for i := range work {
			if counts[i] <= maxTokens {
				continue
			}
			over = true
			work[i] = shrinkTowardTokenBudget(work[i], counts[i], maxTokens, iter)
		}
		if !over {
			return work, nil
		}
		if iter >= hardCap {
			return nil, fmt.Errorf("could not fit input within %d tokens after %d attempts", maxTokens, hardCap)
		}
	}
}

// shrinkTowardTokenBudget shortens text toward maxTokens. Early iterations shrink
// proportionally using the MEASURED chars/token ratio (with a 10% margin) so it
// converges in one or two passes for real content; as a guaranteed-converging
// fallback it halves, so the loop always terminates even on pathological input.
func shrinkTowardTokenBudget(text string, tokenCount, maxTokens, iter int) string {
	if iter < 4 && tokenCount > 0 {
		ratio := float64(len(text)) / float64(tokenCount) // measured chars per token
		target := int(float64(maxTokens) * ratio * 0.9)   // 10% margin under the limit
		if target > 0 && target < len(text) {
			return truncateToChars(text, target)
		}
	}
	return truncateToChars(text, len(text)/2) // monotone shrink → guaranteed convergence
}

// EmbedSparse produces only the sparse embedding (used by SparseRetriever).
func (e *BGEM3Embedder) EmbedSparse(ctx context.Context, text string) (map[int32]float32, error) {
	out, err := e.EmbedFull(ctx, text)
	if err != nil {
		return nil, err
	}
	return out.Sparse, nil
}

// EmbedColBERT produces only the ColBERT per-token embeddings (used by ColBERTRetriever).
func (e *BGEM3Embedder) EmbedColBERT(ctx context.Context, text string) ([][]float32, error) {
	out, err := e.EmbedFull(ctx, text)
	if err != nil {
		return nil, err
	}
	return out.ColBERT, nil
}
