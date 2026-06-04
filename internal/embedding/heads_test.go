package embedding_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestL2Normalize(t *testing.T) {
	vec := []float32{3, 4}
	normed := embedding.L2Normalize(vec)
	assert.InDelta(t, 0.6, normed[0], 1e-6)
	assert.InDelta(t, 0.8, normed[1], 1e-6)
	var mag float64
	for _, v := range normed {
		mag += float64(v) * float64(v)
	}
	assert.InDelta(t, 1.0, mag, 1e-6)
}

func TestL2Normalize_ZeroVector(t *testing.T) {
	vec := []float32{0, 0, 0}
	normed := embedding.L2Normalize(vec)
	for _, v := range normed {
		assert.Equal(t, float32(0), v)
	}
}

func TestDenseHead(t *testing.T) {
	hiddenStates := [][]float32{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
	}
	dense := embedding.DenseHead(hiddenStates)
	assert.Len(t, dense, 4)
	assert.InDelta(t, 1.0, dense[0], 1e-6, "CLS token first dim")
	assert.InDelta(t, 0.0, dense[1], 1e-6)
	var mag float64
	for _, v := range dense {
		mag += float64(v) * float64(v)
	}
	assert.InDelta(t, 1.0, mag, 1e-6, "dense should be unit vector")
}

func TestDenseHead_Empty(t *testing.T) {
	assert.Nil(t, embedding.DenseHead(nil))
}

func TestSparseHead(t *testing.T) {
	hiddenStates := [][]float32{
		{0.5, 0.3},
		{1.0, 0.0},
		{0.0, 1.0},
	}
	tokenIDs := []uint32{101, 500, 600}
	specialMask := []uint32{1, 0, 0}
	weights := []float32{1.0, 0.5}
	bias := float32(0.0)

	sparse := embedding.SparseHead(hiddenStates, tokenIDs, specialMask, weights, bias)
	assert.InDelta(t, 1.0, sparse[500], 1e-6)
	assert.InDelta(t, 0.5, sparse[600], 1e-6)
	_, hasCLS := sparse[101]
	assert.False(t, hasCLS, "CLS token should not be in sparse output")
}

func TestSparseHead_NegativeWeightsZeroed(t *testing.T) {
	hiddenStates := [][]float32{
		{0, 0},
		{-1.0, 0.0},
	}
	tokenIDs := []uint32{101, 500}
	specialMask := []uint32{1, 0}
	weights := []float32{1.0, 0.0}
	bias := float32(0.0)

	sparse := embedding.SparseHead(hiddenStates, tokenIDs, specialMask, weights, bias)
	assert.Empty(t, sparse, "negative weights should be zeroed by ReLU")
}

func TestSparseHead_DuplicateTokenKeepsMax(t *testing.T) {
	hiddenStates := [][]float32{
		{0, 0},
		{1.0, 0.0},
		{2.0, 0.0},
	}
	tokenIDs := []uint32{101, 500, 500} // duplicate token 500
	specialMask := []uint32{1, 0, 0}
	weights := []float32{1.0, 0.0}
	bias := float32(0.0)

	sparse := embedding.SparseHead(hiddenStates, tokenIDs, specialMask, weights, bias)
	assert.InDelta(t, 2.0, sparse[500], 1e-6, "should keep maximum weight for duplicate token")
}

func TestColBERTHead(t *testing.T) {
	hiddenStates := [][]float32{
		{1, 0},
		{1, 0},
		{0, 1},
	}
	weights := [][]float32{
		{1, 0},
		{0, 1},
	}
	bias := []float32{0, 0}

	colbert := embedding.ColBERTHead(hiddenStates, weights, bias)
	require.Len(t, colbert, 2)
	assert.InDelta(t, 1.0, colbert[0][0], 1e-6)
	assert.InDelta(t, 0.0, colbert[0][1], 1e-6)
	assert.InDelta(t, 0.0, colbert[1][0], 1e-6)
	assert.InDelta(t, 1.0, colbert[1][1], 1e-6)
}

func TestColBERTHead_OnlyCLS(t *testing.T) {
	hiddenStates := [][]float32{{1, 0}} // only CLS, no content tokens
	weights := [][]float32{{1, 0}, {0, 1}}
	bias := []float32{0, 0}
	assert.Nil(t, embedding.ColBERTHead(hiddenStates, weights, bias))
}

func TestSparseDotProduct(t *testing.T) {
	a := map[int32]float32{100: 1.0, 200: 0.5, 300: 0.3}
	b := map[int32]float32{100: 0.5, 200: 1.0, 400: 0.9}
	score := embedding.SparseDotProduct(a, b)
	assert.InDelta(t, 1.0, score, 1e-6)
}

func TestSparseDotProduct_NoOverlap(t *testing.T) {
	a := map[int32]float32{100: 1.0}
	b := map[int32]float32{200: 1.0}
	assert.Equal(t, 0.0, embedding.SparseDotProduct(a, b))
}

func TestMaxSimScore(t *testing.T) {
	queryTokens := [][]float32{{1, 0}, {0, 1}}
	docTokens := [][]float32{{1, 0}, {0.7, 0.7}, {0, 1}}
	score := embedding.MaxSimScore(queryTokens, docTokens)
	assert.InDelta(t, 2.0, score, 1e-6)
}

func TestMaxSimScore_EmptyDoc(t *testing.T) {
	queryTokens := [][]float32{{1, 0}}
	score := embedding.MaxSimScore(queryTokens, nil)
	assert.Equal(t, 0.0, score)
}
