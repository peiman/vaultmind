package index_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ListAccessedNotes returns the access stats for every note with
// access_count > 0 — the data backing `vaultmind self`. Empty result
// is valid (a freshly-indexed vault has no accesses recorded yet).
// Pins ordering: results sorted by last_accessed_at DESC so the caller
// gets "most recent first" without re-sorting.
func TestListAccessedNotes_ReturnsOnlyAccessedNotesNewestFirst(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Seed three notes: two accessed, one untouched. Post-migration-007
	// the source of truth is the note_accesses events table, so we
	// insert events directly with controlled timestamps. The scalar
	// columns are still maintained by RecordNoteAccess for backward
	// compat but ListAccessedNotes reads from events.
	for _, n := range []struct{ id, title, hash string }{
		{"n1", "Note One", "h1"},
		{"n2", "Note Two", "h2"},
		{"n3", "Note Three", "h3"},
	} {
		_, err = db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime) VALUES (?, ?, ?, ?, ?, ?)`,
			n.id, n.id+".md", "concept", n.title, n.hash, 0)
		require.NoError(t, err)
	}
	// n1 — three events, latest at 10:00.
	for i := 0; i < 3; i++ {
		_, err = db.Exec(`INSERT INTO note_accesses (note_id, caller, accessed_at) VALUES (?, ?, ?)`,
			"n1", "agent", "2026-04-29T10:00:00Z")
		require.NoError(t, err)
	}
	// n2 — one event at 12:00 (newest).
	_, err = db.Exec(`INSERT INTO note_accesses (note_id, caller, accessed_at) VALUES (?, ?, ?)`,
		"n2", "agent", "2026-04-29T12:00:00Z")
	require.NoError(t, err)
	// n3 — no events; must not appear in results.

	stats, err := index.ListAccessedNotes(db)
	require.NoError(t, err)
	require.Len(t, stats, 2, "untouched note must not appear")

	// Newest first: n2 (12:00) before n1 (10:00).
	assert.Equal(t, "n2", stats[0].NoteID, "n2 was accessed later, must come first")
	assert.Equal(t, 1, stats[0].AccessCount)
	assert.Equal(t, "Note Two", stats[0].Title)
	assert.Equal(t, "concept", stats[0].NoteType)

	assert.Equal(t, "n1", stats[1].NoteID)
	assert.Equal(t, 3, stats[1].AccessCount)
}

func TestListAccessedNotes_EmptyDBReturnsEmptySlice(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	stats, err := index.ListAccessedNotes(db)
	require.NoError(t, err)
	assert.Empty(t, stats)
}

// ListAccessedNotesExcludingCaller is the right-layer fix for the
// SessionStart-pollution bug: hook accesses inflate access_count and
// pollute the proprioceptive view. Filtering by caller != "hook" gives
// `vaultmind self` a clean engagement-only signal. Pins the contract
// that hook events are visible in the underlying log (via the
// unfiltered view) but suppressed from the default agent-facing view.
func TestListAccessedNotesExcludingCaller_FiltersHookAccessesFromAgentView(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	for _, n := range []string{"engaged", "preloaded", "mixed"} {
		_, err = db.Exec(
			`INSERT INTO notes (id, path, type, title, hash, mtime) VALUES (?, ?, ?, ?, ?, ?)`,
			n, n+".md", "concept", n, "h", 0,
		)
		require.NoError(t, err)
	}

	// "engaged" is touched only by agent (deliberate read).
	require.NoError(t, index.RecordNoteAccessAs(db, "engaged", index.CallerAgent))
	// "preloaded" is touched only by hook (SessionStart fan-out).
	require.NoError(t, index.RecordNoteAccessAs(db, "preloaded", index.CallerHook))
	// "mixed" gets one of each — should still surface in the agent view
	// because it has at least one non-hook access.
	require.NoError(t, index.RecordNoteAccessAs(db, "mixed", index.CallerHook))
	require.NoError(t, index.RecordNoteAccessAs(db, "mixed", index.CallerAgent))

	all, err := index.ListAccessedNotes(db)
	require.NoError(t, err)
	require.Len(t, all, 3, "unfiltered view must include every accessed note regardless of caller")

	agentView, err := index.ListAccessedNotesExcludingCaller(db, index.CallerHook)
	require.NoError(t, err)
	ids := make(map[string]int)
	for _, s := range agentView {
		ids[s.NoteID] = s.AccessCount
	}
	assert.Contains(t, ids, "engaged", "engaged note (agent-only) must appear in agent view")
	assert.Contains(t, ids, "mixed", "mixed note (one agent access) must appear in agent view")
	assert.NotContains(t, ids, "preloaded", "hook-only note must NOT pollute the agent view")
	assert.Equal(t, 1, ids["engaged"], "engaged: 1 agent access counted")
	assert.Equal(t, 1, ids["mixed"], "mixed: only the agent access counted, hook excluded")
}

// VAULTMIND_CALLER env var routes to the hook bucket when its value
// contains "hook". Pre-existing persona scripts already set
// VAULTMIND_CALLER=vaultmind-persona-hook so this is what makes them
// classify correctly without touching the scripts.
func TestRecordNoteAccess_ClassifiesEnvHookCallerAsHook(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime) VALUES (?, ?, ?, ?, ?, ?)`,
		"n", "n.md", "concept", "N", "h", 0)
	require.NoError(t, err)

	t.Setenv("VAULTMIND_CALLER", "vaultmind-persona-hook")
	require.NoError(t, index.RecordNoteAccess(db, "n"))

	// The hook caller wrote one event but the agent view filters it out.
	agentView, err := index.ListAccessedNotesExcludingCaller(db, index.CallerHook)
	require.NoError(t, err)
	assert.Empty(t, agentView, "env-set hook caller must not pollute the agent view")

	all, err := index.ListAccessedNotes(db)
	require.NoError(t, err)
	require.Len(t, all, 1, "the hook event is still in the underlying log")
	assert.Equal(t, "n", all[0].NoteID)
}

// ListAccessedNotes integrates with RecordNoteAccess: every note
// touched via Ask / note get / context-pack must show up in the list.
func TestListAccessedNotes_ReflectsRecordNoteAccessCalls(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime) VALUES ('a', 'a.md', 'concept', 'A', 'h', 0)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime) VALUES ('b', 'b.md', 'concept', 'B', 'h', 0)`)
	require.NoError(t, err)

	require.NoError(t, index.RecordNoteAccess(db, "a"))
	require.NoError(t, index.RecordNoteAccess(db, "a"))
	require.NoError(t, index.RecordNoteAccess(db, "b"))

	stats, err := index.ListAccessedNotes(db)
	require.NoError(t, err)
	require.Len(t, stats, 2)

	// Both have last_accessed_at set; ordering by recency is stable but
	// timestamps may collide at sub-millisecond granularity. Just check
	// counts surface correctly.
	byID := map[string]index.NoteAccessStats{stats[0].NoteID: stats[0], stats[1].NoteID: stats[1]}
	assert.Equal(t, 2, byID["a"].AccessCount)
	assert.Equal(t, 1, byID["b"].AccessCount)
	assert.NotEmpty(t, byID["a"].LastAccessedAt)
	assert.NotEmpty(t, byID["b"].LastAccessedAt)

	// Surface a parsed timestamp so the self-renderer can compute "Nm ago".
	ts, err := time.Parse(time.RFC3339Nano, byID["a"].LastAccessedAt)
	require.NoError(t, err, "last_accessed_at must be parseable as RFC3339Nano")
	assert.WithinDuration(t, time.Now(), ts, 5*time.Second)
}
