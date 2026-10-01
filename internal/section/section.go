// Package section splits a long note into the parts retrieval works on.
//
// BGE-M3 embeds at most 8,192 tokens, so everything past that in a long note
// (an imported doc page runs 25–40k) was invisible to semantic search, and a
// single vector over many topics matches each of them weakly. A note that fits
// the window stays one unit; a longer one is cut at its own headings, so each
// part is one topic with the title and heading path to say where it sits.
package section

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/peiman/vaultmind/internal/embedding"
)

// splitThreshold is the most a note may hold and stay one unit: the model's
// window. Anything longer was being truncated.
const splitThreshold = embedding.BGEM3MaxTokens

// minSectionTokens is the smallest part worth its own retrieval unit. A
// two-line "See also" under its own heading would otherwise rank on its own
// and carry nothing.
const minSectionTokens = 200

// maxSectionTokens caps a part. One topic rarely runs longer, and a smaller
// part is what makes the delivered text the answer rather than its page.
const maxSectionTokens = 2048

// maxSplitLevel is the deepest heading a note is cut at. Level 4 and below
// stay inside their section.
const maxSplitLevel = 3

// PathSeparator joins a heading path, in Prefix and where a path is stored.
const PathSeparator = " › "

// Section is one part of a split note.
type Section struct {
	// Ordinal is the part's place in the note, from 0.
	Ordinal int
	// Anchor is unique within the note, e.g. "6-9-the-optimize-command".
	Anchor string
	// HeadingPath is the breadcrumb of headings, outermost first; empty for
	// text before the first heading.
	HeadingPath []string
	// Body is the part's text, starting with its heading line when it has one.
	// The bodies of a note's sections, in order, are the note's body exactly.
	Body string
}

// Split returns the sections of body, or nil when the note fits the model's
// window and stays one unit.
func Split(body string) []Section {
	if !Needed(body) {
		return nil
	}
	return Cut(body)
}

// Needed reports whether text is longer than the model's window. The caller
// passes the text the model is actually given — the index stores a note
// stripped of markdown, and that is what is embedded and truncated.
func Needed(text string) bool {
	return estimateTokens(text) > splitThreshold
}

// TooSmall reports whether text is below the size worth a retrieval unit of
// its own. The indexer asks it of a part after stripping, which drops code.
func TooSmall(text string) bool {
	return estimateTokens(text) < minSectionTokens
}

// Cut divides markdown at its headings, whatever its length. Split is Needed
// and Cut on one text; the indexer decides on the stripped text and cuts the
// markdown, because only the markdown still has its headings.
func Cut(markdown string) []Section {
	var parts []Section
	for _, s := range mergeTiny(atHeadings(markdown)) {
		parts = append(parts, splitLarge(s)...)
	}
	assignAnchors(parts)
	return parts
}

// Prefix is the context line embedded in front of a section:
// "<title> › <heading> › <heading>", or the title alone for text before the
// first heading. A part read on its own loses what "it" refers to.
func Prefix(title string, s Section) string {
	if len(s.HeadingPath) == 0 {
		return title
	}
	return title + PathSeparator + strings.Join(s.HeadingPath, PathSeparator)
}

// estimateTokens is the cheap count the embedder's own pre-cut uses.
func estimateTokens(s string) int {
	return tokensFor(utf8.RuneCountInString(s))
}

// tokensFor is estimateTokens for a rune count already known.
func tokensFor(runes int) int {
	return (runes + embedding.ApproxCharsPerToken - 1) / embedding.ApproxCharsPerToken
}

var headingRE = regexp.MustCompile(`^ {0,3}(#{1,6})[ \t]+(.*?)[ \t]*$`)

// atHeadings cuts body before every heading of level 1–3 outside a fence.
//
// Text accumulates in a builder: appending to a string copies it, and a 1 MiB
// page with no headings took 4.9 s that way.
func atHeadings(body string) []Section {
	var out []Section
	var levels [maxSplitLevel]string
	var curPath []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, Section{HeadingPath: curPath, Body: cur.String()})
			cur.Reset()
		}
	}
	var f fence
	for _, line := range strings.SplitAfter(body, "\n") {
		if line == "" {
			continue
		}
		if !f.inside(line) {
			if level, text, ok := heading(line); ok {
				flush()
				levels[level-1] = text
				for i := level; i < maxSplitLevel; i++ {
					levels[i] = ""
				}
				curPath = path(levels)
			}
		}
		cur.WriteString(line)
	}
	flush()
	return out
}

// heading reports a split-level heading on line and its cleaned text.
func heading(line string) (int, string, bool) {
	m := headingRE.FindStringSubmatch(strings.TrimRight(line, "\r\n"))
	if m == nil || len(m[1]) > maxSplitLevel {
		return 0, "", false
	}
	text := cleanHeading(m[2])
	if text == "" {
		return 0, "", false
	}
	return len(m[1]), text, true
}

var (
	closingHashes = regexp.MustCompile(`[ \t]+#+$`)
	pilcrowLink   = regexp.MustCompile(`\[¶\]\([^)]*\)`)
	markdownLink  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	escapedPunct  = regexp.MustCompile(`\\([[:punct:]])`)
)

// cleanHeading turns a heading's markdown into the words a reader sees: no
// closing #s, no "¶" permalink (Sphinx docs put one on every heading), links
// as their text, backslash escapes undone.
func cleanHeading(s string) string {
	s = closingHashes.ReplaceAllString(s, "")
	s = pilcrowLink.ReplaceAllString(s, "")
	s = markdownLink.ReplaceAllString(s, "$1")
	s = escapedPunct.ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}

func path(levels [maxSplitLevel]string) []string {
	var p []string
	for _, l := range levels {
		if l != "" {
			p = append(p, l)
		}
	}
	return p
}

// mergeTiny folds a too-small part into the one before it; a too-small first
// part (a one-line intro, or blank lines before the first heading) goes into
// the one after instead.
func mergeTiny(in []Section) []Section {
	var out []Section
	for _, s := range in {
		if len(out) > 0 && estimateTokens(s.Body) < minSectionTokens {
			out[len(out)-1].Body += s.Body
			continue
		}
		out = append(out, s)
	}
	if len(out) > 1 && estimateTokens(out[0].Body) < minSectionTokens {
		out[1].Body = out[0].Body + out[1].Body
		out = out[1:]
	}
	return out
}

// splitLarge cuts a part over the cap at blank lines outside fences. A part is
// never cut inside a paragraph or a fence, and never left smaller than the
// minimum, so one huge paragraph stays whole with its heading.
func splitLarge(s Section) []Section {
	if estimateTokens(s.Body) <= maxSectionTokens {
		return []Section{s}
	}
	var out []Section
	var part strings.Builder
	partRunes := 0 // runes in part, so its size is known without re-counting
	for _, block := range units(s.Body) {
		blockRunes := utf8.RuneCountInString(block)
		if part.Len() > 0 && tokensFor(partRunes) >= minSectionTokens &&
			tokensFor(partRunes+blockRunes) > maxSectionTokens {
			out = append(out, Section{HeadingPath: s.HeadingPath, Body: part.String()})
			part.Reset()
			partRunes = 0
		}
		part.WriteString(block)
		partRunes += blockRunes
	}
	switch {
	case part.Len() == 0:
	case len(out) > 0 && tokensFor(partRunes) < minSectionTokens:
		// A closing line or two left over after the last cut: it belongs with
		// what it closes, even if that part runs a little past the cap.
		out[len(out)-1].Body += part.String()
	default:
		out = append(out, Section{HeadingPath: s.HeadingPath, Body: part.String()})
	}
	return out
}

// units are the pieces a part may be cut between: paragraphs, except that a
// paragraph over the cap with no fence in it — a table, a long list — is
// taken line by line. A fenced block is never cut, and neither is one line.
func units(text string) []string {
	var out []string
	for _, p := range paragraphs(text) {
		if estimateTokens(p) <= maxSectionTokens || strings.Contains(p, "```") || strings.Contains(p, "~~~") {
			out = append(out, p)
			continue
		}
		for _, line := range strings.SplitAfter(p, "\n") {
			if line != "" {
				out = append(out, line)
			}
		}
	}
	return out
}

// paragraphs splits text after each blank line outside a fence, keeping every
// byte: the blocks joined are the text.
func paragraphs(text string) []string {
	var out []string
	var f fence
	var block strings.Builder
	for _, line := range strings.SplitAfter(text, "\n") {
		if line == "" {
			continue
		}
		inFence := f.inside(line)
		block.WriteString(line)
		if !inFence && strings.TrimSpace(line) == "" {
			out = append(out, block.String())
			block.Reset()
		}
	}
	if block.Len() > 0 {
		out = append(out, block.String())
	}
	return out
}

// fence tracks whether lines are inside a ``` or ~~~ code block.
type fence struct {
	marker byte
	open   bool
}

// inside reports whether line is part of a fenced block, the fence lines
// themselves included, and advances the state past it.
func (f *fence) inside(line string) bool {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || len(trimmed) < 3 {
		return f.open
	}
	c := trimmed[0]
	if (c != '`' && c != '~') || trimmed[1] != c || trimmed[2] != c {
		return f.open
	}
	switch {
	case !f.open:
		f.marker, f.open = c, true
	case c == f.marker:
		f.open = false
	}
	return true
}

// assignAnchors names each part after its last heading ("preamble" before the
// first), numbers the parts of one cut section -2, -3…, and keeps every anchor
// unique within the note.
func assignAnchors(parts []Section) {
	seen := map[string]bool{}
	for i := range parts {
		base := "preamble"
		if n := len(parts[i].HeadingPath); n > 0 {
			base = slug(parts[i].HeadingPath[n-1])
		}
		anchor := base
		for k := 2; seen[anchor]; k++ {
			anchor = base + "-" + strconv.Itoa(k)
		}
		seen[anchor] = true
		parts[i].Anchor = anchor
		parts[i].Ordinal = i
	}
}

// slug keeps letters and digits, lower-cased, with one dash for every run of
// anything else.
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
			continue
		}
		dash = true
	}
	if b.Len() == 0 {
		return "section"
	}
	return b.String()
}
