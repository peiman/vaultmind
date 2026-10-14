package embedding

import (
	"strings"
	"unicode"
)

// RepeatedNonAlnumCap is the longest run of consecutive characters that are
// neither letters, digits, nor whitespace kept in text that will be tokenized.
// A longer run carries no ranking signal. The tokenizer's lattice on that run
// grows much faster than the length: one punctuation character at 2 KB already
// takes a third of a second, and an unspaced mix of punctuation stalls the
// same way. Exactly this many characters of the run are kept. Letters,
// digits, and whitespace are never shortened.
const RepeatedNonAlnumCap = 16

// CapSymbolRuns shortens every run of characters that are neither letters,
// digits, nor whitespace, when the run is longer than RepeatedNonAlnumCap,
// to its first RepeatedNonAlnumCap characters. A string with no such run is
// returned unchanged, so a short or ordinary query stays byte-identical.
// The ranking query and document embedding both use this function (Principle 7).
func CapSymbolRuns(s string) string {
	if s == "" || !hasLongSymbolRun(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	run := 0
	for _, r := range s {
		run = nextSymbolRun(run, r)
		if run > RepeatedNonAlnumCap {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func hasLongSymbolRun(s string) bool {
	run := 0
	for _, r := range s {
		run = nextSymbolRun(run, r)
		if run > RepeatedNonAlnumCap {
			return true
		}
	}
	return false
}

// nextSymbolRun is the length of the current symbol run after reading r.
// A letter, digit, or whitespace ends the run. Any other character extends
// it, including when it differs from the character before it.
func nextSymbolRun(run int, r rune) int {
	if symbolRune(r) {
		return run + 1
	}
	return 0
}

func symbolRune(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r)
}
