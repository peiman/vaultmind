package query

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/peiman/vaultmind/internal/index"
)

// minTitleMatchToken is the minimum token length considered for fuzzy title
// matching. Filters out stopwords ("do", "it", "my", "of", "in", ...) that
// would otherwise match any title by accident.
//
// The threshold is measured in bytes via len(), which equals rune count for
// ASCII. Vaults authored in non-ASCII scripts (CJK, Cyrillic, accented
// Latin) would have their single-character tokens pass the 3-byte threshold
// incorrectly — acceptable today because vaults are English; revisit with
// utf8.RuneCountInString if/when non-ASCII vaults land.
//
// Accepted false-negative: 2-letter acronyms ("AI", "ML", "NLP") filter out
// alongside stopwords. A silent return still beats a misleading suggestion.
const minTitleMatchToken = 3

// FuzzyTitleMatches returns up to n titles whose words overlap with the
// query's words, ordered by overlap count desc (ties broken by shorter title
// first — more specific titles win). Titles with zero token overlap are
// excluded — a silent return beats a misleading suggestion.
//
// Intended as a zero-hit fallback on ask: when retrieval returns nothing,
// suggest the nearest titles as a user nudge, not as retrieval results.
func FuzzyTitleMatches(query string, titles []index.NoteTitle, n int) []index.NoteTitle {
	qTokens := titleTokens(query)
	if len(qTokens) == 0 {
		return nil
	}

	type scored struct {
		nt    index.NoteTitle
		score int
	}

	ranked := make([]scored, 0, len(titles))
	for _, t := range titles {
		tTokens := titleTokens(t.Title)
		score := 0
		for qt := range qTokens {
			if _, ok := tTokens[qt]; ok {
				score++
			}
		}
		if score > 0 {
			ranked = append(ranked, scored{nt: t, score: score})
		}
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return len(ranked[i].nt.Title) < len(ranked[j].nt.Title)
	})

	if n > 0 && len(ranked) > n {
		ranked = ranked[:n]
	}

	out := make([]index.NoteTitle, len(ranked))
	for i, r := range ranked {
		out[i] = r.nt
	}
	return out
}

// titleTokens lowercases s, splits on non-letter/digit runs, and returns the
// set of tokens with length >= minTitleMatchToken.
func titleTokens(s string) map[string]struct{} {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		if len(f) >= minTitleMatchToken {
			out[f] = struct{}{}
		}
	}
	return out
}

// WriteTitleSuggestions renders a human-readable "did you mean?" block for
// the given matches. Silent (returns false, writes nothing) on empty input
// so callers can compose with other zero-hit diagnostics without adding
// blank sections.
func WriteTitleSuggestions(w io.Writer, matches []index.NoteTitle) bool {
	if len(matches) == 0 {
		return false
	}
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Did you mean one of these?")
	for _, m := range matches {
		_, _ = fmt.Fprintf(w, "  %s  %s\n", m.ID, m.Title)
		_, _ = fmt.Fprintf(w, "      vaultmind note get %s --vault <vault>\n", m.ID)
	}
	return true
}
