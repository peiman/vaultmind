package embedding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlignQueryTokens_DropsCLSAndMapsByteOffsets(t *testing.T) {
	// "café beta": café is 5 bytes (c a f c3 a9), then a space, then beta.
	text := "café beta"
	offsets := [][2]uint{{0, 0}, {0, 5}, {6, 10}, {10, 10}}
	weights := []float32{0, 0.4, 3, 0}
	colbertLen := 3

	gotW, gotWord, words, offs := alignQueryTokens(text, offsets, weights, colbertLen)

	assert.Equal(t, []float32{0.4, 3, 0}, gotW, "CLS weight is dropped so index 0 matches ColBERT")
	assert.Equal(t, []string{"café", "beta"}, words)
	assert.Equal(t, []int{0, 1, -1}, gotWord, "an empty offset is not a word")
	assert.Equal(t, [][2]int{{0, 5}, {6, 10}, {10, 10}}, offs)
}

func TestAlignQueryTokens_NoOffsetsSelectsNoWords(t *testing.T) {
	weights := []float32{0, 1, 2}
	gotW, gotWord, words, offs := alignQueryTokens("one two", nil, weights, 2)
	assert.Equal(t, []float32{1, 2}, gotW)
	assert.Nil(t, words)
	require.Equal(t, []int{-1, -1}, gotWord)
	assert.Equal(t, [][2]int{{-1, -1}, {-1, -1}}, offs)
}

func TestAlignSalientSpans_KeepsSepSlot(t *testing.T) {
	// CLS, "hi", SEP. Weights already dropped CLS. The empty SEP offset is
	// not a word, and the slot stays so its weight still counts.
	offsets := [][2]uint{{0, 0}, {0, 2}, {0, 0}}
	weights := []float32{0.4, 0.9}
	words, word, aligned := alignSalientSpans("hi", offsets, weights)
	assert.Equal(t, []string{"hi"}, words)
	assert.Equal(t, []int{0, -1}, word)
	assert.Len(t, aligned, len(weights))
}

func TestContentTokenSpans_SkipsSpecialsAndEmptyOffsets(t *testing.T) {
	offsets := [][2]uint{{0, 0}, {0, 3}, {3, 6}, {6, 6}, {6, 9}}
	special := []uint32{1, 0, 0, 0, 1}
	assert.Equal(t, [][2]int{{0, 3}, {3, 6}}, contentTokenSpans(offsets, special))
}

func TestCloneOutput_CopiesQueryTokenFields(t *testing.T) {
	src := &BGEM3Output{
		Dense:        []float32{1},
		Sparse:       map[int32]float32{7: 1},
		ColBERT:      [][]float32{{1, 0}},
		TokenWeights: []float32{4, 5},
		TokenWord:    []int{0, 1},
		Words:        []string{"alpha", "beta"},
		TokenOffsets: [][2]int{{0, 1}, {2, 3}},
	}
	dst := cloneOutput(src)
	dst.TokenWeights[0] = 0
	dst.TokenWord[0] = 9
	dst.Words[0] = "changed"
	dst.TokenOffsets[0] = [2]int{9, 9}
	assert.Equal(t, float32(4), src.TokenWeights[0])
	assert.Equal(t, 0, src.TokenWord[0])
	assert.Equal(t, "alpha", src.Words[0])
	assert.Equal(t, [2]int{0, 1}, src.TokenOffsets[0])
}
