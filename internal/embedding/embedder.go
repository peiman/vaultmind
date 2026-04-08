// Package embedding provides text embedding infrastructure for VaultMind.
package embedding

import (
	"context"
	"os"
	"path/filepath"
)

// Default model configuration for the all-MiniLM-L6-v2 embedder.
const (
	DefaultModelName    = "sentence-transformers/all-MiniLM-L6-v2"
	DefaultDims         = 384
	DefaultOnnxFilePath = "onnx/model.onnx"
)

// DefaultCacheDir returns the default model cache directory (~/.vaultmind/models).
func DefaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".vaultmind", "models")
}

// DefaultHugotConfig returns the standard HugotConfig for all-MiniLM-L6-v2.
func DefaultHugotConfig() HugotConfig {
	return HugotConfig{
		ModelName:    DefaultModelName,
		CacheDir:     DefaultCacheDir(),
		Dims:         DefaultDims,
		OnnxFilePath: DefaultOnnxFilePath,
	}
}

// Embedder converts text into dense vector representations.
type Embedder interface {
	// Embed produces a single embedding vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch produces embedding vectors for multiple texts.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dims returns the dimensionality of the embedding vectors.
	Dims() int

	// Close releases resources (model session, etc.).
	Close() error
}
