package embedding_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/require"
)

func TestBGEM3_PureGoBackend(t *testing.T) {
	if os.Getenv("VAULTMIND_TEST_BGEM3") == "" {
		t.Skip("skipping BGE-M3 test (set VAULTMIND_TEST_BGEM3=1 to run, downloads ~2.2GB model)")
	}

	cacheDir := "/tmp/vaultmind-bgem3-test"
	os.MkdirAll(cacheDir, 0o755)
	t.Logf("Cache dir: %s", cacheDir)

	t.Log("Creating BGE-M3 embedder with pure Go backend...")
	start := time.Now()
	embedder, err := embedding.NewHugotEmbedder(embedding.HugotConfig{
		ModelName:    "BAAI/bge-m3",
		CacheDir:     cacheDir,
		Dims:         1024,
		MaxTokens:    8190,
		OnnxFilePath: "onnx/model.onnx",
	})
	if err != nil {
		t.Fatalf("Failed to create embedder: %v", err)
	}
	defer func() { _ = embedder.Close() }()
	t.Logf("Embedder created in %v", time.Since(start))

	// Test 1: Two short texts
	texts := []string{
		"Spreading activation is a retrieval mechanism.",
		"Memory consolidation during sleep helps learning.",
	}
	t.Log("Embedding 2 short texts...")
	start = time.Now()
	vecs, err := embedder.EmbedBatch(context.Background(), texts)
	require.NoError(t, err, "should embed short texts")
	elapsed := time.Since(start)
	t.Logf("Embedded in %v (per-text: %v)", elapsed, elapsed/time.Duration(len(texts)))
	t.Logf("Vector dims: %d", len(vecs[0]))

	require.Equal(t, 1024, len(vecs[0]), "BGE-M3 should produce 1024-dim vectors")

	// Test 2: One longer text
	long := strings.Repeat("This is a longer text to test performance with more tokens. ", 50)
	t.Logf("Embedding 1 longer text (%d chars)...", len(long))
	start = time.Now()
	vec, err := embedder.Embed(context.Background(), long)
	require.NoError(t, err, "should embed longer text")
	t.Logf("Embedded in %v", time.Since(start))
	require.Equal(t, 1024, len(vec))

	fmt.Println("SUCCESS — BGE-M3 works with pure Go backend")
}
