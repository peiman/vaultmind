package index

import (
	"encoding/json"
	"fmt"
	"time"
)

// noteStatusClosed is the notes.status of a gap that has been answered.
// frontmatter status is the same word; the column is what recall already
// filters on, so the credit map uses it too (Principle 7).
const noteStatusClosed = "closed"

// GapCredit is the question a note answered, and when that question was
// open. The map key is the knowledge note id (closed_by), not the gap id.
type GapCredit struct {
	Question string    `json:"question"`
	OpenedAt time.Time `json:"opened_at"`
	ClosedAt time.Time `json:"closed_at"`
}

// GapClosures loads, in one query, the notes that closed a gap.
// A note is gap-closing when a closed gap stores closed_by as that note id.
// The values live in frontmatter_kv, JSON-encoded, the same place
// gap.Measure reads them. Files are not parsed here.
//
// Two closed gaps naming the same note collapse to one entry: the earlier
// opened_at wins, and an equal opened_at keeps the lower gap id (the query
// is ordered by gap id). A row that cannot be parsed is skipped.
// A nil db yields an empty map.
func GapClosures(db RowQuery) (map[string]GapCredit, error) {
	out := map[string]GapCredit{}
	if db == nil {
		return out, nil
	}
	rows, err := db.Query(`
		SELECT COALESCE(b.value_json, ''),
			COALESCE(q.value_json, ''),
			COALESCE(o.value_json, ''),
			COALESCE(c.value_json, '')
		FROM notes n
		JOIN frontmatter_kv b ON b.note_id = n.id AND b.key = 'closed_by'
		LEFT JOIN frontmatter_kv q ON q.note_id = n.id AND q.key = 'question'
		LEFT JOIN frontmatter_kv o ON o.note_id = n.id AND o.key = 'opened_at'
		LEFT JOIN frontmatter_kv c ON c.note_id = n.id AND c.key = 'closed_at'
		WHERE n.type = ? AND n.status = ?
		ORDER BY n.id`, TypeGap, noteStatusClosed)
	if err != nil {
		return nil, fmt.Errorf("loading gap credits: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var closedBy, question, opened, closed string
		if err := rows.Scan(&closedBy, &question, &opened, &closed); err != nil {
			return nil, fmt.Errorf("loading gap credits: %w", err)
		}
		id, credit, ok := parseGapCredit(closedBy, question, opened, closed)
		if !ok {
			continue
		}
		keepEarlierCredit(out, id, credit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("loading gap credits: %w", err)
	}
	return out, nil
}

func parseGapCredit(closedBy, question, opened, closed string) (string, GapCredit, bool) {
	id := fmJSONString(closedBy)
	q := fmJSONString(question)
	openedAt := fmTime(opened)
	closedAt := fmTime(closed)
	if id == "" || q == "" || openedAt.IsZero() || closedAt.IsZero() {
		return "", GapCredit{}, false
	}
	return id, GapCredit{Question: q, OpenedAt: openedAt, ClosedAt: closedAt}, true
}

// keepEarlierCredit stores credit unless id already has an earlier question.
// Equal opened_at keeps the first writer. Callers pass rows in gap-id order.
func keepEarlierCredit(out map[string]GapCredit, id string, credit GapCredit) {
	existing, ok := out[id]
	if ok && !credit.OpenedAt.Before(existing.OpenedAt) {
		return
	}
	out[id] = credit
}

func fmJSONString(raw string) string {
	if raw == "" {
		return ""
	}
	var stored string
	if json.Unmarshal([]byte(raw), &stored) != nil {
		return ""
	}
	return stored
}

func fmTime(raw string) time.Time {
	stored := fmJSONString(raw)
	if stored == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, stored)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
