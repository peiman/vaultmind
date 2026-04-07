package embedding

import (
	"context"
	"fmt"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"
)

// HugotEmbedder wraps the hugot library to produce embeddings using ONNX models.
type HugotEmbedder struct {
	session  *hugot.Session
	pipeline *pipelines.FeatureExtractionPipeline
	dims     int
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
		session:  session,
		pipeline: pipeline,
		dims:     cfg.Dims,
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
func (e *HugotEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
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
