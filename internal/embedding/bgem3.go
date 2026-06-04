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
		session:   session,
		pipeline:  pipeline,
		sparseW:   sparseW[0], // [1][1024] -> [1024]
		sparseB:   sBias,
		colbertW:  colbertW,
		colbertB:  colbertB,
		dims:      cfg.Dims,
		maxTokens: cfg.MaxTokens,
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
	// Truncate texts to model's token limit
	if e.maxTokens > 0 {
		truncated := make([]string, len(texts))
		for i, t := range texts {
			truncated[i] = TruncateForEmbedding(t, e.maxTokens)
		}
		texts = truncated
	}

	// Use hugot's Preprocess (tokenize) + Forward (ONNX inference)
	// but skip Postprocess (which mean-pools the per-token output).
	batch := backends.NewBatch(len(texts))
	defer func() { _ = batch.Destroy() }()

	if err := e.pipeline.Preprocess(batch, texts); err != nil {
		return nil, fmt.Errorf("preprocessing: %w", err)
	}
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
