package experiment

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"
)

// newUUID generates a random UUID v4 using crypto/rand.
// Format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	// Set version bits (v4)
	b[6] = (b[6] & 0x0f) | 0x40
	// Set variant bits
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// StartSession inserts a new session row and returns the generated session ID.
func (d *DB) StartSession(vaultPath string) (string, error) {
	id := newUUID()
	startedAt := time.Now().UTC().Format(time.RFC3339)
	_, err := d.db.Exec(
		`INSERT INTO sessions (session_id, vault_path, started_at) VALUES (?, ?, ?)`,
		id, vaultPath, startedAt,
	)
	if err != nil {
		return "", fmt.Errorf("inserting session: %w", err)
	}
	return id, nil
}

// EndSession sets ended_at for the given session to the current time.
func (d *DB) EndSession(sessionID string) error {
	endedAt := time.Now().UTC().Format(time.RFC3339)
	_, err := d.db.Exec(
		`UPDATE sessions SET ended_at = ? WHERE session_id = ?`,
		endedAt, sessionID,
	)
	if err != nil {
		return fmt.Errorf("ending session: %w", err)
	}
	return nil
}

// RecoverOrphans finds sessions with NULL ended_at and sets ended_at to the
// timestamp of their last event, or started_at + 1 minute if no events exist.
// Returns the number of sessions recovered.
func (d *DB) RecoverOrphans() (int, error) {
	rows, err := d.db.Query(
		`SELECT session_id, started_at FROM sessions WHERE ended_at IS NULL`,
	)
	if err != nil {
		return 0, fmt.Errorf("querying orphan sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type orphan struct {
		id        string
		startedAt string
	}
	var orphans []orphan
	for rows.Next() {
		var o orphan
		if err := rows.Scan(&o.id, &o.startedAt); err != nil {
			return 0, fmt.Errorf("scanning orphan row: %w", err)
		}
		orphans = append(orphans, o)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating orphan rows: %w", err)
	}

	count := 0
	for _, o := range orphans {
		// Find the last event timestamp for this session.
		var lastEventTime sql.NullString
		err := d.db.QueryRow(
			`SELECT MAX(timestamp) FROM events WHERE session_id = ?`, o.id,
		).Scan(&lastEventTime)
		if err != nil {
			return count, fmt.Errorf("querying last event for session %s: %w", o.id, err)
		}

		var endedAt string
		if !lastEventTime.Valid {
			// No events: use started_at + 1 minute.
			started, parseErr := time.Parse(time.RFC3339, o.startedAt)
			if parseErr != nil {
				return count, fmt.Errorf("parsing started_at %q: %w", o.startedAt, parseErr)
			}
			endedAt = started.Add(time.Minute).Format(time.RFC3339)
		} else {
			endedAt = lastEventTime.String
		}

		if _, err := d.db.Exec(
			`UPDATE sessions SET ended_at = ? WHERE session_id = ?`,
			endedAt, o.id,
		); err != nil {
			return count, fmt.Errorf("recovering orphan session %s: %w", o.id, err)
		}
		count++
	}
	return count, nil
}
