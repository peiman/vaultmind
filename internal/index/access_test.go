package index_test

import (
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RecordNoteAccess increments access_count by 1 and updates last_accessed_at.
// First slice of plasticity step 5 — verify the writer makes the columns
// meaningful. Migration 004 added the columns with default 0 / NULL; this
// test pins the contract that calling RecordNoteAccess actually moves them.
func TestRecordNoteAccess_IncrementsCountAndStampsTimestamp(t *testing.T) {
	dbPath := t.TempDir() + "/idx.db"
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, title, is_domain) VALUES (?, ?, ?, ?, ?, ?)`,
		"n1", "n1.md", "h", 0, "T", true,
	)
	require.NoError(t, err)

	// Pre-state: 0 access, no timestamp
	pre, err := index.LookupNoteAccess(db, "n1")
	require.NoError(t, err)
	assert.Equal(t, "n1", pre.NoteID)
	assert.Equal(t, 0, pre.AccessCount)
	assert.Empty(t, pre.LastAccessedAt)

	// First access
	require.NoError(t, index.RecordNoteAccess(db, "n1"))
	post1, err := index.LookupNoteAccess(db, "n1")
	require.NoError(t, err)
	assert.Equal(t, 1, post1.AccessCount, "first access bumps count to 1")
	assert.NotEmpty(t, post1.LastAccessedAt, "first access stamps timestamp")
	assert.True(t, strings.Contains(post1.LastAccessedAt, "T"),
		"timestamp should be RFC3339-like (got %q)", post1.LastAccessedAt)

	// Second access — count goes to 2, timestamp updates
	require.NoError(t, index.RecordNoteAccess(db, "n1"))
	post2, err := index.LookupNoteAccess(db, "n1")
	require.NoError(t, err)
	assert.Equal(t, 2, post2.AccessCount, "second access bumps count to 2")
	assert.NotEmpty(t, post2.LastAccessedAt)
}

// RecordNoteAccess against a non-existent ID must NOT silently succeed —
// otherwise typo/stale-id bugs in callers would be invisible. SQL UPDATE
// with no matching row affects 0 rows but doesn't error; we let the
// caller decide what to do via LookupNoteAccess returning empty NoteID.
func TestRecordNoteAccess_NonexistentNoteIsSilent(t *testing.T) {
	dbPath := t.TempDir() + "/idx.db"
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// No rows in notes table. RecordNoteAccess should not error
	// (the UPDATE is a no-op against an empty table) — but Lookup
	// for the same id returns empty NoteID, which is the
	// caller-detectable "not found" signal.
	require.NoError(t, index.RecordNoteAccess(db, "missing"))
	got, err := index.LookupNoteAccess(db, "missing")
	require.NoError(t, err)
	assert.Empty(t, got.NoteID, "missing note → empty NoteID in lookup")
	assert.Equal(t, 0, got.AccessCount)
}

// Empty noteID is a programming error — fail loud, not silent.
func TestRecordNoteAccess_EmptyIDErrors(t *testing.T) {
	dbPath := t.TempDir() + "/idx.db"
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	err = index.RecordNoteAccess(db, "")
	require.Error(t, err, "empty noteID must error")
	assert.Contains(t, err.Error(), "empty")
}
