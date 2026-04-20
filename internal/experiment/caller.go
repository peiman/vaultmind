package experiment

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// UserSessionThreshold defines how long after the last matching invocation a
// new one still counts as part of the same user-session. 30 minutes is the
// default; invocations beyond this gap mint a new user-session id. "Matching"
// means same caller + same user + same host — workhorse persona loads and
// cli queries stay in separate groupings even when close in time.
const UserSessionThreshold = 30 * time.Minute

// SessionCaller describes the invoking agent for a session. Caller is a short
// label ("workhorse-persona-hook", "claude-code", "cli", or empty if the
// session pre-dates attribution). Meta is a flexible JSON blob — project_dir,
// pid, env-var snapshot — parsed back into a map so callers can read it
// without knowing the shape. UserSessionID groups multiple invocations into
// one working session via the UserSessionThreshold time heuristic.
type SessionCaller struct {
	Caller        string
	Meta          map[string]any
	UserSessionID string
}

// StartSessionWithCaller starts a session and records caller attribution.
// meta is serialized as JSON; nil meta is stored as NULL. A user_session_id
// is computed from (caller, user, host) against recent sessions — reused
// when the last matching session is within UserSessionThreshold, otherwise a
// new id is minted.
func (d *DB) StartSessionWithCaller(vaultPath, caller string, meta map[string]any) (string, error) {
	id := newUUID()
	now := time.Now().UTC()
	startedAt := now.Format(time.RFC3339)

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

	userSessionID := d.resolveUserSessionID(caller, meta, now)

	_, err := d.db.Exec(
		`INSERT INTO sessions (session_id, vault_path, started_at, caller, caller_meta, user_session_id)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, vaultPath, startedAt, callerStr, metaStr, userSessionID,
	)
	if err != nil {
		return "", fmt.Errorf("inserting session with caller: %w", err)
	}
	return id, nil
}

// resolveUserSessionID finds the most recent session that matches
// (caller, user, host). If its started_at is within UserSessionThreshold of
// now, its user_session_id is reused. Otherwise a new UUID is minted. Empty
// caller or missing user/host still produce a grouping, but one that only
// matches other similarly-attributed sessions.
func (d *DB) resolveUserSessionID(caller string, meta map[string]any, now time.Time) string {
	user, _ := meta["user"].(string)
	host, _ := meta["host"].(string)

	var priorID sql.NullString
	var priorStarted sql.NullString
	err := d.db.QueryRow(`
		SELECT user_session_id, started_at FROM sessions
		WHERE COALESCE(caller, '') = ?
		  AND COALESCE(json_extract(caller_meta, '$.user'), '') = ?
		  AND COALESCE(json_extract(caller_meta, '$.host'), '') = ?
		  AND user_session_id IS NOT NULL
		ORDER BY started_at DESC
		LIMIT 1
	`, caller, user, host).Scan(&priorID, &priorStarted)

	if err == nil && priorID.Valid && priorStarted.Valid {
		if t, parseErr := time.Parse(time.RFC3339, priorStarted.String); parseErr == nil {
			if now.Sub(t) <= UserSessionThreshold {
				return priorID.String
			}
		}
	}
	return newUUID()
}

// SessionsByUserSession returns all session_ids (invocation-sessions) that
// belong to the given user_session_id. Useful for reconstructing "everything
// that happened in this working session" across multiple invocations.
func (d *DB) SessionsByUserSession(userSessionID string) ([]string, error) {
	rows, err := d.db.Query(
		`SELECT session_id FROM sessions WHERE user_session_id = ? ORDER BY started_at ASC`,
		userSessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying sessions by user-session: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning session id: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// GetSessionCaller returns the caller attribution for the given session.
// Returns zero values (empty caller, nil meta, empty UserSessionID) for
// sessions that predate attribution or were started via the older
// StartSession.
func (d *DB) GetSessionCaller(sessionID string) (SessionCaller, error) {
	var caller, metaStr, userSessionID sql.NullString
	err := d.db.QueryRow(
		`SELECT caller, caller_meta, user_session_id FROM sessions WHERE session_id = ?`,
		sessionID,
	).Scan(&caller, &metaStr, &userSessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionCaller{}, nil
		}
		return SessionCaller{}, fmt.Errorf("querying session caller: %w", err)
	}

	out := SessionCaller{Caller: caller.String, UserSessionID: userSessionID.String}
	if metaStr.Valid && metaStr.String != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(metaStr.String), &m); err == nil {
			out.Meta = m
		}
	}
	return out, nil
}
