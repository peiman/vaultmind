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
			maxChars := tt.maxTokens * 2 // approxCharsPerToken — see hugot.go
			assert.LessOrEqual(t, len(result), maxChars,
				"result should be at most maxTokens*approxCharsPerToken chars")
			if len(tt.text) <= maxChars {
				assert.Equal(t, tt.text, result, "short text should be unchanged")
			}
		})
	}
}

func TestTruncateForEmbedding_BreaksAtSpace(t *testing.T) {
	// 20 tokens * 2 chars = 40 char limit
	text := strings.Repeat("abcdefgh ", 20) // 180 chars, 9 chars per word+space
	result := embedding.TruncateForEmbedding(text, 20)
	assert.LessOrEqual(t, len(result), 40)
	// Should not end mid-word
	assert.NotContains(t, result[len(result)-5:], "abcde",
		"should break at space, not mid-word")
}

// TestTruncateForEmbedding_DenseContentFitsModelMax pins the empirical
// floor for chars/token. The original 3-chars/token ratio failed on
// dense / code-heavy / non-English content: 30/126 notes in workhorse-vault
// tokenized to >512 tokens and the BGE-M3 ONNX runtime rejected the batch
// with axis-1 mismatch ([N 547 384] vs [1 512 384]). 2 is the empirically
// safer floor until chunk-and-pool (vaultmind#30) ships.
//
// If a future change bumps approxCharsPerToken back up, reproduce the
// overflow on a dense code-heavy ~2000-char note before reverting.
func TestTruncateForEmbedding_DenseContentFitsModelMax(t *testing.T) {
	// Punctuation and short identifiers tokenize at ~2 chars/token —
	// representative of the failure class. ~7400 chars; with the old
	// 3:1 ratio this would truncate to 1536 chars, which still tokenizes
	// to >512 tokens. With 2:1 it caps at 1024 chars and stays under
	// the model limit for content of this density.
	text := strings.Repeat("if x := 1; x > 0 { fmt.Println(\"k\") }\n", 200)
	result := embedding.TruncateForEmbedding(text, 512)
	assert.LessOrEqual(t, len(result), 512*2,
		"truncation must use chars/token=2 to keep dense content under the 512-token model limit")
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
	for range 100 {
		z = (z + x/z) / 2
	}
	return z
}
