package index

import (
	"fmt"
	"os"
	"time"
)

// Caller* constants name the provenance of an access event. The string
// values land in the note_accesses.caller column and let `vaultmind self`
// filter "what I engaged with" from "what the harness pre-loaded."
//
// CallerAgent is the default for explicit agent reads (note get, the
// target of an Ask). CallerAgentNeighbor is set when an Ask's
// context-pack pulls a neighbor in alongside the target — still real
// engagement, but lower-intent than a direct read. CallerHook is set
// when a Claude Code hook (SessionStart persona load, UserPromptSubmit
// pointer fanout, etc.) fires the access; these accesses populate the
// activation log but `self` filters them out by default so its hot list
// reflects deliberate engagement rather than ambient harness traffic.
//
// Set via the VAULTMIND_CALLER env var or passed explicitly to
// RecordNoteAccessAs. RecordNoteAccess (no caller arg) reads the env
// and falls back to CallerAgent.
const (
	CallerAgent         = "agent"
	CallerAgentNeighbor = "agent-neighbor"
	CallerHook          = "hook"
)

// callerEnvVar is the env-var key the persona/recall hooks set so their
// fan-out accesses are distinguishable from the agent's deliberate
// queries. The persona script writes "vaultmind-persona-hook" today;
// any value that ends with "-hook" classifies as a hook caller.
const callerEnvVar = "VAULTMIND_CALLER"

// resolveCaller maps explicit-arg + env-var input to a normalised
// caller value. The env var WINS when it indicates a hook caller —
// otherwise an explicit caller passed by query.Ask (CallerAgent for
// target, CallerAgentNeighbor for neighbors) would silently mislabel
// hook-driven Asks as agent traffic, which is exactly the pollution
// this whole layer is designed to prevent.
//
// Precedence:
//  1. Env var matching "hook" substring → CallerHook (harness override)
//  2. Explicit caller arg if non-empty → returned verbatim
//  3. Env var (any other value) → returned verbatim
//  4. Otherwise → CallerAgent (default for explicit deliberate access)
//
// The "hook" substring rule keeps the persona script's existing
// VAULTMIND_CALLER=vaultmind-persona-hook value working unchanged.
func resolveCaller(explicit string) string {
	envCaller := os.Getenv(callerEnvVar)
	if envCaller != "" && containsHookSubstring(envCaller) {
		return CallerHook
	}
	if explicit != "" {
		return explicit
	}
	if envCaller != "" {
		return envCaller
	}
	return CallerAgent
}

// containsHookSubstring is a tiny helper kept inline so the resolver
// doesn't pull in strings.Contains for one call.
func containsHookSubstring(s string) bool {
	const needle = "hook"
	if len(s) < len(needle) {
		return false
	}
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// RecordNoteAccess records a note access with the default caller
// (read from VAULTMIND_CALLER env var, falling back to CallerAgent).
// Backwards compatible with pre-2026-05-01 callers: the call signature
// is unchanged, so existing call sites don't have to be rewritten.
//
// Use RecordNoteAccessAs when the caller is known structurally (e.g.
// query.Ask passes CallerAgent for the target and CallerAgentNeighbor
// for context-pack neighbors). Use RecordNoteAccess when the caller is
// determined by the runtime context (e.g. a shell hook setting
// VAULTMIND_CALLER).
func RecordNoteAccess(d *DB, noteID string) error {
	return RecordNoteAccessAs(d, noteID, "")
}

// RecordNoteAccessAs records a note access with an explicit caller
// label. Two side effects:
//
//  1. Inserts into note_accesses with (note_id, caller, accessed_at)
//     — the per-event log that `self` and future ACT-R retrieval
//     scoring read from.
//  2. Updates the scalar (notes.access_count, notes.last_accessed_at)
//     — kept for backward compatibility and fast lookup on hot paths.
//
// Best-effort: each per-note tracking miss is the caller's
// responsibility to log at debug; never fail the user query over
// optional bookkeeping.
func RecordNoteAccessAs(d *DB, noteID, caller string) error {
	if noteID == "" {
		return fmt.Errorf("RecordNoteAccess: noteID is empty")
	}
	resolved := resolveCaller(caller)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Append the event row first — this is the new source of truth
	// for caller-aware queries (self, future retrieval scoring). Use
	// INSERT ... SELECT FROM notes so a missing note id silently
	// inserts zero rows instead of triggering a foreign-key violation.
	// Preserves the pre-2026-05-01 best-effort contract documented on
	// RecordNoteAccess: tracking misses on phantom ids never fail the
	// user query (e.g. when an indexer race deletes the row between
	// retrieval and access recording).
	if _, err := d.Exec(
		`INSERT INTO note_accesses (note_id, caller, accessed_at)
		 SELECT id, ?, ? FROM notes WHERE id = ?`,
		resolved, now, noteID,
	); err != nil {
		return fmt.Errorf("logging access event for %q (caller %q): %w", noteID, resolved, err)
	}

	// Mirror to the scalar columns so existing callers (LookupNoteAccess,
	// the experiment scorer, doctor) keep working without per-event-table
	// joins. Both writes happen in a single best-effort path; if the
	// scalar update fails the event is still in the log.
	if _, err := d.Exec(
		`UPDATE notes SET access_count = access_count + 1, last_accessed_at = ? WHERE id = ?`,
		now, noteID,
	); err != nil {
		return fmt.Errorf("recording scalar access for %q: %w", noteID, err)
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

// ListAccessedNotes returns access stats across all notes with at least
// one recorded access, sorted newest-first by last access timestamp.
// Backs `vaultmind self` and any caller that wants "everything that's
// been touched." For the agent-only filtered view (excluding hook
// accesses), use ListAccessedNotesByCaller.
//
// Pre-2026-05-01 this read from the scalar columns. Post-migration-007
// it reads from the events table so callers see consistent data with
// the caller-filtered variant.
func ListAccessedNotes(d *DB) ([]NoteAccessStats, error) {
	return listAccessedNotes(d, "")
}

// ListAccessedNotesByCaller returns access stats restricted to events
// fired by the given caller. Used by `vaultmind self` to filter out
// hook fan-outs from the proprioceptive view: the SessionStart hook
// and per-turn pointer-recall fire RecordNoteAccess across many notes
// before the agent does any deliberate work, and showing them in the
// "hot" list pollutes the engagement signal `self` is supposed to
// surface. Pass an empty string to include all callers (matches
// ListAccessedNotes behaviour).
//
// The "exclude" semantic — "show all callers EXCEPT X" — is provided
// by ListAccessedNotesExcludingCaller, which is the shape `self`
// actually wants ("agent + agent-neighbor, not hook").
func ListAccessedNotesByCaller(d *DB, caller string) ([]NoteAccessStats, error) {
	return listAccessedNotes(d, caller)
}

// ListAccessedNotesExcludingCaller returns access stats from all
// callers EXCEPT the one named. The default `self` view uses this with
// CallerHook to show deliberate engagement minus the harness footprint.
func ListAccessedNotesExcludingCaller(d *DB, excludedCaller string) ([]NoteAccessStats, error) {
	if excludedCaller == "" {
		return ListAccessedNotes(d)
	}
	rows, err := d.Query(`
		SELECT
			n.id,
			COUNT(e.rowid) AS access_count,
			MAX(e.accessed_at) AS last_accessed_at,
			COALESCE(n.title, ''),
			COALESCE(n.type, '')
		FROM notes n
		JOIN note_accesses e ON e.note_id = n.id
		WHERE e.caller <> ?
		GROUP BY n.id
		ORDER BY last_accessed_at DESC
	`, excludedCaller)
	if err != nil {
		return nil, fmt.Errorf("listing accessed notes excluding caller %q: %w", excludedCaller, err)
	}
	defer func() { _ = rows.Close() }()
	return scanAccessRows(rows)
}

// listAccessedNotes is the internal shared implementation behind
// ListAccessedNotes (no filter) and ListAccessedNotesByCaller (filter).
// Reads from the events table so caller-filtered and unfiltered queries
// surface consistent data.
func listAccessedNotes(d *DB, callerFilter string) ([]NoteAccessStats, error) {
	var (
		rows interface {
			Next() bool
			Scan(...any) error
			Err() error
			Close() error
		}
		err error
	)
	if callerFilter == "" {
		rows, err = d.Query(`
			SELECT
				n.id,
				COUNT(e.rowid) AS access_count,
				MAX(e.accessed_at) AS last_accessed_at,
				COALESCE(n.title, ''),
				COALESCE(n.type, '')
			FROM notes n
			JOIN note_accesses e ON e.note_id = n.id
			GROUP BY n.id
			ORDER BY last_accessed_at DESC
		`)
	} else {
		rows, err = d.Query(`
			SELECT
				n.id,
				COUNT(e.rowid) AS access_count,
				MAX(e.accessed_at) AS last_accessed_at,
				COALESCE(n.title, ''),
				COALESCE(n.type, '')
			FROM notes n
			JOIN note_accesses e ON e.note_id = n.id
			WHERE e.caller = ?
			GROUP BY n.id
			ORDER BY last_accessed_at DESC
		`, callerFilter)
	}
	if err != nil {
		return nil, fmt.Errorf("listing accessed notes (caller=%q): %w", callerFilter, err)
	}
	defer func() { _ = rows.Close() }()
	return scanAccessRows(rows)
}

// scanAccessRows materialises NoteAccessStats from the listAccessedNotes
// query shape. Extracted so the three ListAccessedNotes* variants
// share a single scan/error path.
func scanAccessRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]NoteAccessStats, error) {
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
// came back populated). Reads from the scalar columns; for caller-aware
// lookups use the event-table queries directly.
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
