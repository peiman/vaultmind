package index

import (
	"fmt"
	"strings"

	"github.com/peiman/vaultmind/internal/parser"
)

// FTSResult represents a single full-text search hit per SRS-09.
type FTSResult struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Title    string  `json:"title"`
	Path     string  `json:"path"`
	Snippet  string  `json:"snippet"`
	Score    float64 `json:"score"`
	IsDomain bool    `json:"is_domain_note"`
}

// StatusDraft is the frontmatter status of a note that has not been accepted.
// Accepting a draft is flipping this value. Other status values are unchanged.
const StatusDraft = "draft"

// SearchFilters holds optional filters for FTS search.
type SearchFilters struct {
	Type          string // Filter by note type (empty = no filter)
	Tag           string // Filter by tag (empty = no filter)
	PathPrefix    string // Only notes whose vault path starts with this prefix (empty = no filter)
	IncludeDrafts bool   // Rank notes whose status is draft. Off by default.
}

// ExcludesDrafts reports whether a search drops status:draft notes.
// Every retrieval lane calls this, so CLI, MCP, serve and federation agree.
func (f SearchFilters) ExcludesDrafts() bool { return !f.IncludeDrafts }

// PathPrefixLike is the LIKE pattern for a vault-path prefix: %, _ and \ in
// the prefix match literally, and the pattern ends in %. tree --path and
// ask --path both use it.
func PathPrefixLike(prefix string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix) + "%"
}

// SearchFTS performs a full-text search against the fts_notes table.
// Returns results ordered by relevance (rank), limited and offset as specified.
func SearchFTS(d *DB, query string, limit, offset int, filters ...SearchFilters) ([]FTSResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	// Sanitize query for FTS5: wrap each word in double quotes for literal matching.
	// This prevents FTS5 syntax characters (", (, ), :, *, AND, OR, NOT) from
	// being interpreted as operators.
	query = sanitizeFTSQuery(query)

	// Build query with optional filters
	var f SearchFilters
	if len(filters) > 0 {
		f = filters[0]
	}

	q := `SELECT f.note_id, n.type, n.title, n.path,
		substr(COALESCE(n.body_raw, n.body_text, ''), 1, 200), rank, n.is_domain
		FROM fts_notes f
		JOIN notes n ON n.id = f.note_id
		WHERE fts_notes MATCH ?`
	args := []interface{}{query}
	q, args = appendSearchFilters(q, args, f)

	q += " ORDER BY rank LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("FTS search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []FTSResult
	for rows.Next() {
		var r FTSResult
		var noteType, title, path *string
		if scanErr := rows.Scan(&r.ID, &noteType, &title, &path,
			&r.Snippet, &r.Score, &r.IsDomain); scanErr != nil {
			return nil, fmt.Errorf("scanning FTS result: %w", scanErr)
		}
		if noteType != nil {
			r.Type = *noteType
		}
		if title != nil {
			r.Title = *title
		}
		if path != nil {
			r.Path = *path
		}
		// The preview is the first 200 characters of the stored body. A marker
		// inside that window is bookkeeping, not the note. Author comments stay.
		// Search text already dropped every comment when it was indexed.
		r.Snippet = parser.StripVaultMindMarkers(r.Snippet)
		// Normalize score: SQLite BM25 rank is negative (lower = better).
		// Negate to make higher = better, matching SRS-09 convention.
		if r.Score < 0 {
			r.Score = -r.Score
		}
		results = append(results, r)
	}

	// Min-max normalize scores to [0, 1] range.
	if len(results) > 0 {
		minScore := results[0].Score
		maxScore := results[0].Score
		for _, r := range results[1:] {
			if r.Score < minScore {
				minScore = r.Score
			}
			if r.Score > maxScore {
				maxScore = r.Score
			}
		}
		spread := maxScore - minScore
		if spread > 0 {
			for i := range results {
				results[i].Score = (results[i].Score - minScore) / spread
			}
		} else {
			for i := range results {
				results[i].Score = 1.0
			}
		}
	}

	return results, rows.Err()
}

// CountFTS returns the total number of documents matching the query and filters,
// independent of any limit/offset. Used for pagination totals.
func CountFTS(d *DB, query string, filters ...SearchFilters) (int, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return 0, nil
	}
	query = sanitizeFTSQuery(query)

	var f SearchFilters
	if len(filters) > 0 {
		f = filters[0]
	}

	q := `SELECT COUNT(*) FROM fts_notes f
		  JOIN notes n ON n.id = f.note_id
		  WHERE fts_notes MATCH ?`
	args := []interface{}{query}
	q, args = appendSearchFilters(q, args, f)

	var count int
	if err := d.QueryRow(q, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("FTS count: %w", err)
	}
	return count, nil
}

// gapExcludeSQL is the type half of the one search predicate. The literal is
// TypeGap. Dropping this clause is what lets a gap rank. A draft is the
// other half of the same predicate, and only while the caller has not opted in.
const gapExcludeSQL = "IFNULL(n.type, '') = 'gap' OR "

// appendSearchFilters is the one SQL predicate for type, tag, path, drafts
// and gaps. SearchFTS and CountFTS both call it, so a page and its total
// cannot disagree, and the non-knowledge clause is applied before LIMIT.
// A gap is never ranked. A draft is ranked only when the caller opted in.
func appendSearchFilters(q string, args []interface{}, f SearchFilters) (string, []interface{}) {
	if f.Type != "" {
		q += " AND n.type = ?"
		args = append(args, f.Type)
	}
	if f.Tag != "" {
		q += " AND n.id IN (SELECT note_id FROM tags WHERE tag = ?)"
		args = append(args, f.Tag)
	}
	if f.PathPrefix != "" {
		q += ` AND n.path LIKE ? ESCAPE '\'`
		args = append(args, PathPrefixLike(f.PathPrefix))
	}
	// NULL is not equal to 'draft' or 'gap'. IFNULL keeps a note with a
	// missing status or type searchable. include = 0 drops drafts.
	include := 0
	if f.IncludeDrafts {
		include = 1
	}
	q += " AND NOT (" + gapExcludeSQL + "(IFNULL(n.status, '') = ? AND ? = 0))"
	args = append(args, StatusDraft, include)
	return q, args
}

// sanitizeFTSQuery wraps each word in double quotes for literal FTS5 matching.
// This prevents special characters from being interpreted as FTS5 operators.
func sanitizeFTSQuery(query string) string {
	words := strings.Fields(query)
	var quoted []string
	for _, w := range words {
		// Strip existing quotes to avoid double-quoting
		w = strings.ReplaceAll(w, `"`, "")
		if w == "" {
			continue
		}
		quoted = append(quoted, `"`+w+`"`)
	}
	return strings.Join(quoted, " OR ")
}
