package embedding

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSparseFromForward_SpecialsStillContribute pins the pre-change sparse
// computation. A stub mask marks CLS and SEP, and they still contribute.
// SparseHead with that mask would drop them; the forward pass must not.
func TestSparseFromForward_SpecialsStillContribute(t *testing.T) {
	hidden := [][]float32{
		{1, 0}, // CLS
		{0, 2}, // content
		{3, 0}, // SEP
	}
	ids := []uint32{101, 200, 102}
	w := []float32{1, 1}
	mask := []uint32{1, 0, 1}

	got, pos := sparseFromForward(hidden, ids, mask, w, 0)
	want := SparseHead(hidden, ids, nil, w, 0)
	masked := SparseHead(hidden, ids, mask, w, 0)

	require.Equal(t, want, got)
	require.NotEqual(t, masked, got)
	require.Greater(t, pos[0], float32(0))
	require.Greater(t, pos[2], float32(0))
	require.NotContains(t, masked, int32(101))
	require.Contains(t, got, int32(101))
	require.Contains(t, got, int32(102))
}
