package experiment

import (
	"database/sql"
	"fmt"
)

// SessionGap reports one session and its gap from the previous session.
// Sessions are ordered chronologically ascending. The first session has no
// predecessor — PrevSessionID, PrevStartedAt, and GapSeconds are NULL.
//
// Primary consumer: compressed-idle-time analysis (gamma parameter fitting).
type SessionGap struct {
	SessionID     string
	StartedAt     string
	PrevSessionID sql.NullString
	PrevStartedAt sql.NullString
	GapSeconds    sql.NullInt64
}

// SessionGaps returns all sessions with their inter-session gaps, ordered by
// started_at ascending. Derived from the session_gaps SQL view.
func (d *DB) SessionGaps() ([]SessionGap, error) {
	rows, err := d.db.Query(`
		SELECT session_id, started_at, prev_session_id, prev_started_at, gap_seconds
		FROM session_gaps
		ORDER BY started_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying session_gaps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SessionGap
	for rows.Next() {
		var g SessionGap
		if err := rows.Scan(&g.SessionID, &g.StartedAt, &g.PrevSessionID, &g.PrevStartedAt, &g.GapSeconds); err != nil {
			return nil, fmt.Errorf("scanning session_gaps row: %w", err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating session_gaps rows: %w", err)
	}
	return out, nil
}
