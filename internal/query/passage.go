package query

import (
	"math"
	"regexp"
	"strings"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/parser"
)

type passageResult struct {
	Text                  string
	BestScore, FirstScore float64
}

var passageSentenceBoundary = regexp.MustCompile(`[.!?][ \t]+|\n[ \t]*(?:[-*+] |[0-9]+\. )`)

// passageWindows preserves raw source spans and fence boundaries. Windows begin
// at sentence/bullet boundaries and grow to the excerpt limit, without crossing fenced code.
func passageWindows(body string, limit int) []passageWindow {
	type span struct{ start, end int }
	var runs []span
	start, offset, fenced := 0, 0, false
	for _, line := range strings.SplitAfter(body, "\n") {
		fence := strings.HasPrefix(strings.TrimSpace(line), "```") || strings.HasPrefix(strings.TrimSpace(line), "~~~")
		if fence {
			if !fenced && offset > start {
				runs = append(runs, span{start, offset})
			}
			fenced = !fenced
			start = offset + len(line)
		}
		offset += len(line)
	}
	if !fenced && offset > start {
		runs = append(runs, span{start, offset})
	}
	var out []passageWindow
	type candidateKey struct {
		start int
		text  string
	}
	seen := map[candidateKey]bool{}
	for _, run := range runs {
		var units []span
		raw := body[run.start:run.end]
		blockStart := 0
		ends := passageBlank.FindAllStringIndex(raw, -1)
		ends = append(ends, []int{len(raw), len(raw)})
		for _, end := range ends {
			block := raw[blockStart:end[0]]
			trimmed := strings.TrimSpace(block)
			origin := run.start + blockStart + strings.Index(block, trimmed)
			blockStart = end[1]
			if trimmed == "" {
				continue
			}
			headingsOnly := true
			for _, line := range strings.Split(trimmed, "\n") {
				if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
					headingsOnly = false
				}
			}
			if headingsOnly {
				continue
			}
			boundaries := []int{0}
			for _, loc := range passageSentenceBoundary.FindAllStringIndex(trimmed, -1) {
				at := loc[1]
				if trimmed[loc[0]] == '\n' {
					at = loc[0] + 1
				}
				boundaries = append(boundaries, at)
			}
			boundaries = append(boundaries, len(trimmed))
			for i := 0; i < len(boundaries)-1; i++ {
				if strings.TrimSpace(trimmed[boundaries[i]:boundaries[i+1]]) != "" {
					units = append(units, span{origin + boundaries[i], origin + boundaries[i+1]})
				}
			}
		}
		for i, u := range units {
			kept := ""
			for _, v := range units[i:] {
				proposed := strings.TrimSpace(body[u.start:v.end])
				if memory.EstimateTokens(proposed) > limit {
					if kept == "" {
						kept = trimPassage(body[v.start:v.end], limit)
					}
					break
				}
				kept = proposed
			}
			key := candidateKey{u.start, kept}
			if kept != "" && !seen[key] {
				out = append(out, passageWindow{Text: kept})
				seen[key] = true
			}
		}
	}
	return out
}

func passageMarginWins(best, first, ratio float64) bool {
	return best > 0 && (first == 0 || best >= first*ratio)
}

// All normalization is the keyword lane's: StripForFTS for bodies, and
// SQLite's actual default unicode61 tokenizer for both query and candidates.
// Temporary FTS tables exist only in this connection; rollback leaves the
// index and ranking inputs unchanged. Corpus DF is read from existing FTS.
func selectPassage(db *index.DB, query, raw, today string, limit int) (passageResult, error) {
	result := passageResult{Text: today}
	cs := passageWindows(raw, limit)
	if len(cs) == 0 {
		return result, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, sql := range []string{
		"CREATE VIRTUAL TABLE temp.passage_lex_text USING fts5(text)",
		"CREATE VIRTUAL TABLE temp.passage_lex_terms USING fts5vocab('temp','passage_lex_text','instance')",
		"CREATE VIRTUAL TABLE temp.passage_lex_corpus USING fts5vocab('main','fts_notes','col')",
	} {
		if _, err = tx.Exec(sql); err != nil {
			return result, err
		}
	}
	// Rows 1 and 2 are the query and existing excerpt; the remaining rows
	// are candidate windows. All bodies use the keyword lane's normalization.
	texts := []string{strings.ReplaceAll(query, `"`, ""), parser.StripForFTS(today)}
	for _, c := range cs {
		texts = append(texts, parser.StripForFTS(c.Text))
	}
	stmt, err := tx.Prepare("INSERT INTO temp.passage_lex_text(rowid,text) VALUES(?,?)")
	if err != nil {
		return result, err
	}
	for i, text := range texts {
		if _, err = stmt.Exec(i+1, text); err != nil {
			_ = stmt.Close()
			return result, err
		}
	}
	_ = stmt.Close()
	tfs := make([]map[string]int, len(texts))
	lengths := make([]int, len(texts))
	for i := range tfs {
		tfs[i] = map[string]int{}
	}
	rows, err := tx.Query("SELECT term,doc,count(*) FROM temp.passage_lex_terms GROUP BY term,doc")
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var term string
		var doc, count int
		if err = rows.Scan(&term, &doc, &count); err != nil {
			_ = rows.Close()
			return result, err
		}
		tfs[doc-1][term] = count
		lengths[doc-1] += count
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return result, err
	}
	var n int
	if err = tx.QueryRow("SELECT count(*) FROM notes").Scan(&n); err != nil {
		return result, err
	}
	idf := map[string]float64{}
	// Deterministic query-term iteration avoids floating-point map-order drift.
	qrows, err := tx.Query("SELECT term,doc FROM temp.passage_lex_corpus WHERE col='body_text' AND term IN (SELECT term FROM temp.passage_lex_terms WHERE doc=1) ORDER BY term")
	if err != nil {
		return result, err
	}
	var qterms []string
	for qrows.Next() {
		var term string
		var df int
		if err = qrows.Scan(&term, &df); err != nil {
			_ = qrows.Close()
			return result, err
		}
		idf[term] = math.Log(1 + (float64(n-df)+.5)/(float64(df)+.5))
		qterms = append(qterms, term)
	}
	err = qrows.Err()
	_ = qrows.Close()
	if err != nil {
		return result, err
	}
	avg := 0.0
	for _, l := range lengths[2:] {
		avg += float64(l)
	}
	avg /= float64(len(cs))
	if avg == 0 {
		avg = 1
	}
	score := func(i int) float64 {
		s := 0.0
		for _, term := range qterms {
			tf := float64(tfs[i][term])
			if tf > 0 {
				s += idf[term] * tf * (passageBM25K1 + 1) / (tf + passageBM25K1*(1-passageBM25B+passageBM25B*float64(lengths[i])/avg))
			}
		}
		return s
	}
	result.FirstScore = score(1)
	best := -1
	for i := range cs {
		s := score(i + 2)
		if s > result.BestScore {
			best = i
			result.BestScore = s
		}
	}
	if best < 0 {
		return result, nil
	}
	if !passageMarginWins(result.BestScore, result.FirstScore, passageFirstBlockMargin) {
		return result, nil
	}
	result.Text = cs[best].Text
	return result, nil
}

// passageFirstBlockMargin is the frozen S5 policy: a later window must score
// at least twice the existing first-block excerpt before it replaces it.
const (
	passageFirstBlockMargin = 2.0
	// BM25 saturation and length normalization match the frozen lexical probe.
	passageBM25K1 = 1.2
	passageBM25B  = 0.75
)

type passageWindow struct{ Text string }

var passageBlank = regexp.MustCompile(`\n\s*\n`)

// trimPassage bounds an oversized sentence at a word boundary, preserving raw
// identifiers and marking the cut. Ordinary windows are raw source spans.
func trimPassage(s string, limit int) string {
	s = strings.TrimSpace(s)
	if memory.EstimateTokens(s) <= limit {
		return s
	}
	var kept []string
	for _, w := range strings.Fields(s) {
		if memory.EstimateTokens(strings.Join(append(kept, w), " ")+"…") > limit {
			break
		}
		kept = append(kept, w)
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, " ") + "…"
}

// applyPassageExcerpt changes only ask's already-excerpted target. Packing has
// already reserved the context bodies, so a larger window must fit the budget
// left after those reservations; it cannot evict or rewrite a neighbor.
func applyPassageExcerpt(db *index.DB, cfg AskConfig, pack *memory.ContextPackResult) error {
	if cfg.ExcerptTokens <= 0 || pack == nil || pack.Target == nil || !pack.Target.BodyExcerpted {
		return nil
	}
	target := pack.Target
	var raw string
	if err := db.QueryRow("SELECT body_raw FROM notes WHERE id = ?", target.ID).Scan(&raw); err != nil {
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
