package experiment

import (
	"encoding/json"
	"fmt"
	"time"
)

// NoteAccessTimes returns timestamps of all note_access events for noteID,
// ordered ascending. Filters by parsing event_data JSON for matching note_id.
func (d *DB) NoteAccessTimes(noteID string) ([]time.Time, error) {
	rows, err := d.db.Query(
		`SELECT timestamp, event_data FROM events
		 WHERE event_type = ? ORDER BY timestamp ASC`,
		EventNoteAccess,
	)
	if err != nil {
		return nil, fmt.Errorf("querying note access events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var times []time.Time
	for rows.Next() {
		var ts, dataJSON string
		if err := rows.Scan(&ts, &dataJSON); err != nil {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
			continue
		}
		if data["note_id"] != noteID {
			continue
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		times = append(times, t)
	}
	return times, rows.Err()
}

// RecentSessionWindows returns the N most recent completed sessions as
// SessionWindows. Orphans (NULL ended_at) are excluded.
func (d *DB) RecentSessionWindows(limit int) ([]SessionWindow, error) {
	rows, err := d.db.Query(
		`SELECT started_at, ended_at FROM sessions
		 WHERE ended_at IS NOT NULL
		 ORDER BY started_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying session windows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var windows []SessionWindow
	for rows.Next() {
		var startStr, endStr string
		if err := rows.Scan(&startStr, &endStr); err != nil {
			continue
		}
		start, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			continue
		}
		end, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			continue
		}
		windows = append(windows, SessionWindow{Start: start, End: end})
	}
	return windows, rows.Err()
}

// BatchNoteAccessTimes returns access times for multiple notes in one pass.
func (d *DB) BatchNoteAccessTimes(noteIDs []string) (map[string][]time.Time, error) {
	result := make(map[string][]time.Time, len(noteIDs))
	for _, id := range noteIDs {
		result[id] = nil
	}
	if len(noteIDs) == 0 {
		return result, nil
	}

	wanted := make(map[string]bool, len(noteIDs))
	for _, id := range noteIDs {
		wanted[id] = true
	}

	rows, err := d.db.Query(
		`SELECT timestamp, event_data FROM events
		 WHERE event_type = ? ORDER BY timestamp ASC`,
		EventNoteAccess,
	)
	if err != nil {
		return nil, fmt.Errorf("querying batch note access: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var ts, dataJSON string
		if err := rows.Scan(&ts, &dataJSON); err != nil {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
			continue
		}
		noteID, ok := data["note_id"].(string)
		if !ok || !wanted[noteID] {
			continue
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		result[noteID] = append(result[noteID], t)
	}
	return result, rows.Err()
}

// AccessedNoteIDs returns all unique note IDs from note_access events.
func (d *DB) AccessedNoteIDs() ([]string, error) {
	rows, err := d.db.Query(
		`SELECT event_data FROM events WHERE event_type = ?`,
		EventNoteAccess,
	)
	if err != nil {
		return nil, fmt.Errorf("querying accessed note IDs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	seen := make(map[string]bool)
	var ids []string
	for rows.Next() {
		var dataJSON string
		if err := rows.Scan(&dataJSON); err != nil {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
			continue
		}
		noteID, ok := data["note_id"].(string)
		if !ok || seen[noteID] {
			continue
		}
		seen[noteID] = true
		ids = append(ids, noteID)
	}
	return ids, rows.Err()
}
