package query

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
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

var passageHeading = regexp.MustCompile(`^#{1,6}[ \t]+`)
var passageSetext = regexp.MustCompile(`^[=-]+$`)
var passageBlank = regexp.MustCompile(`\n\s*\n`)

// Sections never cross a heading or a code fence. Preserve all governing
// headings, nearest first: a parent warning can qualify a neutral subsection.
func passageUnits(body string) []passageUnit {
	var units []passageUnit
	var headings []string
	at, start, section, fenced := 0, 0, 0, false
	heading := func() string {
		var path []string
		for i := len(headings) - 1; i >= 0; i-- {
			if headings[i] != "" {
				path = append(path, headings[i])
			}
		}
		return strings.Join(path, "\n")
	}
	flush := func(end int) {
		raw := body[start:end]
		blockStart := 0
		ends := passageBlank.FindAllStringIndex(raw, -1)
		ends = append(ends, []int{len(raw), len(raw)})
		for _, end := range ends {
			block := raw[blockStart:end[0]]
			trimmed := strings.TrimSpace(block)
			origin := start + blockStart + strings.Index(block, trimmed)
			blockStart = end[1]
			if trimmed == "" {
				continue
			}
			boundaries := passageSentenceStarts(trimmed)
			boundaries = append(boundaries, len(trimmed))
			for i := 0; i < len(boundaries)-1; i++ {
				text := trimmed[boundaries[i]:boundaries[i+1]]
				kept := strings.TrimSpace(text)
				if kept != "" {
					units = append(units, passageUnit{origin + boundaries[i] + strings.Index(text, kept), origin + boundaries[i] + strings.Index(text, kept) + len(kept), section, heading()})
				}
			}
		}
	}
	lines := strings.SplitAfter(body, "\n")
	skipUnderline := false
	for i, line := range lines {
		if skipUnderline {
			at += len(line)
			start = at
			skipUnderline = false
			continue
		}
		trimmed := strings.TrimSpace(line)
		fence := strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
		marker := passageHeading.FindString(trimmed)
		level, label := len(strings.TrimSpace(marker)), trimmed
		if !fenced && !fence && marker == "" && trimmed != "" && i+1 < len(lines) {
			underline := strings.TrimSpace(lines[i+1])
			if passageSetext.MatchString(underline) {
				level = 2
				if underline[0] == '=' {
					level = 1
				}
				label += "\n" + underline
				skipUnderline = true
			}
		}
		if fence {
			if !fenced {
				flush(at)
			}
			fenced = !fenced
			section++
			start = at + len(line)
		} else if !fenced && level > 0 {
			flush(at)
			section++
			for len(headings) < level {
				headings = append(headings, "")
			}
			headings = headings[:level]
			headings[level-1] = label
			start = at + len(line)
		}
		at += len(line)
	}
	if !fenced {
		flush(len(body))
	}
	return units
}

func passageWindows(body string, limit int, tokens []passageToken, terms []string) []passageWindow {
	units := passageUnits(body)
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

// Linear sentence/bullet scan avoids a regexp match and allocation per short
// sentence. Boundaries match the established ASCII punctuation/bullet policy.
func passageSentenceStarts(text string) []int {
	starts := []int{0}
	for i := 0; i < len(text); i++ {
		if strings.ContainsRune(".!?", rune(text[i])) {
			end := i + 1
			for end < len(text) && (text[end] == ' ' || text[end] == '\t') {
				end++
			}
			if end > i+1 {
				starts = append(starts, end)
				i = end - 1
			}
		} else if text[i] == '\n' {
			end := i + 1
			for end < len(text) && (text[end] == ' ' || text[end] == '\t') {
				end++
			}
			if end+1 < len(text) && strings.ContainsRune("-*+", rune(text[end])) && text[end+1] == ' ' {
				starts = append(starts, i+1)
				i = end + 1
			} else {
				digitStart := end
				for end < len(text) && text[end] >= '0' && text[end] <= '9' {
					end++
				}
				if end > digitStart && end+1 < len(text) && text[end] == '.' && text[end+1] == ' ' {
					starts = append(starts, i+1)
					i = end + 1
				}
			}
		}
	}
	return starts
}
