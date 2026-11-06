package query

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/peiman/vaultmind/internal/passage"
)

type passageToken struct {
	term       string
	start, end int
}
type passageUnit struct {
	start, end, section int
	heading             string
}
type passageWindow struct {
	start, end, lo, hi  int
	unitStart, capBytes int
	heading             string
	cut                 bool
	partial             []string
}

var passageSetext = passage.Setext

// The lexical windows retain their private representation; parsing is shared.
func passageUnits(body, title string) []passageUnit {
	shared := passage.Units(body, title)
	units := make([]passageUnit, len(shared))
	for i, u := range shared {
		units[i] = passageUnit{u.Start, u.End, u.Section, u.Heading}
	}
	return units
}

func passageWindows(body, title string, limit int, tokens []passageToken, terms []string) []passageWindow {
	units := passageUnits(body, title)
	var out []passageWindow
	end := 0
	for i, u := range units {
		capBytes := limit * 4
		if u.heading != "" {
			capBytes -= len(u.heading) + 2
		}
		if capBytes <= passageCutReserve {
			continue
		} // never trim a qualification to make an answer fit
		end = max(end, i)
		for end < len(units) && units[end].section == u.section && units[end].end-u.start <= capBytes {
			end++
		}
		if end > i {
			out = append(out, passageWindow{start: u.start, end: units[end-1].end, heading: u.heading})
			continue
		}
		// Oversized sentences are scored across their entire token stream. Retain
		// whole raw identifiers; only a token larger than the cap needs rune cuts.
		lo := sort.Search(len(tokens), func(j int) bool { return tokens[j].start >= u.start })
		for j := lo; j < len(tokens) && tokens[j].start < u.end; j++ {
			t := tokens[j]
			if t.end-t.start > capBytes-passageCutReserve {
				for _, term := range terms {
					if p := strings.Index(t.term, term); p >= 0 && len(t.term) == t.end-t.start && len(term) <= capBytes-passageCutReserve && passageCJK(term) {
						// Unspaced CJK has unchanged byte lengths under unicode61 folding.
						start := t.start + p
						finish := min(u.end, start+capBytes-passageCutReserve)
						for finish > start && finish < len(body) && !utf8.RuneStart(body[finish]) {
							finish--
						}
						out = append(out, passageWindow{start: start, end: finish, heading: u.heading, unitStart: u.start, capBytes: capBytes, cut: true, partial: []string{term}})
					}
				}
				continue
			}
			hi := sort.Search(len(tokens)-j, func(k int) bool { return tokens[j+k].end > min(u.end, t.start+capBytes-passageCutReserve) }) + j
			finish := tokens[hi-1].end
			if hi == len(tokens) || tokens[hi].start >= u.end {
				finish = min(u.end, t.start+capBytes-passageCutReserve)
			}
			out = append(out, passageWindow{start: t.start, end: finish, heading: u.heading, unitStart: u.start, capBytes: capBytes, cut: true})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].start < out[j].start })
	return out
}

func (w passageWindow) deliver(body string, tokens []passageToken, terms []string) string {
	start, end := w.start, w.end
	if w.cut {
		// Keep some lead-in around a winning token span when it fits without
		// losing any matched token. Short sentences keep their original opening.
		left := max(w.unitStart, start-passageLeadInBytes)
		for left < start && !utf8.RuneStart(body[left]) {
			left++
		}
		if left > w.unitStart {
			if space := strings.IndexAny(body[left:start], " \t\n"); space >= 0 {
				left += space + 1
			}
		}
		right := min(end, left+w.capBytes-passageCutReserve)
		for right > left && right < len(body) && !utf8.RuneStart(body[right]) {
			right--
		}
		matchedEnd := start
		for i := w.lo; i < w.hi; i++ {
			for _, term := range terms {
				if tokens[i].term == term {
					matchedEnd = tokens[i].end
				}
			}
		}
		if len(w.partial) > 0 {
			matchedEnd = end
		}
		if right >= matchedEnd {
			start, end = left, right
		}
	}
	text := strings.TrimSpace(body[start:end])
	if w.cut {
		text += "…"
	}
	if w.heading != "" {
		text = w.heading + "\n\n" + text
	}
	return text
}

// Sub-token matching is limited to unspaced CJK text, not long identifiers.
func passageCJK(term string) bool {
	for _, r := range term {
		if !unicode.Is(unicode.Han, r) && !unicode.Is(unicode.Hiragana, r) && !unicode.Is(unicode.Katakana, r) && !unicode.Is(unicode.Hangul, r) {
			return false
		}
	}
	return term != ""
}
