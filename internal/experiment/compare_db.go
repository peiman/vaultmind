package experiment

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ComparableEventFilter narrows which events LoadComparableEvents returns.
// Zero-value fields mean "no filter." EventTypes empty → default to
// ask/search/context_pack.
type ComparableEventFilter struct {
	SessionID    string
	Caller       string
	SinceRFC3339 string
	EventTypes   []string
}

// LoadComparableEvents returns events that carry at least one shadow variant
// (i.e., a variants map with primary plus another) with their pairs
// pre-extracted. Rows whose primary_variant is NULL/empty are skipped.
func (d *DB) LoadComparableEvents(f ComparableEventFilter) ([]ComparableEvent, error) {
	types := f.EventTypes
	if len(types) == 0 {
		types = []string{"ask", "search", "context_pack"}
	}

	placeholders := make([]string, len(types))
	args := []any{}
	for i, t := range types {
		placeholders[i] = "?"
		args = append(args, t)
	}
	where := []string{
		"e.event_type IN (" + strings.Join(placeholders, ",") + ")",
		"e.primary_variant IS NOT NULL",
		"e.primary_variant <> ''",
	}
	if f.SessionID != "" {
		where = append(where, "e.session_id = ?")
		args = append(args, f.SessionID)
	}
	if f.Caller != "" {
		where = append(where, "s.caller = ?")
		args = append(args, f.Caller)
	}
	if f.SinceRFC3339 != "" {
		where = append(where, "e.timestamp >= ?")
		args = append(args, f.SinceRFC3339)
	}

	query := fmt.Sprintf(`
		SELECT e.event_id, e.primary_variant, e.event_data
		FROM events e
		LEFT JOIN sessions s ON s.session_id = e.session_id
		WHERE %s
		ORDER BY e.timestamp ASC`,
		strings.Join(where, " AND "),
	)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying comparable events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ComparableEvent
	for rows.Next() {
		var eventID, primary, dataStr string
		if err := rows.Scan(&eventID, &primary, &dataStr); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
			continue
		}
		pairs, err := ExtractEventPairs(data, primary)
		if err != nil || len(pairs) == 0 {
			continue
		}
		out = append(out, ComparableEvent{EventID: eventID, Pairs: pairs})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration: %w", err)
	}
	return out, nil
}
