package experiment

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// linkableEvent holds a parsed event row that is a candidate for outcome linkage.
type linkableEvent struct {
	eventID string
	data    map[string]any
}

// variantMatch records a variant name and the rank at which a note appeared.
type variantMatch struct {
	variant string
	rank    int
}

// LinkOutcomes looks back at recent search/ask/context_pack events (within the
// outcome window) and creates outcome rows for any variant where noteID appears
// in the results. Returns the number of outcome rows created.
func (d *DB) LinkOutcomes(currentSessionID, noteID string, outcomeWindow int) (int, error) {
	sessionIDs, err := d.recentSessionIDs(currentSessionID, outcomeWindow)
	if err != nil {
		return 0, fmt.Errorf("finding recent sessions: %w", err)
	}

	events, err := d.linkableEvents(sessionIDs)
	if err != nil {
		return 0, fmt.Errorf("finding linkable events: %w", err)
	}

	accessedAt := time.Now().UTC().Format(time.RFC3339)
	count := 0
	for _, evt := range events {
		matches := findNoteInVariants(evt.data, noteID)
		for _, m := range matches {
			outcomeID := newUUID()
			_, err := d.db.Exec(
				`INSERT INTO outcomes
				   (outcome_id, event_id, note_id, variant, rank, accessed_at, session_id)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				outcomeID, evt.eventID, noteID, m.variant, m.rank, accessedAt, currentSessionID,
			)
			if err != nil {
				return count, fmt.Errorf("inserting outcome for event %s variant %s: %w",
					evt.eventID, m.variant, err)
			}
			count++
		}
	}
	return count, nil
}

// recentSessionIDs returns the current session plus up to window-1 prior
// sessions, ordered by started_at descending. The current session is always
// included regardless of ordering.
func (d *DB) recentSessionIDs(currentSessionID string, window int) ([]string, error) {
	ids := []string{currentSessionID}
	if window <= 1 {
		return ids, nil
	}

	rows, err := d.db.Query(
		`SELECT session_id FROM sessions
		 WHERE session_id != ?
		 ORDER BY started_at DESC
		 LIMIT ?`,
		currentSessionID, window-1,
	)
	if err != nil {
		return nil, fmt.Errorf("querying recent sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning session id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating session rows: %w", err)
	}
	return ids, nil
}

// linkableEvents returns all search/ask/context_pack events from the given
// sessions with their parsed event_data JSON.
func (d *DB) linkableEvents(sessionIDs []string) ([]linkableEvent, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}

	// Build placeholder list: (?, ?, ...) using strings.Builder to avoid
	// gosec G201 (SQL string formatting via fmt.Sprintf).
	var sb strings.Builder
	for i := range sessionIDs {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("?")
	}

	args := make([]any, len(sessionIDs))
	for i, id := range sessionIDs {
		args[i] = id
	}
	args = append(args, EventSearch, EventAsk, EventContextPack)

	// #nosec G202 -- sb.String() contains only "?" placeholder characters, never user input
	query := `SELECT event_id, event_data FROM events WHERE session_id IN (` +
		sb.String() +
		`) AND event_type IN (?, ?, ?)`

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying linkable events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []linkableEvent
	for rows.Next() {
		var eventID, dataStr string
		if err := rows.Scan(&eventID, &dataStr); err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
			return nil, fmt.Errorf("parsing event_data for event %s: %w", eventID, err)
		}
		events = append(events, linkableEvent{eventID: eventID, data: data})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating event rows: %w", err)
	}
	return events, nil
}

// findNoteInVariants searches data["variants"][name]["results"] for a matching
// note_id and returns one variantMatch per variant where the note appears.
func findNoteInVariants(data map[string]any, noteID string) []variantMatch {
	variantsRaw, ok := data["variants"]
	if !ok {
		return nil
	}
	variants, ok := variantsRaw.(map[string]any)
	if !ok {
		return nil
	}

	var matches []variantMatch
	for variantName, variantRaw := range variants {
		variantMap, ok := variantRaw.(map[string]any)
		if !ok {
			continue
		}
		resultsRaw, ok := variantMap["results"]
		if !ok {
			continue
		}
		results, ok := resultsRaw.([]any)
		if !ok {
			continue
		}
		for _, resultRaw := range results {
			result, ok := resultRaw.(map[string]any)
			if !ok {
				continue
			}
			id, ok := result["note_id"].(string)
			if !ok || id != noteID {
				continue
			}
			rank := 0
			if rankRaw, ok := result["rank"]; ok {
				switch r := rankRaw.(type) {
				case float64:
					rank = int(r)
				case int:
					rank = r
				}
			}
			matches = append(matches, variantMatch{variant: variantName, rank: rank})
		}
	}
	return matches
}
