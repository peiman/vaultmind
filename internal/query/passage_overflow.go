package query

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/passage"
	"github.com/rs/zerolog/log"
)

// overflowPassage is ask's OverflowExcerpt (#331), for a context note whose
// whole text does not fit the room left. The packer calls it in rank order,
// so the note is sized before any lower-ranked note takes room. Text-mode
// pointers-only renders no bodies, so it skips the selection there, as the
// target does.
func overflowPassage(ctx context.Context, db *index.DB, cfg AskConfig, rankingQuery string) func(*index.FullNote, int) (string, string) {
	return func(n *index.FullNote, room int) (string, string) {
		packed := memory.Excerpt(n.Body, room)
		if packed == "" || (cfg.PointersOnly && !cfg.JSONOutput) {
			return packed, "none"
		}
		text, selector, err := overflowSelection(ctx, db, cfg, rankingQuery, passageNote{id: n.ID, title: n.Title, packed: packed}, room)
		if err != nil {
			log.Debug().Err(err).Str("note_id", n.ID).Msg("passage selection unavailable; keeping original excerpt")
		}
		return text, selector
	}
}

// overflowSelection makes the same picks as the target's selection (the
// keyword window first, then up to three dense blocks) and then uses the room:
// each pick grows to the heading section around it. A pick alone is a
// sentence or a paragraph; on a note too large to send whole, a room of
// hundreds of tokens is better spent on the section that holds the answer,
// with its code, than on three lines from three places (#331).
func overflowSelection(ctx context.Context, db *index.DB, cfg AskConfig, rankingQuery string, note passageNote, room int) (string, string, error) {
	var raw string
	if err := db.QueryRow("SELECT COALESCE(body_raw, body_text, '') FROM notes WHERE id = ?", note.id).Scan(&raw); err != nil {
		return note.packed, "none", err
	}
	if memory.HasPrincipleSection(raw) {
		return note.packed, "principle", nil
	}
	raw = parser.StripHTMLComments(raw)
	selected, err := selectPassage(db, cfg.Query, raw, note.packed, room, note.title, isNavigation)
	if err != nil {
		return note.packed, "lexical", err
	}
	units, ranked := rankedDenseBlocks(ctx, db, cfg, note.id, rankingQuery, raw, note.title)
	picks := append([]passagePick{lexicalPick(raw, selected)}, densePicks(raw, units, ranked)...)
	text, dense := growPicks(raw, note.title, picks, room)
	switch {
	case text == "":
		return note.packed, "none", nil
	case dense:
		return text, "dense", nil
	}
	return text, "lexical", nil
}

// passagePick is a span of the comment-stripped body that a lane chose.
type passagePick struct {
	start, end int
	dense      bool
}

// passageSpan is a delivered stretch of the body and its rendered text.
type passageSpan struct {
	start, end int
	text       string
}

// lexicalPick is the keyword window, or the opening block when no window
// beat it. start is -1 when the body has neither.
func lexicalPick(raw string, selected passageResult) passagePick {
	if selected.End > 0 {
		return passagePick{start: selected.Start, end: selected.End}
	}
	first := memory.Excerpt(raw, memory.EstimateTokens(raw))
	start := strings.Index(raw, first)
	if first == "" || start < 0 {
		return passagePick{start: -1}
	}
	return passagePick{start: start, end: start + len(first)}
}

// densePicks is the top three dense blocks that are not navigation. A block
// that only links elsewhere in the note is skipped, not counted.
func densePicks(raw string, units []passage.Unit, ranked []densePassageBlock) []passagePick {
	var picks []passagePick
	for _, b := range ranked {
		if len(picks) == densePassageBlocks {
			break
		}
		start, end := units[b.first].Start, units[b.last].End
		if !isNavigation(raw[start:end]) {
			picks = append(picks, passagePick{start: start, end: end, dense: true})
		}
	}
	return picks
}

// passageNavLine is one line made only of links to headings in the same note,
// as a list item or bare, e.g. "- [Setup](#setup)" or "[A](#a) | [B](#b)".
var passageNavLine = regexp.MustCompile(`^(?:[-*+]|\d+[.)])?\s*(?:\[[^\]]*\]\(#[^)\s]*\)[\s,;|]*)+$`)

// isNavigation reports whether text is only in-note links: a table of
// contents or a "jump to" line. It repeats the headings' words, so it scores
// well against a question, but it names where the answer is rather than
// giving it.
func isNavigation(text string) bool {
	lines := 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !passageNavLine.MatchString(trimmed) {
			return false
		}
		lines++
	}
	return lines > 0
}

// growPicks delivers the picks in order of preference while room is left,
// each grown by growPick and charged to the room, and renders them in
// document order. dense reports whether a dense pick was delivered.
func growPicks(raw, title string, picks []passagePick, room int) (string, bool) {
	sections := passage.Sections(raw, title)
	gap := memory.EstimateTokens(passageGap)
	var spans []passageSpan
	left, dense := room, false
	for _, p := range picks {
		if p.start < 0 || covered(spans, p) {
			continue
		}
		avail := left
		if len(spans) > 0 {
			avail -= gap
		}
		span, ok := growPick(raw, sections, spans, p, avail)
		if !ok {
			continue
		}
		spans = append(spans, span)
		left = avail - memory.EstimateTokens(span.text)
		dense = dense || p.dense
	}
	return renderSpans(raw, spans), dense
}

// growPick returns the outermost heading section around p that avail tokens
// can hold, cut at a line if need be, as long as the cut still holds p. A
// section that overlaps text already chosen is passed over. With none, p is
// delivered alone under its headings, cut to fit.
func growPick(raw string, sections []passage.Section, spans []passageSpan, p passagePick, avail int) (passageSpan, bool) {
	// Sections are in document order, so an enclosing one comes before the
	// sections inside it: the first that qualifies is the outermost.
	for _, s := range sections {
		if s.Start > p.start || p.end > s.End || overlaps(spans, s.Start, s.End) {
			continue
		}
		if span, ok := sectionSpan(raw, sections, s, avail); ok && span.end >= p.end {
			return span, true
		}
	}
	if overlaps(spans, p.start, p.end) {
		return passageSpan{}, false
	}
	prefix := ancestorHeadings(sections, p.start, p.end)
	text := fitPick(strings.TrimSpace(raw[p.start:p.end]), avail*4-len(prefix), p.dense)
	if text == "" {
		return passageSpan{}, false
	}
	return passageSpan{start: p.start, end: p.end, text: prefix + text}, true
}

// fitPick returns text whole when it fits budget bytes, else cut at a line.
// When not one line fits, the keyword pick is cut at a word, as the packer
// cuts; a dense pick is dropped, since a word or two of a block elsewhere in
// the note is a fragment, not an answer. "" when nothing fits.
func fitPick(text string, budget int, dense bool) string {
	if len(text) <= budget {
		return text
	}
	if kept, cut := cutAtLine(text, budget); kept > 0 {
		return cut
	}
	if dense {
		return ""
	}
	return memory.Excerpt(text, budget/4)
}

// sectionSpan renders s under the headings of the sections around it, cut at
// a line when it does not fit avail tokens. It fails when not even the
// heading fits.
func sectionSpan(raw string, sections []passage.Section, s passage.Section, avail int) (passageSpan, bool) {
	prefix := ancestorHeadings(sections, s.Start, s.End)
	budget := avail*4 - len(prefix) // EstimateTokens is ceil(bytes/4)
	body := strings.TrimRight(raw[s.Start:s.End], " \t\r\n")
	if len(body) <= budget {
		return passageSpan{start: s.Start, end: s.Start + len(body), text: prefix + body}, true
	}
	kept, text := cutAtLine(body, budget)
	if kept == 0 {
		return passageSpan{}, false
	}
	return passageSpan{start: s.Start, end: s.Start + kept, text: prefix + text}, true
}

// cutAtLine keeps the longest run of whole lines of body that fits budget
// bytes once marked as cut. A code fence left open is closed, so the cut
// cannot turn the rest of the answer into code. It returns the bytes of body
// kept (0 when not one line fits) and the text to deliver.
func cutAtLine(body string, budget int) (int, string) {
	const more = "\n…"
	kept, best, bestText, fence := 0, 0, "", ""
	for _, line := range strings.SplitAfter(body, "\n") {
		trimmed := strings.TrimSpace(line)
		for _, marker := range []string{"```", "~~~"} {
			if strings.HasPrefix(trimmed, marker) {
				switch fence {
				case "":
					fence = marker
				case marker:
					fence = ""
				}
			}
		}
		kept += len(line)
		text := strings.TrimRight(body[:kept], " \t\r\n")
		if fence != "" {
			text += "\n" + fence
		}
		text += more
		if len(text) > budget {
			break
		}
		best, bestText = kept, text
	}
	return best, bestText
}

// ancestorHeadings is the headings of the sections that enclose [start, end)
// and start before it, outermost first, each capped like a block heading.
func ancestorHeadings(sections []passage.Section, start, end int) string {
	var path []string
	for _, s := range sections {
		if s.Start < start && end <= s.End {
			if heading := capHeadingTokens(s.Heading, firstBlockHeadingTokens); heading != "" {
				path = append(path, heading)
			}
		}
	}
	if len(path) == 0 {
		return ""
	}
	return strings.Join(path, "\n") + "\n\n"
}

func covered(spans []passageSpan, p passagePick) bool {
	for _, s := range spans {
		if s.start <= p.start && p.end <= s.end {
			return true
		}
	}
	return false
}

func overlaps(spans []passageSpan, start, end int) bool {
	for _, s := range spans {
		if start < s.end && s.start < end {
			return true
		}
	}
	return false
}

// renderSpans joins the spans in document order: adjacent ones as paragraphs,
// the rest with the gap marker.
func renderSpans(raw string, spans []passageSpan) string {
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	var out strings.Builder
	for i, s := range spans {
		if i > 0 {
			if strings.TrimSpace(raw[spans[i-1].end:s.start]) == "" {
				out.WriteString("\n\n")
			} else {
				out.WriteString(passageGap)
			}
		}
		out.WriteString(s.text)
	}
	return out.String()
}
