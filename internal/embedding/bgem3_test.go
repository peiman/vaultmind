package embedding_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBGEM3Embedder_EmbedFull(t *testing.T) {
	if os.Getenv("VAULTMIND_TEST_BGEM3") == "" {
		t.Skip("skipping BGE-M3 test (set VAULTMIND_TEST_BGEM3=1, requires ~2.2GB model)")
	}

	cfg := embedding.BGEM3Config()
	cfg.CacheDir = "/tmp/vaultmind-bgem3-test"
	require.NoError(t, os.MkdirAll(cfg.CacheDir, 0o750))

	t.Log("Creating BGE-M3 embedder...")
	start := time.Now()
	embedder, err := embedding.NewBGEM3Embedder(cfg)
	require.NoError(t, err, "should create BGE-M3 embedder")
	defer func() { _ = embedder.Close() }()
	t.Logf("Embedder created in %v", time.Since(start))

	assert.Equal(t, 1024, embedder.Dims())

	// Test EmbedFull
	out, err := embedder.EmbedFull(context.Background(), "Spreading activation is a retrieval mechanism.")
	require.NoError(t, err)

	// Dense: 1024 dims, L2-normalized
	require.Len(t, out.Dense, 1024)
	var mag float64
	for _, v := range out.Dense {
		mag += float64(v) * float64(v)
	}
	assert.InDelta(t, 1.0, mag, 0.01, "dense should be L2-normalized")

	// Sparse: non-empty, all positive (ReLU)
	assert.NotEmpty(t, out.Sparse, "sparse should have entries")
	for _, w := range out.Sparse {
		assert.Greater(t, w, float32(0), "sparse weights should be positive")
	}
	t.Logf("Sparse entries: %d", len(out.Sparse))

	// ColBERT: non-empty, 1024 dims per token
	assert.NotEmpty(t, out.ColBERT, "ColBERT should have per-token vectors")
	assert.Len(t, out.ColBERT[0], 1024, "ColBERT tokens should be 1024 dims")
	t.Logf("ColBERT tokens: %d", len(out.ColBERT))
}

func TestBGEM3Embedder_Embed(t *testing.T) {
	if os.Getenv("VAULTMIND_TEST_BGEM3") == "" {
		t.Skip("skipping BGE-M3 test")
	}

	cfg := embedding.BGEM3Config()
	cfg.CacheDir = "/tmp/vaultmind-bgem3-test"
	require.NoError(t, os.MkdirAll(cfg.CacheDir, 0o750))

	embedder, err := embedding.NewBGEM3Embedder(cfg)
	require.NoError(t, err)
	defer func() { _ = embedder.Close() }()

	// Embed (Embedder interface) should return dense vector
	vec, err := embedder.Embed(context.Background(), "test query")
	require.NoError(t, err)
	assert.Len(t, vec, 1024)
}
