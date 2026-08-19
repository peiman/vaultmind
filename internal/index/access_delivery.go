package index

import (
	"fmt"
	"time"
)

// RecordNoteAccessDelivered records an access and whether it DELIVERED note
// text, rather than leaving that to be inferred from the caller.
//
// Caller answers "who initiated this" and is still the right axis for
// deliberate-vs-ambient. It was only ever a stand-in for "what arrived", and
// that stand-in held solely because the hook and neighbour callers never
// delivered anything. Once they did, every consumer reading caller as delivery
// silently inverted — `vaultmind self` began hiding real reads, and hid more of
// them the better delivery worked.
//
// The bool comes from the same AskResult.DeliveredTo value the experiment
// ledger records, so both ledgers answer the delivery question from one source
// instead of two disagreeing heuristics.
func RecordNoteAccessDelivered(d *DB, noteID, caller string, bodyDelivered bool) error {
	return recordAccess(d, noteID, caller, &bodyDelivered)
}

// ListDeliveredNotes returns access stats counting only accesses that delivered
// note text — the honest basis for "what have I actually engaged with".
//
// Rows written before delivery was tracked carry NULL, which is NOT the same as
// false. For those the caller heuristic was correct at the time: an `agent`
// access really was a deliberate read, a `hook` access really did deliver
// nothing. So NULL falls back to that original meaning rather than being
// retroactively asserted in either direction — writing 0 would claim a
// measurement nobody took, and writing 1 would fabricate deliveries.
func ListDeliveredNotes(d *DB) ([]NoteAccessStats, error) {
	const q = `
		SELECT
			n.id,
			COUNT(e.rowid) AS access_count,
			MAX(e.accessed_at) AS last_accessed_at,
			COALESCE(n.title, ''),
			COALESCE(n.type, '')
		FROM notes n
		JOIN note_accesses e ON e.note_id = n.id
		WHERE e.body_delivered = 1
		   OR (e.body_delivered IS NULL AND e.caller = ?)
		GROUP BY n.id
		ORDER BY last_accessed_at DESC`

	rows, err := d.Query(q, CallerAgent)
	if err != nil {
		return nil, fmt.Errorf("listing delivered note accesses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []NoteAccessStats
	for rows.Next() {
		var s NoteAccessStats
		if err := rows.Scan(&s.NoteID, &s.AccessCount, &s.LastAccessedAt, &s.Title, &s.NoteType); err != nil {
			return nil, fmt.Errorf("scanning delivered note access: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// recordAccess is the shared write path. bodyDelivered is a pointer so "not
// recorded" (legacy callers) stays distinguishable from "recorded as false" —
// the distinction ListDeliveredNotes depends on to read history honestly.
func recordAccess(d *DB, noteID, caller string, bodyDelivered *bool) error {
	if noteID == "" {
		return fmt.Errorf("RecordNoteAccess: noteID is empty")
	}
	resolved := resolveCaller(caller)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// INSERT ... SELECT FROM notes so a missing id inserts zero rows rather than
	// tripping the foreign key — preserving the best-effort contract that a
	// tracking miss never fails the user's query.
	if _, err := d.Exec(
		`INSERT INTO note_accesses (note_id, caller, accessed_at, body_delivered)
		 SELECT id, ?, ?, ? FROM notes WHERE id = ?`,
		resolved, now, bodyDelivered, noteID,
	); err != nil {
		return fmt.Errorf("logging access event for %q (caller %q): %w", noteID, resolved, err)
	}

	if _, err := d.Exec(
		`UPDATE notes SET access_count = access_count + 1, last_accessed_at = ? WHERE id = ?`,
		now, noteID,
	); err != nil {
		return fmt.Errorf("updating access scalars for %q: %w", noteID, err)
	}
	return nil
}
