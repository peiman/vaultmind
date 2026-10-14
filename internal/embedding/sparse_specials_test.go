package embedding

import (
	"fmt"
	"testing"

	"github.com/gomlx/go-huggingface/tokenizers/api"
	"github.com/stretchr/testify/require"
)

type stubSpecials struct {
	ids map[api.SpecialToken]int
}

func (s stubSpecials) SpecialTokenID(kind api.SpecialToken) (int, error) {
	id, ok := s.ids[kind]
	if !ok {
		return -1, fmt.Errorf("no %v", kind)
	}
	return id, nil
}

// TestSparseMap_ExcludesSpecialIDs is the model-free pin: CLS, SEP, PAD, and
// UNK leave the sparse map, and the mask token stays. The ids come from the
// tokenizer's special-token map, not from a hard-coded list in the head.
func TestSparseMap_ExcludesSpecialIDs(t *testing.T) {
	set, err := sparseSpecialIDs(stubSpecials{ids: map[api.SpecialToken]int{
		api.TokClassification:      0,
		api.TokPad:                 1,
		api.TokEndOfSentence:       2,
		api.TokUnknown:             3,
		api.TokBeginningOfSentence: 0, // same id as CLS; the set does not grow
		api.TokMask:                250001,
	}})
	require.NoError(t, err)
	_, maskToken := set[250001]
	require.False(t, maskToken, "the mask token stays in the sparse vector")

	ids := []uint32{0, 10, 2, 3, 99, 1, 250001}
	mask := sparseMaskFromTokenIDs(ids, set)
	require.Equal(t, []uint32{1, 0, 1, 1, 0, 1, 0}, mask)
	require.Nil(t, sparseMaskFromTokenIDs(nil, set))
	require.Nil(t, sparseMaskFromTokenIDs(ids, nil))

	hidden := make([][]float32, len(ids))
	for i := range hidden {
		hidden[i] = []float32{1}
	}
	got, pos := sparseFromForward(hidden, ids, mask, []float32{1}, 0)
	require.Equal(t, float32(0), pos[0])
	require.Greater(t, pos[1], float32(0))
	require.NotContains(t, got, int32(0))
	require.NotContains(t, got, int32(1))
	require.NotContains(t, got, int32(2))
	require.NotContains(t, got, int32(3))
	require.Contains(t, got, int32(10))
	require.Contains(t, got, int32(99))
	require.Contains(t, got, int32(250001))
}

func TestSparseSpecialIDs_DistinctBOSIsIncluded(t *testing.T) {
	set, err := sparseSpecialIDs(stubSpecials{ids: map[api.SpecialToken]int{
		api.TokClassification:      0,
		api.TokPad:                 1,
		api.TokEndOfSentence:       2,
		api.TokUnknown:             3,
		api.TokBeginningOfSentence: 4,
	}})
	require.NoError(t, err)
	_, ok := set[4]
	require.True(t, ok)
}

func TestSparseSpecialIDs_MissingRequiredIsAnError(t *testing.T) {
	_, err := sparseSpecialIDs(stubSpecials{ids: map[api.SpecialToken]int{
		api.TokClassification: 0,
	}})
	require.Error(t, err)
	_, err = sparseSpecialIDs(nil)
	require.Error(t, err)
}
