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
	DefaultMaxTokens    = 510 // MiniLM max is 512 minus 2 for CLS/SEP tokens
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
		MaxTokens:    DefaultMaxTokens,
		OnnxFilePath: DefaultOnnxFilePath,
	}
}

// BGE-M3 model configuration.
const (
	BGEM3ModelName    = "BAAI/bge-m3"
	BGEM3Dims         = 1024
	BGEM3MaxTokens    = 8190 // 8192 minus 2 for CLS/SEP
	BGEM3OnnxFilePath = "onnx/model.onnx"
)

// CLI model tokens — the values the --model flag and RunEmbed receive. Distinct
// from the HuggingFace IDs above (DefaultModelName / BGEM3ModelName); named here
// so the token→dimension mapping has a single source of truth.
const (
	ModelMiniLM = "minilm"
	ModelBGEM3  = "bge-m3"
)

// ExpectedDenseDims returns the dense-embedding dimensionality a CLI model token
// produces, and whether the token is recognized. It is a pure lookup — it never
// loads a model — so callers can compare a vault's stored dimension against a
// requested model without paying the (multi-GB, multi-second) embedder load.
func ExpectedDenseDims(model string) (dims int, known bool) {
	switch model {
	case ModelMiniLM:
		return DefaultDims, true
	case ModelBGEM3:
		return BGEM3Dims, true
	default:
		return 0, false
	}
}

// ModelForDenseDims is the reverse of ExpectedDenseDims: the CLI model token that
// produces embeddings of the given dense dimension, and whether it is recognized.
// Used to name the model behind a vault's stored embeddings in diagnostics.
func ModelForDenseDims(dims int) (model string, known bool) {
	switch dims {
	case DefaultDims:
		return ModelMiniLM, true
	case BGEM3Dims:
		return ModelBGEM3, true
	default:
		return "", false
	}
}

// BGEM3Config returns the HugotConfig for BGE-M3.
func BGEM3Config() HugotConfig {
	return HugotConfig{
		ModelName:    BGEM3ModelName,
		CacheDir:     DefaultCacheDir(),
		Dims:         BGEM3Dims,
		MaxTokens:    BGEM3MaxTokens,
		OnnxFilePath: BGEM3OnnxFilePath,
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

// FullEmbedder extends Embedder with multi-output capability (BGE-M3).
type FullEmbedder interface {
	Embedder
	EmbedFullBatch(ctx context.Context, texts []string) ([]*BGEM3Output, error)
}
