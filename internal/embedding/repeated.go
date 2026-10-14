package embedding

import (
	"strings"
	"unicode"
)

// RepeatedNonAlnumCap is the longest run of one non-alphanumeric character
// kept in text that will be tokenized. A longer run carries no ranking
// signal. The tokenizer's lattice on that run grows much faster than the
// length: one punctuation character at 2 KB already takes a third of a
// second, and a longer divider stalls the forward pass. Exactly this many
// characters are kept. Letters and digits are never shortened.
const RepeatedNonAlnumCap = 16

// CollapseRepeatedNonAlnum shortens every run of the same non-alphanumeric
// character longer than RepeatedNonAlnumCap to exactly that length.
// A string with no such run is returned unchanged, so a short or ordinary
// query stays byte-identical. This is the one rule (Principle 7).
func CollapseRepeatedNonAlnum(s string) string {
	if s == "" || !hasLongRepeatedNonAlnum(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	run := 0
	var prev rune
	for _, r := range s {
		if run > 0 && r == prev {
			run++
		} else {
			prev = r
			run = 1
		}
		if run > RepeatedNonAlnumCap && !isAlphanumericRune(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func hasLongRepeatedNonAlnum(s string) bool {
	run := 0
	var prev rune
	for _, r := range s {
		if run > 0 && r == prev {
			run++
		} else {
			prev = r
			run = 1
		}
		if run > RepeatedNonAlnumCap && !isAlphanumericRune(r) {
			return true
		}
	}
	return false
}

func isAlphanumericRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
