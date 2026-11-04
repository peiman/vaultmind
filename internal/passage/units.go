// Package passage defines shared spans for embedding and delivery.
package passage

import (
	"regexp"
	"strings"
)

// Unit is a sentence or bullet with byte offsets into the comment-stripped body.
type Unit struct {
	Start, End, Section int
	Heading             string
}

var atxHeading = regexp.MustCompile(`^#{1,6}[ \t]+`)

// Setext recognizes a Markdown heading underline.
var Setext = regexp.MustCompile(`^[=-]+$`)
var blank = regexp.MustCompile(`\n\s*\n`)

// VectorThresholdTokens is the measured excerpt size above which units are indexed.
// Ask's CLI excerpt default is 0 (off); no shared 160-token default exists.
const VectorThresholdTokens = 160

// EstimateTokens estimates tokens with ceiling division at four bytes per token.
func EstimateTokens(text string) int { return (len(text) + 3) / 4 }

// Sections never cross a heading or a code fence. Preserve every governing
// heading, top-down, so a parent warning still qualifies a subsection. An H1
// that repeats the note title is left out: the title is already shown.
func Units(body, title string) []Unit {
	var units []Unit
	var headings []string
	at, start, section, fenced := 0, 0, 0, false
	heading := func() string {
		var path []string
		for level, label := range headings {
			if label == "" || (level == 0 && h1RepeatsTitle(label, title)) {
				continue
			}
			path = append(path, label)
		}
		return strings.Join(path, "\n")
	}
	flush := func(end int) {
		raw := body[start:end]
		blockStart := 0
		ends := blank.FindAllStringIndex(raw, -1)
		ends = append(ends, []int{len(raw), len(raw)})
		for _, end := range ends {
			block := raw[blockStart:end[0]]
			trimmed := strings.TrimSpace(block)
			origin := start + blockStart + strings.Index(block, trimmed)
			blockStart = end[1]
			if trimmed == "" {
				continue
			}
			boundaries := SentenceStarts(trimmed)
			boundaries = append(boundaries, len(trimmed))
			for i := 0; i < len(boundaries)-1; i++ {
				text := trimmed[boundaries[i]:boundaries[i+1]]
				kept := strings.TrimSpace(text)
				if kept != "" {
					units = append(units, Unit{origin + boundaries[i] + strings.Index(text, kept), origin + boundaries[i] + strings.Index(text, kept) + len(kept), section, heading()})
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
		marker := atxHeading.FindString(trimmed)
		level, label := len(strings.TrimSpace(marker)), trimmed
		if !fenced && !fence && marker == "" && trimmed != "" && i+1 < len(lines) {
			underline := strings.TrimSpace(lines[i+1])
			if Setext.MatchString(underline) {
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

// Linear sentence/bullet scan avoids a regexp match and allocation per short
// sentence. Boundaries match the established ASCII punctuation/bullet policy.
func SentenceStarts(text string) []int {
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

// h1RepeatsTitle reports whether a level-1 heading is the note title. The
// title is already shown on the note. A setext underline, a closing hash run,
// case, and runs of spaces do not make it a different title.
func h1RepeatsTitle(label, title string) bool {
	if strings.TrimSpace(title) == "" {
		return false
	}
	return sameTitle(headingPlain(label), title)
}

// headingPlain is one stored heading's visible text: no ATX marker, no
// closing hash run, and no setext underline.
func headingPlain(label string) string {
	line, _, _ := strings.Cut(label, "\n")
	text := strings.TrimSpace(strings.TrimLeft(line, "#"))
	if i := strings.LastIndex(text, " "); i >= 0 {
		tail := strings.TrimSpace(text[i+1:])
		if tail != "" && strings.Trim(tail, "#") == "" {
			text = strings.TrimSpace(text[:i])
		}
	}
	return text
}

func sameTitle(a, b string) bool {
	af, bf := strings.Fields(a), strings.Fields(b)
	if len(af) != len(bf) || len(af) == 0 {
		return false
	}
	for i := range af {
		if !strings.EqualFold(af[i], bf[i]) {
			return false
		}
	}
	return true
}
