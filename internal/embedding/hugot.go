package embedding

import (
	"context"
	"fmt"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"
)

// HugotEmbedder wraps the hugot library to produce embeddings using ONNX models.
type HugotEmbedder struct {
	session   *hugot.Session
	pipeline  *pipelines.FeatureExtractionPipeline
	dims      int
	maxTokens int
}

// approxCharsPerToken is a conservative estimate for English text.
// Transformer subword tokenizers average 3-4 characters per token.
// We use 3 (conservative) to avoid exceeding the model's context window.
const approxCharsPerToken = 3

// TruncateForEmbedding truncates text to fit within the model's token limit.
// Uses a character-based approximation (4 chars/token for English).
// Breaks at word boundaries when possible.
func TruncateForEmbedding(text string, maxTokens int) string {
	if maxTokens <= 0 {
		return ""
	}
	maxChars := maxTokens * approxCharsPerToken
	if len(text) <= maxChars {
		return text
	}
	cut := text[:maxChars]
	// Find last space for clean word break
	for i := len(cut) - 1; i > maxChars-40 && i > 0; i-- {
		if cut[i] == ' ' {
			return cut[:i]
		}
	}
	return cut
}

// HugotConfig configures the HugotEmbedder.
type HugotConfig struct {
	// ModelPath is the local path to the ONNX model directory.
	// If empty, the model will be downloaded from HuggingFace.
	ModelPath string

	// ModelName is the HuggingFace model ID (e.g., "sentence-transformers/all-MiniLM-L6-v2").
	// Used for downloading if ModelPath is not set.
	ModelName string

	// CacheDir is where downloaded models are stored.
	CacheDir string

	// Dims is the embedding dimensionality (e.g., 384 for MiniLM, 1024 for BGE-M3).
	Dims int

	// OnnxFilePath specifies which ONNX file to use when a model has multiple variants.
	// E.g., "onnx/model.onnx" for the default, "onnx/model_O2.onnx" for optimized.
	OnnxFilePath string

	// MaxTokens is the model's context window size. Texts longer than this (in approximate
	// tokens) are truncated before embedding. 0 means no truncation.
	MaxTokens int
}

// NewHugotEmbedder creates an embedder using hugot with the Go backend.
// For ORT backend (faster, supports larger models), build with -tags ORT.
func NewHugotEmbedder(cfg HugotConfig) (*HugotEmbedder, error) {
	session, err := hugot.NewGoSession()
	if err != nil {
		return nil, fmt.Errorf("creating hugot session: %w", err)
	}

	modelPath := cfg.ModelPath
	if modelPath == "" {
		if cfg.ModelName == "" {
			return nil, fmt.Errorf("either ModelPath or ModelName must be set")
		}
		cacheDir := cfg.CacheDir
		if cacheDir == "" {
			cacheDir = "./models"
		}
		opts := hugot.NewDownloadOptions()
		if cfg.OnnxFilePath != "" {
			opts.OnnxFilePath = cfg.OnnxFilePath
		}
		modelPath, err = hugot.DownloadModel(cfg.ModelName, cacheDir, opts)
		if err != nil {
			return nil, fmt.Errorf("downloading model %q: %w", cfg.ModelName, err)
		}
	}

	pipeline, err := hugot.NewPipeline(session, hugot.FeatureExtractionConfig{
		ModelPath: modelPath,
		Name:      "vaultmind-embedder",
	})
	if err != nil {
		_ = session.Destroy()
		return nil, fmt.Errorf("creating embedding pipeline: %w", err)
	}

	return &HugotEmbedder{
		session:   session,
		pipeline:  pipeline,
		dims:      cfg.Dims,
		maxTokens: cfg.MaxTokens,
	}, nil
}

// Embed produces a single embedding vector.
func (e *HugotEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	batch, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(batch) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}
	return batch[0], nil
}

// EmbedBatch produces embedding vectors for multiple texts.
// Texts exceeding the model's token limit are truncated automatically.
func (e *HugotEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	if e.maxTokens > 0 {
		truncated := make([]string, len(texts))
		for i, t := range texts {
			truncated[i] = TruncateForEmbedding(t, e.maxTokens)
		}
		texts = truncated
	}
	result, err := e.pipeline.RunPipeline(texts)
	if err != nil {
		return nil, fmt.Errorf("running embedding pipeline: %w", err)
	}
	embeddings := make([][]float32, len(result.Embeddings))
	copy(embeddings, result.Embeddings)
	return embeddings, nil
}

// Dims returns the dimensionality of the embedding vectors.
func (e *HugotEmbedder) Dims() int {
	return e.dims
}

// Close releases the hugot session.
func (e *HugotEmbedder) Close() error {
	if e.session != nil {
		return e.session.Destroy()
	}
	return nil
}
