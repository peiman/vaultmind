package embedding

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

func randTokens(r *rand.Rand, n, dims int) [][]float32 {
	out := make([][]float32, n)
	for i := range out {
		out[i] = make([]float32, dims)
		for j := range out[i] {
			out[i][j] = r.Float32()*2 - 1
		}
	}
	return out
}

// Scoring notes on several cores must give exactly the scores one core gives,
// each at its note's position — a ranking built on them is then unchanged.
// Covers more workers than notes, one note, and none.
func TestMaxSimAll_MatchesOneAtATimeInOrder(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	query := randTokens(r, 7, 32)
	for _, n := range []int{0, 1, 3, 50} {
		docs := make([][][]float32, n)
		for i := range docs {
			docs[i] = randTokens(r, 1+r.IntN(40), 32)
		}
		want := make([]float64, n)
		for i, d := range docs {
			want[i] = MaxSimScore(query, d)
		}
		for _, workers := range []int{1, 2, 4, 8, 64} {
			require.Equal(t, want, MaxSimAll(query, docs, workers), "n=%d workers=%d", n, workers)
		}
	}
}
