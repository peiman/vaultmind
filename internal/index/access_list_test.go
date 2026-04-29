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

	// Seed three notes: two accessed, one untouched. Use direct INSERTs
	// so the test doesn't depend on the indexer pipeline. hash + mtime
	// are NOT NULL per the schema; values are arbitrary for this test.
	_, err = db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime, access_count, last_accessed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "n1", "n1.md", "concept", "Note One", "h1", 0, 3, "2026-04-29T10:00:00Z")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime, access_count, last_accessed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "n2", "n2.md", "concept", "Note Two", "h2", 0, 1, "2026-04-29T12:00:00Z")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime, access_count, last_accessed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "n3", "n3.md", "concept", "Note Three", "h3", 0, 0, nil)
	require.NoError(t, err)

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
