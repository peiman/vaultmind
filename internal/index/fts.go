package index

import (
	"fmt"
	"strings"
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

// SearchFilters holds optional filters for FTS search.
type SearchFilters struct {
	Type string // Filter by note type (empty = no filter)
	Tag  string // Filter by tag (empty = no filter)
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
		snippet(fts_notes, -1, '...', '...', '', 32), rank, n.is_domain
		FROM fts_notes f
		JOIN notes n ON n.id = f.note_id
		WHERE fts_notes MATCH ?`
	args := []interface{}{query}

	if f.Type != "" {
		q += " AND n.type = ?"
		args = append(args, f.Type)
	}
	if f.Tag != "" {
		q += " AND n.id IN (SELECT note_id FROM tags WHERE tag = ?)"
		args = append(args, f.Tag)
	}

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
	return strings.Join(quoted, " ")
}
