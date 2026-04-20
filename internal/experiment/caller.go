package experiment

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SessionCaller describes the invoking agent for a session. Caller is a short
// label ("workhorse-persona-hook", "claude-code", "cli", or empty if the
// session pre-dates attribution). Meta is a flexible JSON blob — project_dir,
// pid, env-var snapshot — parsed back into a map so callers can read it
// without knowing the shape.
type SessionCaller struct {
	Caller string
	Meta   map[string]any
}

// StartSessionWithCaller starts a session and records caller attribution.
// meta is serialized as JSON; nil meta is stored as NULL.
func (d *DB) StartSessionWithCaller(vaultPath, caller string, meta map[string]any) (string, error) {
	id := newUUID()
	startedAt := time.Now().UTC().Format(time.RFC3339)

	var metaStr sql.NullString
	if len(meta) > 0 {
		b, err := json.Marshal(meta)
		if err != nil {
			return "", fmt.Errorf("marshaling caller meta: %w", err)
		}
		metaStr = sql.NullString{String: string(b), Valid: true}
	}

	var callerStr sql.NullString
	if caller != "" {
		callerStr = sql.NullString{String: caller, Valid: true}
	}

	_, err := d.db.Exec(
		`INSERT INTO sessions (session_id, vault_path, started_at, caller, caller_meta)
		 VALUES (?, ?, ?, ?, ?)`,
		id, vaultPath, startedAt, callerStr, metaStr,
	)
	if err != nil {
		return "", fmt.Errorf("inserting session with caller: %w", err)
	}
	return id, nil
}

// GetSessionCaller returns the caller attribution for the given session.
// Returns zero values (empty caller, nil meta) for sessions that predate
// attribution or were started via the older StartSession.
func (d *DB) GetSessionCaller(sessionID string) (SessionCaller, error) {
	var caller, metaStr sql.NullString
	err := d.db.QueryRow(
		`SELECT caller, caller_meta FROM sessions WHERE session_id = ?`,
		sessionID,
	).Scan(&caller, &metaStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionCaller{}, nil
		}
		return SessionCaller{}, fmt.Errorf("querying session caller: %w", err)
	}

	out := SessionCaller{Caller: caller.String}
	if metaStr.Valid && metaStr.String != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(metaStr.String), &m); err == nil {
			out.Meta = m
		}
	}
	return out, nil
}
