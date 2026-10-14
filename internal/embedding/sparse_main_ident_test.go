package embedding

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// mainSparseHead is e30ca14's SparseHead, copied so a refactor can be
// compared input-for-input. CLS/SEP contribute when the mask does not mark
// them, which is every vector main indexed.
func mainSparseHead(hiddenStates [][]float32, tokenIDs, specialMask []uint32, weights []float32, bias float32) map[int32]float32 {
	sparse := make(map[int32]float32)
	for i, hidden := range hiddenStates {
		if i < len(specialMask) && specialMask[i] == 1 {
			continue
		}
		var val float32
		for j, h := range hidden {
			if j < len(weights) {
				val += h * weights[j]
			}
		}
		val += bias
		if val <= 0 {
			continue
		}
		tid := tokenIDs[i]
		if tid > math.MaxInt32 {
			continue
		}
		vocabID := int32(tid) //nolint:gosec // guarded by MaxInt32 check above
		if existing, ok := sparse[vocabID]; !ok || val > existing {
			sparse[vocabID] = val
		}
	}
	return sparse
}

func TestSparseHead_MatchesMainForTheSameInputs(t *testing.T) {
	hidden := [][]float32{
		{1, 0},    // CLS
		{0, 2},    // content
		{-1, 0.5}, // negative before bias
		{3, 0},    // SEP
		{1, 1},    // duplicate of content id
	}
	ids := []uint32{101, 200, 201, 102, 200}
	w := []float32{1, 0.5}
	cases := [][]uint32{
		nil,
		{},
		{0, 0, 0, 0, 0},
		{1, 0, 0, 1, 0},
		{1},
	}
	for _, mask := range cases {
		require.Equal(t, mainSparseHead(hidden, ids, mask, w, 0.1), SparseHead(hidden, ids, mask, w, 0.1))
		pos := SparsePositionWeights(hidden, mask, w, 0.1)
		require.Equal(t, mainSparseHead(hidden, ids, mask, w, 0.1), poolSparseWeights(pos, ids))
	}
	// Empty mask is what the index tokenizer hands the forward pass.
	// CLS and SEP keep a positive weight.
	pos := SparsePositionWeights(hidden, nil, w, 0)
	require.Greater(t, pos[0], float32(0))
	require.Greater(t, pos[3], float32(0))
	got := SparseHead(hidden, ids, nil, w, 0)
	require.Contains(t, got, int32(101))
	require.Contains(t, got, int32(102))

	// The forward path drops a presented mask, so it still matches main's
	// empty-mask result rather than main's masked result.
	forward, fwdPos := sparseFromForward(hidden, ids, []uint32{1, 0, 0, 1, 0}, w, 0)
	require.Equal(t, SparseHead(hidden, ids, nil, w, 0), forward)
	require.Equal(t, pos, fwdPos)
	require.NotEqual(t, SparseHead(hidden, ids, []uint32{1, 0, 0, 1, 0}, w, 0), forward)
}
