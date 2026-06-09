package index_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStoreNote_PathCollisionMigratesID is the load-bearing regression test for
// the silent-data-loss bug: a note first indexed WITHOUT a frontmatter id is
// stored under a path-derived `_path:` id. When the file later GAINS a real id,
// the next index pass upserts the NEW id for the SAME path and used to hit
// `UNIQUE constraint failed: notes.path`, skipping the note (stale `_path:` row
// kept serving search; content never updated).
//
// The fix treats a path-collision-with-new-id as an id MIGRATION: the existing
// row is re-keyed to the new id, content updates land, and note_accesses
// (activation/recall history) is carried forward to the new id.
func TestStoreNote_PathCollisionMigratesID(t *testing.T) {
	db := openTestDB(t)
	const relPath = "references/foo.md"
	const oldID = "_path:references/foo.md"
	const newID = "reference-foo"

	// (1) First index WITHOUT an id → stored under the _path: id.
	first := buildTestRecord(oldID, relPath)
	first.IsDomain = false
	first.Hash = "hash-v1"
	first.BodyText = "Original body."
	require.NoError(t, index.StoreNote(db, first))

	// (2) Record a note_accesses row + scalar access for the _path: id.
	require.NoError(t, index.RecordNoteAccessAs(db, oldID, index.CallerAgent))

	// (3) The file GAINS `id: reference-foo`. Re-index the SAME path, new id.
	second := buildTestRecord(newID, relPath)
	second.Hash = "hash-v2"
	second.BodyText = "Updated body after gaining an id."

	// (4) Must NOT error on the UNIQUE(path) collision — it is an id migration.
	require.NoError(t, index.StoreNote(db, second),
		"path-collision with a new id must migrate, not fail with UNIQUE constraint")

	// (5a) The new id resolves and content updated.
	var gotID, gotBody string
	require.NoError(t, db.QueryRow(
		"SELECT id, body_text FROM notes WHERE path = ?", relPath).Scan(&gotID, &gotBody))
	assert.Equal(t, newID, gotID, "row should be re-keyed to the new id")
	assert.Equal(t, "Updated body after gaining an id.", gotBody, "content should update")

	// (5b) The OLD _path: id no longer exists.
	var oldCount int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM notes WHERE id = ?", oldID).Scan(&oldCount))
	assert.Equal(t, 0, oldCount, "stale _path: id must be gone")

	// (5c) note_accesses migrated to the new id (history survives).
	var newAccesses int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM note_accesses WHERE note_id = ?", newID).Scan(&newAccesses))
	assert.Equal(t, 1, newAccesses, "note_accesses must be carried forward to the new id")

	var oldAccesses int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM note_accesses WHERE note_id = ?", oldID).Scan(&oldAccesses))
	assert.Equal(t, 0, oldAccesses, "no orphaned note_accesses under the old id")

	// (5d) Scalar access history (access_count) carried forward on the row.
	stats, err := index.LookupNoteAccess(db, newID)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.AccessCount, "scalar access_count must survive the migration")
}

// TestStoreNote_NoMigrationWhenIDUnchanged guards the common path: a normal
// content update (same id, same path) must NOT trigger any id-migration
// side effects — access history stays put and the row updates in place.
func TestStoreNote_NoMigrationWhenIDUnchanged(t *testing.T) {
	db := openTestDB(t)
	rec := buildTestRecord("concept-stable", "concepts/stable.md")
	rec.Hash = "h1"
	require.NoError(t, index.StoreNote(db, rec))
	require.NoError(t, index.RecordNoteAccessAs(db, "concept-stable", index.CallerAgent))

	rec.Hash = "h2"
	rec.BodyText = "Edited in place."
	require.NoError(t, index.StoreNote(db, rec))

	stats, err := index.LookupNoteAccess(db, "concept-stable")
	require.NoError(t, err)
	assert.Equal(t, 1, stats.AccessCount, "same-id update must preserve access history")
	var body string
	require.NoError(t, db.QueryRow(
		"SELECT body_text FROM notes WHERE id = ?", "concept-stable").Scan(&body))
	assert.Equal(t, "Edited in place.", body)
}

// TestIncremental_AdoptsFrontmatterID is the end-to-end version through the
// incremental indexer: a real file gains an id and a re-index just works.
func TestIncremental_AdoptsFrontmatterID(t *testing.T) {
	vaultDir := t.TempDir()
	configDir := filepath.Join(vaultDir, ".vaultmind")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"),
		[]byte("types:\n  reference:\n    required: [title]\n    statuses: []\n"), 0o644))

	refDir := filepath.Join(vaultDir, "references")
	require.NoError(t, os.MkdirAll(refDir, 0o755))
	notePath := filepath.Join(refDir, "foo.md")

	// First index: NO frontmatter id → stored under _path:references/foo.md.
	require.NoError(t, os.WriteFile(notePath,
		[]byte("# Foo\n\nOriginal reference body.\n"), 0o644))

	dbPath := filepath.Join(t.TempDir(), "test.db")
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultDir, dbPath, cfg)

	_, err = idxr.Rebuild()
	require.NoError(t, err)

	const oldID = "_path:references/foo.md"
	const newID = "reference-foo"

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Confirm it landed under the _path: id and record an access against it.
	var startID string
	require.NoError(t, db.QueryRow(
		"SELECT id FROM notes WHERE path = ?", "references/foo.md").Scan(&startID))
	require.Equal(t, oldID, startID)
	require.NoError(t, index.RecordNoteAccessAs(db, oldID, index.CallerAgent))
	require.NoError(t, db.Close())

	// The file GAINS an id and new content.
	require.NoError(t, os.WriteFile(notePath,
		[]byte("---\nid: reference-foo\ntype: reference\ntitle: Foo\n---\n# Foo\n\nUpdated reference body.\n"), 0o644))
	futureTime := time.Now().Add(10 * time.Second)
	require.NoError(t, os.Chtimes(notePath, futureTime, futureTime))

	// Re-index: must update (not error/skip).
	result, err := idxr.Incremental()
	require.NoError(t, err)
	assert.Equal(t, 0, result.Errors, "no store error on id adoption")
	assert.Empty(t, result.ErrorDetails, "no skipped-file error details")
	assert.Equal(t, 1, result.Updated, "the note should be updated, not skipped")

	db2, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db2.Close() }()

	// New id resolves with updated content; old id gone; access carried forward.
	var gotID, gotBody string
	require.NoError(t, db2.QueryRow(
		"SELECT id, body_text FROM notes WHERE path = ?", "references/foo.md").Scan(&gotID, &gotBody))
	assert.Equal(t, newID, gotID)
	assert.Contains(t, gotBody, "Updated reference body.")

	var oldCount int
	require.NoError(t, db2.QueryRow(
		"SELECT COUNT(*) FROM notes WHERE id = ?", oldID).Scan(&oldCount))
	assert.Equal(t, 0, oldCount, "stale _path: id must be gone after adoption")

	var newAccesses int
	require.NoError(t, db2.QueryRow(
		"SELECT COUNT(*) FROM note_accesses WHERE note_id = ?", newID).Scan(&newAccesses))
	assert.Equal(t, 1, newAccesses, "access history must survive the id adoption")
}
