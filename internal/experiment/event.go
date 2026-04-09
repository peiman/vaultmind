package experiment

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Event type constants.
const (
	EventSearch      = "search"
	EventAsk         = "ask"
	EventContextPack = "context_pack"
	EventNoteAccess  = "note_access"
	EventIndexEmbed  = "index_embed"
)

// Event holds the data for a single experiment event to be logged.
type Event struct {
	SessionID      string
	Type           string
	VaultPath      string
	QueryText      string
	QueryMode      string
	PrimaryVariant string
	Data           map[string]any
}

// LogEvent inserts an event row into the events table and returns the generated
// event ID. Optional string fields (QueryText, QueryMode, PrimaryVariant) are
// stored as NULL when empty.
func (d *DB) LogEvent(evt Event) (string, error) {
	id := newUUID()
	ts := time.Now().UTC().Format(time.RFC3339)

	dataJSON, err := json.Marshal(evt.Data)
	if err != nil {
		return "", fmt.Errorf("marshaling event data: %w", err)
	}

	queryText := nullableString(evt.QueryText)
	queryMode := nullableString(evt.QueryMode)
	primaryVariant := nullableString(evt.PrimaryVariant)

	_, err = d.db.Exec(
		`INSERT INTO events
		   (event_id, session_id, event_type, timestamp, vault_path,
		    query_text, query_mode, primary_variant, event_data)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, evt.SessionID, evt.Type, ts, evt.VaultPath,
		queryText, queryMode, primaryVariant, string(dataJSON),
	)
	if err != nil {
		return "", fmt.Errorf("inserting event: %w", err)
	}
	return id, nil
}

// nullableString returns a sql.NullString that is Valid only when s is non-empty.
func nullableString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
