package query

import (
	"math"
	"strings"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/parser"
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
func selectPassage(db *index.DB, query, raw, today string, limit int) (passageResult, error) {
	result := passageResult{Text: today}
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
	cs := passageWindows(raw, limit, tokens, lex.terms)
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
func applyPassageExcerpt(db *index.DB, cfg AskConfig, pack *memory.ContextPackResult) error {
	if (cfg.PointersOnly && !cfg.JSONOutput) || cfg.ExcerptTokens <= 0 || pack == nil || pack.Target == nil || !pack.Target.BodyExcerpted {
		return nil
	}
	target := pack.Target
	var raw string
	if err := db.QueryRow("SELECT COALESCE(body_raw, body_text, '') FROM notes WHERE id = ?", target.ID).Scan(&raw); err != nil {
		return err
	}
	if memory.HasPrincipleSection(raw) {
		return nil
	}
	limit := min(cfg.ExcerptTokens, cfg.Budget-pack.UsedTokens+memory.EstimateTokens(target.Body))
	if limit <= 0 {
		return nil
	}
	selected, err := selectPassage(db, cfg.Query, raw, target.Body, limit)
	if err != nil {
		return err
	}
	best := selected.Text
	pack.UsedTokens += memory.EstimateTokens(best) - memory.EstimateTokens(target.Body)
	target.Body = best
	return nil
}
