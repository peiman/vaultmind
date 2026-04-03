package index

import (
	"fmt"
	"strings"
)

// FTSResult represents a single full-text search hit.
type FTSResult struct {
	NoteID  string  `json:"note_id"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Rank    float64 `json:"rank"`
}

// SearchFTS performs a full-text search against the fts_notes table.
// Returns results ordered by relevance (rank), limited and offset as specified.
func SearchFTS(d *DB, query string, limit, offset int) ([]FTSResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	rows, err := d.Query(`
		SELECT f.note_id, n.title, snippet(fts_notes, 2, '...', '...', '', 32), rank
		FROM fts_notes f
		JOIN notes n ON n.id = f.note_id
		WHERE fts_notes MATCH ?
		ORDER BY rank
		LIMIT ? OFFSET ?`,
		query, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("FTS search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []FTSResult
	for rows.Next() {
		var r FTSResult
		var title *string
		if scanErr := rows.Scan(&r.NoteID, &title, &r.Snippet, &r.Rank); scanErr != nil {
			return nil, fmt.Errorf("scanning FTS result: %w", scanErr)
		}
		if title != nil {
			r.Title = *title
		}
		results = append(results, r)
	}

	return results, rows.Err()
}
