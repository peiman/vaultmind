package experiment

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// SanitizeEventData returns a sanitized copy of event_data for the given
// telemetry tier. Under TelemetryAnonymous, the contract documented in
// telemetry.go is enforced: variants[*].results[*].note_id and .path are
// stripped; aggregate fields (rank, score, scores, type, …) survive.
// Under TelemetryFull the data is returned as-is.
//
// SanitizeEventData does NOT mutate its input. It walks the structure
// and rebuilds the parts it touches, so callers can reuse the original
// map without surprise.
func SanitizeEventData(data map[string]any, tier string) map[string]any {
	if tier == TelemetryFull {
		return data
	}
	out := make(map[string]any, len(data))
	for k, v := range data {
		if k != "variants" {
			out[k] = v
			continue
		}
		variants, ok := v.(map[string]any)
		if !ok {
			out[k] = v
			continue
		}
		out["variants"] = sanitizeVariants(variants)
	}
	return out
}

func sanitizeVariants(variants map[string]any) map[string]any {
	out := make(map[string]any, len(variants))
	for name, v := range variants {
		variant, ok := v.(map[string]any)
		if !ok {
			out[name] = v
			continue
		}
		variantOut := make(map[string]any, len(variant))
		for k, vv := range variant {
			if k == "results" {
				if results, ok := vv.([]any); ok {
					variantOut[k] = sanitizeResults(results)
					continue
				}
			}
			variantOut[k] = vv
		}
		out[name] = variantOut
	}
	return out
}

// sanitizeResults strips note_id and path from each result row but
// preserves all other fields (rank, score, type, etc).
func sanitizeResults(results []any) []any {
	out := make([]any, len(results))
	for i, r := range results {
		row, ok := r.(map[string]any)
		if !ok {
			out[i] = r
			continue
		}
		clean := make(map[string]any, len(row))
		for k, v := range row {
			if k == "note_id" || k == "path" {
				continue
			}
			clean[k] = v
		}
		out[i] = clean
	}
	return out
}

// ExportToJSONL writes a sanitized snapshot of the experiment DB to w
// as a JSONL stream: one manifest record on line 1, then one record
// per session, event, and outcome. Each record is a JSON object with a
// "kind" discriminator ("manifest" / "session" / "event" / "outcome").
//
// Tier handling:
//   - TelemetryOff   → returns an error and writes nothing. The user
//     opted out; producing a file implies we collected data we
//     shouldn't have.
//   - TelemetryAnonymous → strips vault_path on sessions and events,
//     query_text on events, caller_meta on sessions, and the documented
//     fields inside event_data via SanitizeEventData.
//   - TelemetryFull → preserves everything.
func ExportToJSONL(db *DB, tier string, w io.Writer) error {
	if tier == TelemetryOff {
		return fmt.Errorf("export refused: telemetry tier is %q (the user opted out)", TelemetryOff)
	}
	if tier != TelemetryAnonymous && tier != TelemetryFull {
		return fmt.Errorf("export: unknown tier %q (want %q | %q | %q)",
			tier, TelemetryOff, TelemetryAnonymous, TelemetryFull)
	}

	sessions, err := readSessions(db)
	if err != nil {
		return fmt.Errorf("read sessions: %w", err)
	}
	events, err := readEvents(db)
	if err != nil {
		return fmt.Errorf("read events: %w", err)
	}
	outcomes, err := readOutcomes(db)
	if err != nil {
		return fmt.Errorf("read outcomes: %w", err)
	}

	enc := json.NewEncoder(w)
	manifest := map[string]any{
		"kind":           "manifest",
		"tier":           tier,
		"exported_at":    time.Now().UTC().Format(time.RFC3339),
		"session_count":  len(sessions),
		"event_count":    len(events),
		"outcome_count":  len(outcomes),
		"schema_version": exportSchemaVersion,
	}
	if err := enc.Encode(manifest); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	for _, s := range sessions {
		if err := enc.Encode(sessionRecord(s, tier)); err != nil {
			return fmt.Errorf("write session: %w", err)
		}
	}
	for _, e := range events {
		if err := enc.Encode(eventRecord(e, tier)); err != nil {
			return fmt.Errorf("write event: %w", err)
		}
	}
	for _, o := range outcomes {
		if err := enc.Encode(outcomeRecord(o, tier)); err != nil {
			return fmt.Errorf("write outcome: %w", err)
		}
	}
	return nil
}

// exportSchemaVersion bumps when the JSONL record shape changes in a
// non-additive way. Receivers can use this to dispatch on shape.
const exportSchemaVersion = 1

type sessionRow struct {
	SessionID     string
	VaultPath     string
	StartedAt     string
	EndedAt       *string
	Caller        *string
	CallerMeta    *string
	UserSessionID *string
}

type eventRow struct {
	EventID        string
	SessionID      string
	EventType      string
	Timestamp      string
	VaultPath      string
	QueryText      *string
	QueryMode      *string
	PrimaryVariant *string
	EventData      string
}

type outcomeRow struct {
	OutcomeID  string
	EventID    string
	NoteID     string
	Variant    string
	Rank       int
	AccessedAt string
	SessionID  string
}

func readSessions(db *DB) ([]sessionRow, error) {
	rows, err := db.db.Query(`SELECT session_id, vault_path, started_at, ended_at,
	                                  caller, caller_meta, user_session_id
	                          FROM sessions ORDER BY started_at`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []sessionRow
	for rows.Next() {
		var s sessionRow
		if err := rows.Scan(&s.SessionID, &s.VaultPath, &s.StartedAt, &s.EndedAt,
			&s.Caller, &s.CallerMeta, &s.UserSessionID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func readEvents(db *DB) ([]eventRow, error) {
	rows, err := db.db.Query(`SELECT event_id, session_id, event_type, timestamp, vault_path,
	                                  query_text, query_mode, primary_variant, event_data
	                          FROM events ORDER BY timestamp`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []eventRow
	for rows.Next() {
		var e eventRow
		if err := rows.Scan(&e.EventID, &e.SessionID, &e.EventType, &e.Timestamp, &e.VaultPath,
			&e.QueryText, &e.QueryMode, &e.PrimaryVariant, &e.EventData); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func readOutcomes(db *DB) ([]outcomeRow, error) {
	rows, err := db.db.Query(`SELECT outcome_id, event_id, note_id, variant, rank,
	                                  accessed_at, session_id
	                          FROM outcomes ORDER BY accessed_at`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []outcomeRow
	for rows.Next() {
		var o outcomeRow
		if err := rows.Scan(&o.OutcomeID, &o.EventID, &o.NoteID, &o.Variant, &o.Rank,
			&o.AccessedAt, &o.SessionID); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func sessionRecord(s sessionRow, tier string) map[string]any {
	rec := map[string]any{
		"kind":       "session",
		"session_id": s.SessionID,
		"started_at": s.StartedAt,
	}
	if s.EndedAt != nil {
		rec["ended_at"] = *s.EndedAt
	}
	if s.Caller != nil {
		rec["caller"] = *s.Caller
	}
	if s.UserSessionID != nil {
		rec["user_session_id"] = *s.UserSessionID
	}
	if tier == TelemetryFull {
		rec["vault_path"] = s.VaultPath
		if s.CallerMeta != nil {
			rec["caller_meta"] = *s.CallerMeta
		}
	}
	return rec
}

func eventRecord(e eventRow, tier string) map[string]any {
	rec := map[string]any{
		"kind":       "event",
		"event_id":   e.EventID,
		"session_id": e.SessionID,
		"event_type": e.EventType,
		"timestamp":  e.Timestamp,
	}
	if e.QueryMode != nil {
		rec["query_mode"] = *e.QueryMode
	}
	if e.PrimaryVariant != nil {
		rec["primary_variant"] = *e.PrimaryVariant
	}
	if tier == TelemetryFull {
		rec["vault_path"] = e.VaultPath
		if e.QueryText != nil {
			rec["query_text"] = *e.QueryText
		}
	}
	// event_data is JSON-encoded text; decode → sanitize → re-encode as a
	// structured object so the receiver doesn't have to double-decode.
	if e.EventData != "" {
		var data map[string]any
		if err := json.Unmarshal([]byte(e.EventData), &data); err == nil {
			rec["data"] = SanitizeEventData(data, tier)
		}
	}
	return rec
}

func outcomeRecord(o outcomeRow, tier string) map[string]any {
	rec := map[string]any{
		"kind":        "outcome",
		"outcome_id":  o.OutcomeID,
		"event_id":    o.EventID,
		"variant":     o.Variant,
		"rank":        o.Rank,
		"accessed_at": o.AccessedAt,
		"session_id":  o.SessionID,
	}
	if tier == TelemetryFull {
		rec["note_id"] = o.NoteID
	}
	return rec
}
