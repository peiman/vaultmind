package query_test

import (
	"math"
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
)

func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float32{1, 2, 3}
	sim := query.CosineSimilarity(a, a)
	assert.InDelta(t, 1.0, sim, 1e-6)
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	sim := query.CosineSimilarity(a, b)
	assert.InDelta(t, 0.0, sim, 1e-6)
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{-1, -2, -3}
	sim := query.CosineSimilarity(a, b)
	assert.InDelta(t, -1.0, sim, 1e-6)
}

func TestCosineSimilarity_KnownValue(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{1, 1}
	// cos(45°) = 1/√2 ≈ 0.7071
	sim := query.CosineSimilarity(a, b)
	assert.InDelta(t, 1.0/math.Sqrt(2), sim, 1e-6)
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	sim := query.CosineSimilarity(a, b)
	assert.Equal(t, 0.0, sim)
}
