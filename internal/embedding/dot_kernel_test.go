package embedding

import (
	"math"
	"math/rand"
	"testing"
)

// referenceDot is the obvious loop the kernel must agree with.
func referenceDot(a, b []float32) float64 {
	var dot float64
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}

func randomVec(r *rand.Rand, n int) []float32 {
	v := make([]float32, n)
	for i := range v {
		v[i] = r.Float32()*2 - 1
	}
	return v
}

// The unrolled kernel sums in a different order, so it may differ from the
// plain loop in the last bits — never by enough to reorder a ranking.
func TestDotProductF32_AgreesWithThePlainLoop(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, n := range []int{0, 1, 3, 4, 5, 7, 8, 1023, 1024, 1025} {
		a, b := randomVec(r, n), randomVec(r, n)
		if got, want := dotProductF32(a, b), referenceDot(a, b); math.Abs(got-want) > 1e-9 {
			t.Fatalf("n=%d: kernel %v, plain loop %v", n, got, want)
		}
	}
}

// Vectors of different length use the shorter, as the loop always did.
func TestDotProductF32_UsesTheShorterVector(t *testing.T) {
	a := []float32{1, 2, 3, 4, 5}
	b := []float32{1, 1}
	if got := dotProductF32(a, b); got != 3 {
		t.Fatalf("got %v, want 3", got)
	}
	if got := dotProductF32(b, a); got != 3 {
		t.Fatalf("reversed: got %v, want 3", got)
	}
}

func BenchmarkMaxSimScore(b *testing.B) {
	r := rand.New(rand.NewSource(1))
	query := make([][]float32, 8)
	for i := range query {
		query[i] = randomVec(r, 1024)
	}
	doc := make([][]float32, 500)
	for i := range doc {
		doc[i] = randomVec(r, 1024)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MaxSimScore(query, doc)
	}
}
