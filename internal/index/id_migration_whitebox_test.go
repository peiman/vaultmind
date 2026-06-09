package index

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// whiteboxDB opens a fresh migrated DB in a temp dir for in-package tests that
// need to reach migrateNoteID's unexported error branches.
func whiteboxDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "wb.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedPathRow inserts a minimal stored row for path under oldID so a subsequent
// store under a new id triggers the migration path.
func seedPathRow(t *testing.T, db *DB, oldID, path string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, is_domain) VALUES (?, ?, 'h', 1, 0)`,
		oldID, path)
	require.NoError(t, err)
}

// TestMigrateNoteID_StoredLookupError covers storedIDForPath's error branch:
// when the notes table is missing, the SELECT fails and the error propagates.
func TestMigrateNoteID_StoredLookupError(t *testing.T) {
	db := whiteboxDB(t)
	tx, err := db.Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec("DROP TABLE notes")
	require.NoError(t, err)

	err = migrateNoteID(tx, "references/foo.md", "reference-foo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "looking up stored id")
}

// TestMigrateNoteID_EvictDependentError covers evictOldIDDependents' error
// branch: with a dependent table dropped, the eviction DELETE fails mid-migration.
func TestMigrateNoteID_EvictDependentError(t *testing.T) {
	db := whiteboxDB(t)
	const oldID = "_path:references/foo.md"
	const path = "references/foo.md"
	seedPathRow(t, db, oldID, path)

	tx, err := db.Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	// Drop the first dependent table so the eviction loop fails on it.
	_, err = tx.Exec("DROP TABLE aliases")
	require.NoError(t, err)

	err = migrateNoteID(tx, path, "reference-foo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "evicting old id from aliases")
}

// TestMigrateNoteID_EvictLinksError covers the outbound-links eviction branch:
// with links dropped (but the FK-dependent tables intact), the DELETE fails.
func TestMigrateNoteID_EvictLinksError(t *testing.T) {
	db := whiteboxDB(t)
	const oldID = "_path:references/foo.md"
	const path = "references/foo.md"
	seedPathRow(t, db, oldID, path)

	tx, err := db.Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec("DROP TABLE links")
	require.NoError(t, err)

	err = migrateNoteID(tx, path, "reference-foo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outbound links")
}

// TestMigrateNoteID_AccessForwardError covers the note_accesses re-key branch:
// dropping note_accesses makes the UPDATE fail after dependents are evicted.
func TestMigrateNoteID_AccessForwardError(t *testing.T) {
	db := whiteboxDB(t)
	const oldID = "_path:references/foo.md"
	const path = "references/foo.md"
	seedPathRow(t, db, oldID, path)

	tx, err := db.Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec("DROP TABLE note_accesses")
	require.NoError(t, err)

	err = migrateNoteID(tx, path, "reference-foo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "note_accesses")
}

// TestMigrateNoteID_NoOpGuards confirms the early-return guards: no stored row,
// or an unchanged id, are clean no-ops. Seeding happens BEFORE Begin() — under
// WAL SQLite a single connection is held by the open tx, so a db.Exec issued
// while the tx is live would deadlock on the connection pool.
func TestMigrateNoteID_NoOpGuards(t *testing.T) {
	db := whiteboxDB(t)
	// Stored row whose id already equals the incoming id → must be a no-op.
	seedPathRow(t, db, "reference-foo", "references/foo.md")

	tx, err := db.Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	// No stored row for this path → no-op, no error.
	require.NoError(t, migrateNoteID(tx, "references/missing.md", "reference-missing"))

	// Stored row whose id already equals the incoming id → no-op, no error.
	require.NoError(t, migrateNoteID(tx, "references/foo.md", "reference-foo"))
}
