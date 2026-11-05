package query

import (
	"context"
	"math"
	"strings"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/rs/zerolog/log"
)

type passageResult struct {
	Text                  string
	BestScore, FirstScore float64
}

func passageMarginWins(best, first, ratio float64) bool {
	return best > 0 && (first == 0 || best >= first*ratio)
}

// selectPassage normalizes and tokenizes the body once. SQLite supplies the
// keyword lane's actual unicode61 folding for each distinct token; windows reuse that
// token stream and corpus frequencies without indexing individual windows.
func selectPassage(db *index.DB, query, raw, today string, limit int, title string) (passageResult, error) {
	result := passageResult{Text: today}
	raw = parser.StripHTMLComments(raw)
	text, offsets := parser.StripForFTSOffsets(raw)
	bounds := passageTokenBounds(text)
	vocabulary, ordinals, distinct := passageVocabulary(text, bounds)
	lex, err := passageLex(db, query, vocabulary, today)
	if err != nil {
		return result, err
	}
	if distinct != len(lex.body) {
		return result, nil
	} // unfamiliar Unicode boundary: safe fallback
	tokens := make([]passageToken, len(bounds))
	for i, b := range bounds {
		tokens[i] = passageToken{term: lex.body[ordinals[i]], start: offsets[b[0]], end: offsets[b[1]-1] + 1}
	}
	cs := passageWindows(raw, title, limit, tokens, lex.terms)
	if len(cs) == 0 {
		return result, nil
	}
	avg := 0.0
	tokenStart, tokenEnd := 0, 0
	for i := range cs {
		for tokenStart < len(tokens) && tokens[tokenStart].start < cs[i].start {
			tokenStart++
		}
		for tokenEnd < len(tokens) && tokens[tokenEnd].end <= cs[i].end {
			tokenEnd++
		}
		for tokenEnd > 0 && tokens[tokenEnd-1].end > cs[i].end {
			tokenEnd--
		}
		cs[i].lo, cs[i].hi = tokenStart, max(tokenStart, tokenEnd)
		avg += float64(cs[i].hi - cs[i].lo)
	}
	avg = math.Max(1, avg/float64(len(cs)))
	score := func(tf map[string]int, length int) float64 {
		s := 0.0
		for _, term := range lex.terms {
			f := float64(tf[term])
			if f > 0 {
				s += lex.idf[term] * f * (passageBM25K1 + 1) / (f + passageBM25K1*(1-passageBM25B+passageBM25B*float64(length)/avg))
			}
		}
		return s
	}
	result.FirstScore = score(lex.first, lex.firstLength)
	// Maintain counts as the source window moves; no repeated normalization or
	// token counting for overlapping windows. Endpoints can retreat at headings.
	tf := map[string]int{}
	lo, hi, best := 0, 0, -1
	for i, c := range cs {
		for hi < c.hi {
			tf[tokens[hi].term]++
			hi++
		}
		for hi > c.hi {
			hi--
			tf[tokens[hi].term]--
		}
		for lo < c.lo {
			tf[tokens[lo].term]--
			lo++
		}
		for lo > c.lo {
			lo--
			tf[tokens[lo].term]++
		}
		// A long unspaced CJK token can exceed the entire delivery cap. Its
		// normalized query substring supplies a bounded span inside that token.
		for _, term := range c.partial {
			tf[term]++
		}
		s := score(tf, max(1, c.hi-c.lo))
		for _, term := range c.partial {
			tf[term]--
		}
		if s > result.BestScore {
			best, result.BestScore = i, s
		}
	}
	if best >= 0 && passageMarginWins(result.BestScore, result.FirstScore, passageFirstBlockMargin) {
		first := memory.Excerpt(raw, memory.EstimateTokens(raw))
		start := strings.Index(raw, first)
		if cs[best].start < start || cs[best].start >= start+len(first) {
			result.Text = cs[best].deliver(raw, tokens, lex.terms)
		}
	}
	return result, nil
}

// passageFirstBlockMargin is the frozen S5 policy: a later window must score
// at least twice the existing first-block excerpt before it replaces it.
const (
	passageFirstBlockMargin = 2.0
	// BM25 saturation and length normalization match the frozen lexical probe.
	passageBM25K1 = 1.2
	passageBM25B  = 0.75
	// Delivery reserves room for cut markers and up to 64 bytes of lead-in.
	passageLeadInBytes = 64
	passageCutReserve  = 6
)

// applyPassageExcerpt changes only ask's already-excerpted target. Packing has
// already reserved the context bodies, so a larger window must fit the budget
// left after those reservations; it cannot evict or rewrite a neighbor.
func applyPassageExcerpt(ctx context.Context, db *index.DB, cfg AskConfig, rankingQuery string, pack *memory.ContextPackResult) error {
	if pack != nil && pack.Target != nil {
		pack.Target.PassageSelector = "none"
	}
	if (cfg.PointersOnly && !cfg.JSONOutput) || cfg.ExcerptTokens <= 0 || pack == nil || pack.Target == nil || !pack.Target.BodyExcerpted {
		return nil
	}
	target := pack.Target
	title, _ := target.Frontmatter["title"].(string)
	note := passageNote{id: target.ID, title: title, packed: target.Body}
	left := cfg.Budget - pack.UsedTokens + memory.EstimateTokens(target.Body)
	best, selector, err := notePassage(ctx, db, cfg, rankingQuery, note, min(cfg.ExcerptTokens, left), min(2*cfg.ExcerptTokens, left))
	target.PassageSelector = selector
	if err != nil {
		return err
	}
	pack.UsedTokens += memory.EstimateTokens(best) - memory.EstimateTokens(target.Body)
	target.Body = best
	return nil
}

// overflowPassage is ask's OverflowExcerpt (#331): the same selection the
// target gets, for a context note whose whole text does not fit the room
// left, sized to that room. The packer calls it in rank order, so the note
// is sized before any lower-ranked note takes room. Text-mode pointers-only
// renders no bodies, so it skips the selection there, as the target does.
func overflowPassage(ctx context.Context, db *index.DB, cfg AskConfig, rankingQuery string) func(*index.FullNote, int) (string, string) {
	return func(n *index.FullNote, room int) (string, string) {
		packed := memory.Excerpt(n.Body, room)
		if packed == "" || (cfg.PointersOnly && !cfg.JSONOutput) {
			return packed, "none"
		}
		text, selector, err := notePassage(ctx, db, cfg, rankingQuery, passageNote{id: n.ID, title: n.Title, packed: packed}, room, room)
		if err != nil {
			log.Debug().Err(err).Str("note_id", n.ID).Msg("passage selection unavailable; keeping original excerpt")
		}
		return text, selector
	}
}

// passageNote is one packed note the selector may re-excerpt: its id, its
// title, and the excerpt the packer chose for it.
type passageNote struct {
	id, title, packed string
}

// notePassage is #330's selection for one note. limit bounds the lexical
// pick; total bounds the pick plus the dense blocks added after it. It returns
// the text to deliver and the selector that chose it. On "none", "principle"
// or an error the packed excerpt comes back unchanged.
func notePassage(ctx context.Context, db *index.DB, cfg AskConfig, rankingQuery string, note passageNote, limit, total int) (string, string, error) {
	var raw string
	if err := db.QueryRow("SELECT COALESCE(body_raw, body_text, '') FROM notes WHERE id = ?", note.id).Scan(&raw); err != nil {
		return note.packed, "none", err
	}
	if memory.HasPrincipleSection(raw) {
		return note.packed, "principle", nil
	}
	if limit <= 0 {
		return note.packed, "none", nil
	}
	selected, err := selectPassage(db, cfg.Query, raw, note.packed, limit, note.title)
	if err != nil {
		return note.packed, "lexical", err
	}
	best := selected.Text
	if best == note.packed {
		best = firstBlockHeadingExcerpt(raw, best, note.title, limit)
	}
	// The lexical pick answers exact terms and has first claim on the budget;
	// dense blocks answer paraphrases in whatever room is left.
	separator := ""
	if best != "" {
		separator = passageGap
	}
	allowance := total - memory.EstimateTokens(best+separator)
	if dense := selectDensePassages(ctx, db, cfg, note.id, rankingQuery, raw, note.title, best, allowance); dense != "" {
		return best + separator + dense, "dense", nil
	}
	return best, "lexical", nil
}

// passageGap joins delivered text that is not adjacent in the note.
const passageGap = "\n…\n"

// firstBlockHeadingTokens bounds the nearest heading on a kept first block.
// Sixteen tokens is at most a tenth of the 160-token excerpt, so a warning
// can travel with its block without consuming the answer. The number was
// fixed before measurement.
const firstBlockHeadingTokens = 16

// firstBlockHeadingExcerpt prefixes only the nearest governing heading when
// the retained block sits under a heading other than the note title. The
// full path is left off: a long ancestor list ate the excerpt and dropped
// the answer. The heading is capped; the prose is what gets trimmed. A block
// directly under the title is unchanged.
func firstBlockHeadingExcerpt(raw, today string, title string, limit int) string {
	first := memory.Excerpt(raw, memory.EstimateTokens(raw))
	start := strings.Index(raw, first)
	if first == "" || start < 0 {
		return today
	}
	for _, unit := range passageUnits(raw, title) {
		if unit.start <= start && start < unit.end {
			heading := nearestGoverningHeading(unit.heading, title)
			if heading == "" {
				return today
			}
			heading = capHeadingTokens(heading, firstBlockHeadingTokens)
			if heading == "" {
				return today
			}
			return fitHeadingAndBlock(heading, today, limit)
		}
	}
	return today
}

// nearestGoverningHeading returns the closest heading in a top-down path,
// including a setext underline. The title is already delivered as metadata,
// and parent headings are not part of this prefix.
func nearestGoverningHeading(path, title string) string {
	if path == "" {
		return ""
	}
	lines := strings.Split(path, "\n")
	start := len(lines) - 1
	if start > 0 && passageSetext.MatchString(lines[start]) {
		start--
	}
	label := strings.TrimSpace(strings.TrimLeft(lines[start], "#"))
	if label == "" || label == title {
		return ""
	}
	return strings.Join(lines[start:], "\n")
}

// capHeadingTokens cuts a heading at a word boundary and marks the cut.
// A single token wider than the cap is cut by rune so the result still fits.
func capHeadingTokens(heading string, maxTokens int) string {
	if memory.EstimateTokens(heading) <= maxTokens {
		return heading
	}
	const ellipsis = "…"
	words := strings.Fields(heading)
	var kept []string
	for _, word := range words {
		candidate := strings.Join(append(kept, word), " ") + ellipsis
		if memory.EstimateTokens(candidate) > maxTokens {
			break
		}
		kept = append(kept, word)
	}
	if len(kept) > 0 {
		return strings.Join(kept, " ") + ellipsis
	}
	var b strings.Builder
	for _, r := range heading {
		candidate := b.String() + string(r) + ellipsis
		if memory.EstimateTokens(candidate) > maxTokens {
			break
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return ""
	}
	return b.String() + ellipsis
}

// fitHeadingAndBlock reserves the heading and trims the prose to the excerpt
// cap. The heading is not shortened to make more of the block fit. When the
// heading itself exceeds the cap, the packed block is left as it was.
func fitHeadingAndBlock(heading, today string, limit int) string {
	if memory.EstimateTokens(heading) > limit {
		return today
	}
	prefix := heading + "\n\n"
	budget := limit - memory.EstimateTokens(prefix)
	if budget <= 0 {
		return heading
	}
	text := memory.Excerpt(today, budget)
	for text != "" && memory.EstimateTokens(prefix+text) > limit {
		budget--
		if budget <= 0 {
			text = ""
			break
		}
		text = memory.Excerpt(today, budget)
	}
	if text == "" {
		return heading
	}
	return prefix + text
}
