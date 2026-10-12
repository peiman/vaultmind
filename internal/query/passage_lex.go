package query

import (
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/parser"
)

type passageLexicon struct {
	body, terms []string
	first       map[string]int
	firstLength int
	idf         map[string]float64
}

func passageLex(db *index.DB, query, body, today string) (passageLexicon, error) {
	lex := passageLexicon{first: map[string]int{}, idf: map[string]float64{}}
	tx, err := db.Begin()
	if err != nil {
		return lex, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, sql := range []string{
		"CREATE VIRTUAL TABLE temp.passage_lex_text USING fts5(text)",
		"CREATE VIRTUAL TABLE temp.passage_lex_terms USING fts5vocab('temp','passage_lex_text','instance')",
		"CREATE VIRTUAL TABLE temp.passage_lex_corpus USING fts5vocab('main','fts_notes','col')",
	} {
		if _, err = tx.Exec(sql); err != nil {
			return lex, err
		}
	}
	// Three texts, irrespective of note size or candidate-window count.
	for i, text := range []string{strings.ReplaceAll(query, `"`, ""), parser.StripForFTS(today), body} {
		if _, err = tx.Exec("INSERT INTO temp.passage_lex_text(rowid,text) VALUES(?,?)", i+1, text); err != nil {
			return lex, err
		}
	}
	rows, err := tx.Query("SELECT term,doc,group_concat(offset) FROM temp.passage_lex_terms GROUP BY term,doc")
	if err != nil {
		return lex, err
	}
	for rows.Next() {
		var term, positions string
		var doc int
		if err = rows.Scan(&term, &doc, &positions); err != nil {
			_ = rows.Close()
			return lex, err
		}
		positionsList := strings.Split(positions, ",")
		switch doc {
		case 1: // Query terms are deduplicated and ordered by the corpus query below.
		case 2:
			lex.first[term] += len(positionsList)
			lex.firstLength += len(positionsList)
		case 3:
			for _, position := range positionsList {
				at, _ := strconv.Atoi(position) // SQLite integer offsets.
				for len(lex.body) <= at {
					lex.body = append(lex.body, "")
				}
				lex.body[at] = term
			}
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return lex, err
	}
	var n int
	if err = tx.QueryRow("SELECT count(*) FROM notes").Scan(&n); err != nil {
		return lex, err
	}
	rows, err = tx.Query("SELECT q.term,coalesce(c.doc,0) FROM (SELECT DISTINCT term FROM temp.passage_lex_terms WHERE doc=1) q LEFT JOIN temp.passage_lex_corpus c ON c.term=q.term AND c.col='body_text' ORDER BY q.term")
	if err != nil {
		return lex, err
	}
	for rows.Next() {
		var term string
		var df int
		if err = rows.Scan(&term, &df); err != nil {
			_ = rows.Close()
			return lex, err
		}
		lex.terms = append(lex.terms, term)
		lex.idf[term] = math.Log(1 + (float64(n-df)+.5)/(float64(df)+.5))
	}
	err = rows.Err()
	_ = rows.Close()
	return lex, err
}

// Locate normalized token bytes without reimplementing case or accent folding:
// the actual terms come from SQLite. unicode61 uses L*, N*, Co and attaches
// combining diacritics to an existing token. Count disagreement falls back.
func passageTokenBounds(text string) [][2]int {
	var bounds [][2]int
	start := -1
	for at, r := range text {
		token := unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.Is(unicode.Co, r)
		if token || (start >= 0 && unicode.Is(unicode.Mn, r)) {
			if start < 0 {
				start = at
			}
		} else if start >= 0 {
			bounds = append(bounds, [2]int{start, at})
			start = -1
		}
	}
	if start >= 0 {
		bounds = append(bounds, [2]int{start, len(text)})
	}
	return bounds
}

// Each distinct source token needs unicode61 normalization only once. Repeated
// prose reuses its ordinal, avoiding SQLite work proportional to repetitions.
// This is one vocabulary for the whole note, independent of window count.
func passageVocabulary(text string, bounds [][2]int) (string, []int, int) {
	seen := map[string]int{}
	ordinals := make([]int, len(bounds))
	var vocabulary strings.Builder
	for i, b := range bounds {
		token := text[b[0]:b[1]]
		at, exists := seen[token]
		if !exists {
			at = len(seen)
			seen[token] = at
			if at > 0 {
				vocabulary.WriteByte(' ')
			}
			vocabulary.WriteString(token)
		}
		ordinals[i] = at
	}
	return vocabulary.String(), ordinals, len(seen)
}
