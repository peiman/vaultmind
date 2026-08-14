package embedding_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/assert"
)

// Cosine has no test in its own package until now — it was covered only
// transitively through query.CosineSimilarity's one-line delegation, so
// removing that delegation would have silently dropped all coverage of the
// function two layers of this tool depend on.
//
// The mismatched-length case matters most: it is the one that returns 0 for
// "cannot compare", and 0 rendered beside real scores reads as "not similar".
func TestCosineSimilarity(t *testing.T) {
	cases := []struct {
		name string
		a, b []float32
		want float64
	}{
		{"identical", []float32{1, 0, 0}, []float32{1, 0, 0}, 1},
		{"opposite", []float32{1, 0}, []float32{-1, 0}, -1},
		{"orthogonal", []float32{1, 0}, []float32{0, 1}, 0},
		{"mismatched length is not comparable", []float32{1, 0, 0}, []float32{1, 0}, 0},
		{"zero magnitude", []float32{0, 0}, []float32{1, 0}, 0},
		{"both empty", []float32{}, []float32{}, 0},
		{"magnitude does not matter", []float32{2, 0}, []float32{5, 0}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.InDelta(t, c.want, embedding.CosineSimilarity(c.a, c.b), 0.0001)
		})
	}
}
