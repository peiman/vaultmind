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
	// Offsets are byte ranges into the text the tokenizer saw, aligned with
	// Weights. A token with no span has end <= start.
	Offsets [][2]int
}

// alignSalientSpans maps query-only offsets onto weights that already
// dropped the CLS position. It does not apply a mask itself. The forward
// pass zeroes CLS, SEP, PAD, and UNK in those weights before this runs.
func alignSalientSpans(text string, offsets [][2]uint, weights []float32) ([]string, []int, [][2]int) {
	padded := make([]float32, 1+len(weights))
	copy(padded[1:], weights)
	_, word, words, aligned := alignQueryTokens(text, offsets, padded, len(weights))
	return words, word, aligned
}

// alignQueryTokens drops the CLS position so index 0 matches ColBERT[0],
// then trims to the ColBERT length. Offsets are byte offsets into text,
// which is the string the tokenizer actually saw.
func alignQueryTokens(text string, offsets [][2]uint, positionWeights []float32, colbertLen int) ([]float32, []int, []string, [][2]int) {
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
	aligned := make([][2]int, len(weights))
	for i := range aligned {
		aligned[i] = [2]int{-1, -1}
	}
	for i := 0; i < len(weights) && i < len(offs); i++ {
		aligned[i] = clampSpan(text, int(offs[i][0]), int(offs[i][1]))
	}
	return weights, tokenWord, words, aligned
}

// clampSpans pulls tokenizer spans back onto text. The last piece of a
// no-space run can report an end a few bytes past the string; the embedded
// prefix is still that string.
func clampSpans(text string, spans [][2]int) [][2]int {
	if len(spans) == 0 {
		return spans
	}
	out := make([][2]int, len(spans))
	for i, sp := range spans {
		out[i] = clampSpan(text, sp[0], sp[1])
	}
	return out
}

func clampSpan(text string, start, end int) [2]int {
	if end > len(text) {
		end = len(text)
	}
	return [2]int{start, end}
}

// WhitespaceTokenSpans is the one whitespace split. A keyword-only ask has
// no BGE-M3 tokenizer, so each whitespace word is one token. alignTokenWords
// uses the same split, and the two cannot drift (Principle 7).
func WhitespaceTokenSpans(text string) [][2]int {
	_, spans := whitespaceSpans(text)
	return spans
}

// contentTokenSpans keeps tokens the model will score: not special, and with
// a real span. Hugot turns a special's negative span into [0,0], so an empty
// span is not content even when the mask is missing.
func contentTokenSpans(offsets [][2]uint, special []uint32) [][2]int {
	var out [][2]int
	for i, off := range offsets {
		if i < len(special) && special[i] != 0 {
			continue
		}
		if off[1] <= off[0] {
			continue
		}
		out = append(out, [2]int{int(off[0]), int(off[1])})
	}
	return out
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
