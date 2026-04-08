package embedding_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testModelName = "sentence-transformers/all-MiniLM-L6-v2"
const testModelDims = 384

func TestHugotEmbedder_Embed(t *testing.T) {
	if os.Getenv("VAULTMIND_TEST_EMBEDDING") == "" {
		t.Skip("skipping embedding test (set VAULTMIND_TEST_EMBEDDING=1 to run, downloads ~90MB model)")
	}

	cacheDir := t.TempDir()
	embedder, err := embedding.NewHugotEmbedder(embedding.HugotConfig{
		ModelName:    testModelName,
		CacheDir:     cacheDir,
		Dims:         testModelDims,
		OnnxFilePath: "onnx/model.onnx",
	})
	require.NoError(t, err, "should create embedder")
	defer func() { _ = embedder.Close() }()

	assert.Equal(t, testModelDims, embedder.Dims())

	vec, err := embedder.Embed(context.Background(), "Spreading activation is a retrieval mechanism.")
	require.NoError(t, err, "should produce embedding")
	assert.Len(t, vec, testModelDims, "embedding should be %d dimensions", testModelDims)

	// Vector should not be all zeros
	nonZero := false
	for _, v := range vec {
		if v != 0 {
			nonZero = true
			break
		}
	}
	assert.True(t, nonZero, "embedding should not be all zeros")
}

func TestHugotEmbedder_SimilarTextsCloser(t *testing.T) {
	if os.Getenv("VAULTMIND_TEST_EMBEDDING") == "" {
		t.Skip("skipping embedding test (set VAULTMIND_TEST_EMBEDDING=1)")
	}

	cacheDir := t.TempDir()
	embedder, err := embedding.NewHugotEmbedder(embedding.HugotConfig{
		ModelName:    testModelName,
		CacheDir:     cacheDir,
		Dims:         testModelDims,
		OnnxFilePath: "onnx/model.onnx",
	})
	require.NoError(t, err)
	defer func() { _ = embedder.Close() }()

	ctx := context.Background()
	vecA, err := embedder.Embed(ctx, "Memory consolidation during sleep")
	require.NoError(t, err)
	vecB, err := embedder.Embed(ctx, "Sleep helps stabilize new memories")
	require.NoError(t, err)
	vecC, err := embedder.Embed(ctx, "The recipe calls for three cups of flour")
	require.NoError(t, err)

	simAB := cosine(vecA, vecB)
	simAC := cosine(vecA, vecC)

	assert.Greater(t, simAB, simAC,
		"similar texts (memory+sleep) should have higher cosine than unrelated (memory+flour): AB=%.3f AC=%.3f", simAB, simAC)
}

func TestTruncateForEmbedding(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		maxTokens int
		wantLen   int // 0 means check <= maxTokens * approxCharsPerToken
	}{
		{
			name:      "short text unchanged",
			text:      "hello world",
			maxTokens: 512,
		},
		{
			name:      "long text truncated",
			text:      strings.Repeat("word ", 1000), // 5000 chars
			maxTokens: 512,
		},
		{
			name:      "zero maxTokens returns empty",
			text:      "some text",
			maxTokens: 0,
		},
		{
			name:      "empty text unchanged",
			text:      "",
			maxTokens: 512,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := embedding.TruncateForEmbedding(tt.text, tt.maxTokens)
			if tt.maxTokens == 0 {
				assert.Empty(t, result)
				return
			}
			maxChars := tt.maxTokens * 3 // approx 3 chars per token
			assert.LessOrEqual(t, len(result), maxChars,
				"result should be at most maxTokens*4 chars")
			if len(tt.text) <= maxChars {
				assert.Equal(t, tt.text, result, "short text should be unchanged")
			}
		})
	}
}

func TestTruncateForEmbedding_BreaksAtSpace(t *testing.T) {
	// 20 tokens * 3 chars = 60 char limit
	text := strings.Repeat("abcdefgh ", 20) // 180 chars, 9 chars per word+space
	result := embedding.TruncateForEmbedding(text, 20)
	assert.LessOrEqual(t, len(result), 60)
	// Should not end mid-word
	assert.NotContains(t, result[len(result)-5:], "abcde",
		"should break at space, not mid-word")
}

// cosine computes cosine similarity between two vectors.
func cosine(a, b []float32) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 100; i++ {
		z = (z + x/z) / 2
	}
	return z
}
