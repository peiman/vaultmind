package index

import (
	"fmt"
	"time"
)

// RecordNoteAccess increments the note's access_count by 1 and updates
// last_accessed_at to the current UTC timestamp. First slice of plasticity
// roadmap step 5 (decay + reinforcement): the storage strength side of the
// ACT-R base-level activation equation needs an actual count of past
// retrievals. Migration 004 added the columns; this is the writer that
// makes them mean something.
//
// Called from query.Ask when a context-pack is successfully built around
// a target note — that's the "agent built its working context around this
// note's body" signal, the strongest non-explicit retrieval-access marker
// we have. Future slices will: extend to context-pack neighbors (medium
// signal), wire the count into RRF score blending so reinforcement
// actually shapes ranking, add time-based decay using last_accessed_at.
//
// Best-effort: if the row doesn't exist (note id mismatch, stale cache)
// or the UPDATE fails, RecordNoteAccess returns the error but the caller
// is expected to log-and-continue rather than fail the user-facing query
// over a tracking miss. The whole function is principle-9-shaped: every
// retrieval that lands a note becomes a recorded activation event by
// design, not by the agent remembering to log it.
func RecordNoteAccess(d *DB, noteID string) error {
	if noteID == "" {
		return fmt.Errorf("RecordNoteAccess: noteID is empty")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := d.Exec(
		`UPDATE notes SET access_count = access_count + 1, last_accessed_at = ? WHERE id = ?`,
		now, noteID,
	)
	if err != nil {
		return fmt.Errorf("recording access for %q: %w", noteID, err)
	}
	return nil
}

// NoteAccessStats reports the access counters for a single note. Useful
// for doctor / debugging / verifying that RecordNoteAccess is firing on
// the paths it's supposed to. Title and NoteType are populated by
// ListAccessedNotes so the self-rendering layer can produce
// human-readable output without a separate join. LookupNoteAccess
// leaves them empty (single-id callers don't need them).
type NoteAccessStats struct {
	NoteID         string
	AccessCount    int
	LastAccessedAt string // RFC3339Nano UTC, empty when never accessed
	Title          string
	NoteType       string
}

// ListAccessedNotes returns access stats for every note with
// access_count > 0, sorted newest-first by last_accessed_at. Backs
// `vaultmind self`'s "recent" view and the activation-driven hot/stale
// views; the renderer downstream sorts/filters by activation score
// without re-querying. Title and NoteType are joined in so the renderer
// can produce human-readable output without a follow-up lookup per row.
// Returns an empty slice when the vault is freshly indexed and nothing
// has been touched yet — that's a valid "blank-slate self" state.
func ListAccessedNotes(d *DB) ([]NoteAccessStats, error) {
	rows, err := d.Query(`
		SELECT id, access_count, last_accessed_at, COALESCE(title, ''), COALESCE(type, '')
		FROM notes
		WHERE access_count > 0
		ORDER BY last_accessed_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing accessed notes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []NoteAccessStats
	for rows.Next() {
		var s NoteAccessStats
		var lastAccessed *string
		if err := rows.Scan(&s.NoteID, &s.AccessCount, &lastAccessed, &s.Title, &s.NoteType); err != nil {
			return nil, fmt.Errorf("scanning accessed-notes row: %w", err)
		}
		if lastAccessed != nil {
			s.LastAccessedAt = *lastAccessed
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating accessed-notes rows: %w", err)
	}
	return out, nil
}

// LookupNoteAccess returns the access stats for a single note, or
// (zero-stats, nil) when the note doesn't exist (deliberately mirrors
// QueryFullNote's "not found" semantics — caller checks whether NoteID
// came back populated).
func LookupNoteAccess(d *DB, noteID string) (NoteAccessStats, error) {
	var stats NoteAccessStats
	var lastAccessed *string
	err := d.QueryRow(
		`SELECT id, access_count, last_accessed_at FROM notes WHERE id = ?`,
		noteID,
	).Scan(&stats.NoteID, &stats.AccessCount, &lastAccessed)
	if err != nil {
		// Treat sql.ErrNoRows as "not found" — return zero stats, no error
		if err.Error() == "sql: no rows in result set" {
			return NoteAccessStats{}, nil
		}
		return NoteAccessStats{}, fmt.Errorf("looking up access stats for %q: %w", noteID, err)
	}
	if lastAccessed != nil {
		stats.LastAccessedAt = *lastAccessed
	}
	return stats, nil
}
