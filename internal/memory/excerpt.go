package memory

import "strings"

// arcSections are the headings the arc discipline prescribes, in order. They are
// the parse boundaries for Excerpt: an arc's decision rule lives under
// principleHeading, and everything after it is provenance the agent does not
// need at a decision point.
//
// Keeping the list here rather than inferring "any short line" is deliberate —
// a heuristic that guesses at headings will silently mis-slice a note, and a
// mis-sliced excerpt looks exactly like a correct one.
var arcSections = []string{
	"Trigger",
	"Push",
	"Deeper sight",
	principleHeading,
	"What I did differently",
	"Source",
}

// principleHeading names the section that carries an arc's decision rule — the
// sentence that changes what an agent does. It is what Excerpt delivers when it
// is present.
const principleHeading = "Principle"

// ellipsis marks an excerpt cut inside a sentence, so a truncated line reads as
// deliberately shortened rather than as a note that simply stops.
const ellipsis = "…"

// Excerpt returns the most decision-relevant part of a note body, within
// maxTokens.
//
// WHY THIS EXISTS. The packer includes bodies all-or-nothing
// (`if bodyTokens <= *remaining`), so a note larger than the leftover budget
// contributes nothing at all and the pack reports "3 items, 0 with bodies" —
// items present, content absent. Measured on the identity vault, whose median
// note is ~1,084 tokens against a 900-token hook budget, that is the common
// case rather than the edge case.
//
// An excerpt always fits, so it turns "no body" into "the part that matters".
//
// It prefers the Principle section because of how arcs are written: the first
// paragraph is the Trigger — story setup — and the rule an agent could act on is
// several sections down. Injecting the lead hands over the anecdote and withholds
// the lesson.
func Excerpt(body string, maxTokens int) string {
	if strings.TrimSpace(body) == "" || maxTokens <= 0 {
		return ""
	}
	body = stripFrontmatter(body)
	text := principleSection(body)
	if text == "" {
		text = leadParagraph(body)
	}
	return truncateToTokens(text, maxTokens)
}

// frontmatterFence delimits a note's YAML frontmatter block.
const frontmatterFence = "---"

// stripFrontmatter removes a leading YAML block so it cannot be mistaken for the
// note's opening paragraph. Callers that pass an already-parsed body are
// unaffected; callers that pass a raw note file would otherwise get `id: … type:
// … tags: …` delivered as the memory.
func stripFrontmatter(body string) string {
	lines := strings.Split(body, "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	if start >= len(lines) || strings.TrimSpace(lines[start]) != frontmatterFence {
		return body
	}
	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == frontmatterFence {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return body // unterminated fence: treat the whole body as content
}

// principleSection returns the text under the Principle heading, or "" when the
// note has no such section (references, concepts and sources generally do not).
func principleSection(body string) string {
	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		if isSectionHeading(line, principleHeading) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return ""
	}
	var out []string
	for _, line := range lines[start:] {
		if isAnySectionHeading(line) {
			break
		}
		out = append(out, strings.TrimSpace(line))
	}
	return strings.TrimSpace(strings.Join(out, " "))
}

// leadParagraph returns the note's first block of actual prose.
//
// It identifies prose rather than identifying headings, because headings are
// not reliably marked: `note get` renders them bare, so a note can open with a
// lone "Overview" line that carries no "#" and is not one of the arc sections.
// Matching on shape ("a short line followed by a blank") is the fragile version
// of this and would mis-slice a note that genuinely opens with a short sentence.
//
// A block counts as prose when it ends a sentence or runs longer than a heading
// plausibly would. Headings do neither.
func leadParagraph(body string) string {
	var firstContent string
	for _, block := range strings.Split(body, "\n\n") {
		var lines []string
		for _, line := range strings.Split(block, "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				lines = append(lines, trimmed)
			}
		}
		text := strings.TrimSpace(strings.Join(lines, " "))
		if text == "" || isAnySectionHeading(text) || strings.HasPrefix(text, "#") {
			continue
		}
		if isProse(text) {
			return text
		}
		// Not prose-shaped, but it IS content. Hold the first such block as a
		// fallback rather than discarding it — see below.
		if firstContent == "" {
			firstContent = text
		}
	}
	// THE INVARIANT: a non-empty body never excerpts to nothing. Plenty of real
	// notes are bullet lists, tables, or short CJK lines — none of which contain
	// ASCII sentence punctuation or run longer than a heading. Returning ""
	// for those turned a cap into content loss: a note that previously arrived
	// whole arrived empty instead.
	return firstContent
}

// maxHeadingChars is the length past which a block is prose regardless of
// punctuation — a heading that long is not a heading.
const maxHeadingChars = 80

// sentenceEnders covers the CJK forms alongside the ASCII ones. Without the
// wide variants, 研究は重要である。 reads as "not prose" — it ends in a full stop the
// ASCII set does not contain, and is far too short to clear maxHeadingChars.
const sentenceEnders = ".!?。！？"

func isProse(text string) bool {
	return strings.ContainsAny(text, sentenceEnders) || len(text) > maxHeadingChars
}

// isSectionHeading reports whether line is the named heading, with or without
// Markdown "#" markers — `note get` renders headings bare, files carry the "#".
func isSectionHeading(line, name string) bool {
	trimmed := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
	return strings.EqualFold(trimmed, name)
}

func isAnySectionHeading(line string) bool {
	for _, s := range arcSections {
		if isSectionHeading(line, s) {
			return true
		}
	}
	return false
}

// truncateToTokens keeps whole sentences while they fit, so an excerpt ends
// where a thought ends. Only when not even the first sentence fits does it cut
// mid-sentence, and then it says so with an ellipsis rather than stopping
// mid-word the way the current teaser does.
func truncateToTokens(text string, maxTokens int) string {
	if text == "" || EstimateTokens(text) <= maxTokens {
		return text
	}
	var kept string
	for _, sentence := range splitSentences(text) {
		candidate := strings.TrimSpace(kept + " " + sentence)
		if EstimateTokens(candidate) > maxTokens {
			break
		}
		kept = candidate
	}
	if kept != "" {
		return kept
	}
	return hardTruncate(text, maxTokens)
}

// splitSentences breaks on sentence-ending punctuation followed by a space,
// keeping the punctuation with the sentence it ends.
func splitSentences(text string) []string {
	var out []string
	start := 0
	runes := []rune(text)
	for i := 0; i < len(runes)-1; i++ {
		if (runes[i] == '.' || runes[i] == '!' || runes[i] == '?') && runes[i+1] == ' ' {
			out = append(out, strings.TrimSpace(string(runes[start:i+1])))
			start = i + 1
		}
	}
	if tail := strings.TrimSpace(string(runes[start:])); tail != "" {
		out = append(out, tail)
	}
	return out
}

// hardTruncate cuts at a word boundary and marks the cut, for the case where a
// single sentence exceeds the whole budget.
//
// When not even one word fits it returns "" rather than a bare ellipsis. An
// ellipsis alone is a delivery that delivers nothing, and callers mark an item
// as carrying a body whenever the excerpt is non-empty — so returning "…" let a
// pack report "3 items, 3 excerpted" while handing the agent three ellipses.
// Returning empty makes that state unexpressible instead of merely unlikely.
func hardTruncate(text string, maxTokens int) string {
	words := strings.Fields(text)
	var kept []string
	for _, w := range words {
		candidate := append(kept, w) //nolint:gocritic // candidate is discarded on overflow
		if EstimateTokens(strings.Join(candidate, " ")+ellipsis) > maxTokens {
			break
		}
		kept = candidate
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, " ") + ellipsis
}
