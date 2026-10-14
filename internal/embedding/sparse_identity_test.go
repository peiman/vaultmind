package embedding

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSparseFromForward_DropsMaskedSpecials pins the sparse forward pass.
// A mask that marks CLS and SEP removes them from the map and zeroes their
// position weights. This used to assert the opposite (specials still
// contributed): that was the bug, and the identity tests now describe the
// intended behavior.
func TestSparseFromForward_DropsMaskedSpecials(t *testing.T) {
	hidden := [][]float32{
		{1, 0}, // CLS
		{0, 2}, // content
		{3, 0}, // SEP
	}
	ids := []uint32{101, 200, 102}
	w := []float32{1, 1}
	mask := []uint32{1, 0, 1}

	got, pos := sparseFromForward(hidden, ids, mask, w, 0)
	unmasked := SparseHead(hidden, ids, nil, w, 0)
	masked := SparseHead(hidden, ids, mask, w, 0)

	require.Equal(t, masked, got)
	require.NotEqual(t, unmasked, got)
	require.Equal(t, float32(0), pos[0])
	require.Equal(t, float32(0), pos[2])
	require.Greater(t, pos[1], float32(0))
	require.NotContains(t, got, int32(101))
	require.NotContains(t, got, int32(102))
	require.Contains(t, got, int32(200))
	require.NotContains(t, masked, int32(101))
}
