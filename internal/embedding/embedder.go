// Package embedding provides text embedding infrastructure for VaultMind.
package embedding

import (
	"context"
	"os"
	"path/filepath"

	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/rs/zerolog/log"
)

// Default model configuration for the all-MiniLM-L6-v2 embedder.
const (
	DefaultModelName    = "sentence-transformers/all-MiniLM-L6-v2"
	DefaultDims         = 384
	DefaultMaxTokens    = 510 // MiniLM max is 512 minus 2 for CLS/SEP tokens
	DefaultOnnxFilePath = "onnx/model.onnx"
)

// legacyCacheDir is where model weights used to live: ~/.vaultmind/models.
//
// That put the tool's own cache behind the vault marker, so every home directory
// that had ever downloaded weights answered "yes" to "is this a vault?" — and
// the guessed-vault guard, whose job is refusing directories the user never
// chose, waved $HOME through and indexed it. Making the type registry the marker
// fixes the bug; moving the cache out is what stops the ambiguity existing.
func legacyCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".vaultmind", "models")
}

// DefaultCacheDir returns the model cache directory — the XDG cache dir.
//
// Weights are ~2.2 GB, so relocating must never mean re-downloading. On first
// use an existing legacy cache is renamed into place: instant when both live
// under $HOME, as they normally do. If the rename fails (separate filesystem,
// permissions), the legacy directory keeps being used rather than silently
// starting an empty cache beside a full one.
func DefaultCacheDir() string {
	cacheRoot, pathErr := xdg.CacheDirPath()
	legacy := legacyCacheDir()
	if pathErr != nil {
		// The app name is unset, so there is no XDG cache to migrate into.
		// Return the legacy directory and do not create or rename anything.
		return legacy
	}
	target := filepath.Join(cacheRoot, "models")
	// Before CacheDir's MkdirAll and before the rename. Under go test this
	// panics when a path is still under the home the process started with.
	xdg.GuardTestIsolation(legacy)
	xdg.GuardTestIsolation(cacheRoot)
	xdg.GuardTestIsolation(target)

	xdgCache, err := xdg.CacheDir()
	if err != nil {
		return legacy
	}
	target = filepath.Join(xdgCache, "models")

	migrateLegacyCache(legacy, target)

	if _, statErr := os.Stat(target); statErr == nil {
		return target
	}
	if _, statErr := os.Stat(legacy); statErr == nil {
		return legacy // migration did not happen; keep using what is there
	}
	return target
}

// migrateLegacyCache renames the legacy cache into target. Deliberately not
// guarded by a sync.Once: a once-per-process guard made the outcome depend on
// which caller ran first, which is untestable and, across processes, does not
// bound anything anyway. The stat checks below make it a cheap no-op after the
// first success, and os.Rename is atomic — two processes racing means one wins
// and the loser simply finds the target already in place.
func migrateLegacyCache(legacy, target string) {
	if _, err := os.Stat(target); err == nil {
		return // already migrated
	}
	if _, err := os.Stat(legacy); err != nil {
		return // nothing to move
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		log.Debug().Err(err).Msg("model cache migration skipped; using the legacy directory")
		return
	}
	if err := os.Rename(legacy, target); err != nil {
		log.Debug().Err(err).Msg("model cache migration skipped; using the legacy directory")
		return
	}
	log.Info().Str("from", legacy).Str("to", target).
		Msg("moved the model cache out of the vault-marker directory")
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
	// BGEM3EmbedMaxTokens is the per-note token cap the embedder truncates to.
	// The model accepts BGEM3MaxTokens, but forward memory grows faster than
	// the token count. Peak memory of a full embed (ORT CPU, model included):
	// 12–16 GB at 8190, about 6 GB at 2048, about 3 GB at 1024. On labelled
	// sets, 1024 kept Hit@3 within one query of 8190. Search text (FTS) still
	// holds the whole note; only the vectors read the first 1024 tokens.
	BGEM3EmbedMaxTokens = 1024
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
		MaxTokens:    BGEM3EmbedMaxTokens,
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
