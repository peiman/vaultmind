package embedding

import (
	"unicode"
	"unicode/utf8"
)

// QueryTokens is one query's per-token view of the forward pass, aligned
// with ColBERT (the CLS position is already dropped).
//
// Weights are the per-position ReLU sparse weights BEFORE vocab-id
// max-pooling. Word is -1 when a token has no word: a special token, an
// empty offset, or a pass that did not report offsets. Sidecar output has
// none of these fields, and salient word selection does not run for it.
type QueryTokens struct {
	Weights []float32
	Word    []int
	Words   []string
}

// alignQueryTokens drops the CLS position so index 0 matches ColBERT[0],
// then trims to the ColBERT length. Offsets are byte offsets into text,
// which is the string the tokenizer actually saw.
func alignQueryTokens(text string, offsets [][2]uint, positionWeights []float32, colbertLen int) ([]float32, []int, []string) {
	weights := positionWeights
	if len(weights) > 0 {
		weights = weights[1:]
	}
	offs := offsets
	if len(offs) > 0 {
		offs = offs[1:]
	}
	if colbertLen < len(weights) {
		weights = weights[:colbertLen]
	}
	if colbertLen < len(offs) {
		offs = offs[:colbertLen]
	}
	weights = append([]float32(nil), weights...)
	words, tokenWord := alignTokenWords(text, offs)
	if len(tokenWord) != len(weights) {
		fixed := make([]int, len(weights))
		for i := range fixed {
			fixed[i] = -1
		}
		copy(fixed, tokenWord)
		tokenWord = fixed
	}
	return weights, tokenWord, words
}

// alignTokenWords maps each token onto the whitespace word that contains
// its start byte. An empty offset (start == end) is not a word. Missing
// offsets leave every index at -1 and return no words, so the caller does
// not fall back to the first N words.
func alignTokenWords(text string, offsets [][2]uint) ([]string, []int) {
	tokenWord := make([]int, len(offsets))
	for i := range tokenWord {
		tokenWord[i] = -1
	}
	if text == "" || len(offsets) == 0 {
		return nil, tokenWord
	}
	words, spans := whitespaceSpans(text)
	for ti, off := range offsets {
		if off[0] == off[1] {
			continue
		}
		start := int(off[0])
		for wi, sp := range spans {
			if start >= sp[0] && start < sp[1] {
				tokenWord[ti] = wi
				break
			}
		}
	}
	return words, tokenWord
}

// whitespaceSpans splits text the same way strings.Fields does, and keeps
// the byte range of each word so tokenizer offsets can find it.
func whitespaceSpans(text string) ([]string, [][2]int) {
	var words []string
	var spans [][2]int
	i := 0
	for i < len(text) {
		r, size := utf8.DecodeRuneInString(text[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		start := i
		for i < len(text) {
			r, size = utf8.DecodeRuneInString(text[i:])
			if unicode.IsSpace(r) {
				break
			}
			i += size
		}
		words = append(words, text[start:i])
		spans = append(spans, [2]int{start, i})
	}
	return words, spans
}
