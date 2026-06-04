package index_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuild_PurgesNotesDeletedFromDisk pins the contract that a full rebuild
// reflects ONLY the files currently on disk: a note whose source file has been
// deleted must be removed from the index, and counted in result.Deleted.
//
// Regression guard for issue #40 — Rebuild() was upsert-only and never swept
// orphans, so deleted/excluded notes lingered in the index and polluted
// retrieval. Incremental() already swept; Rebuild() did not. RED before the
// fix (result.Deleted==0, stale note still present), GREEN after.
func TestRebuild_PurgesNotesDeletedFromDisk(t *testing.T) {
	vaultDir := t.TempDir()
	vmDir := filepath.Join(vaultDir, ".vaultmind")
	require.NoError(t, os.MkdirAll(vmDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vmDir, "config.yaml"), []byte("types: {}"), 0o644))

	keepPath := filepath.Join(vaultDir, "keep.md")
	dropPath := filepath.Join(vaultDir, "drop.md")
	require.NoError(t, os.WriteFile(keepPath,
		[]byte("---\nid: keep-note\ntype: concept\ntitle: Keep\ncreated: 2026-04-03\n---\nKeep body"), 0o644))
	require.NoError(t, os.WriteFile(dropPath,
		[]byte("---\nid: drop-note\ntype: concept\ntitle: Drop\ncreated: 2026-04-03\n---\nDrop body"), 0o644))

	dbPath := filepath.Join(t.TempDir(), "index.db")
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)

	// First rebuild: both notes indexed.
	idxr := index.NewIndexer(vaultDir, dbPath, cfg)
	first, err := idxr.Rebuild()
	require.NoError(t, err)
	assert.Equal(t, 2, first.Indexed, "first rebuild indexes both notes")
	assert.Equal(t, 0, first.Deleted, "first rebuild deletes nothing")

	// Remove one file from disk, then rebuild again.
	require.NoError(t, os.Remove(dropPath))

	second, err := idxr.Rebuild()
	require.NoError(t, err)

	assert.Equal(t, 1, second.Deleted, "rebuild must purge the note whose file was deleted")

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var dropCount, keepCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notes WHERE id = ?", "drop-note").Scan(&dropCount))
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notes WHERE id = ?", "keep-note").Scan(&keepCount))
	assert.Equal(t, 0, dropCount, "deleted-from-disk note must be gone from the index")
	assert.Equal(t, 1, keepCount, "surviving note must remain in the index")
}

// TestRebuild_PurgesAllNotesWhenVaultEmptied pins the boundary case: if every
// source file is removed, a full rebuild empties the index (rather than leaving
// every prior note as an orphan). Guards the sweep against an off-by-one that
// would skip deletion when nothing remains to store.
func TestRebuild_PurgesAllNotesWhenVaultEmptied(t *testing.T) {
	vaultDir := t.TempDir()
	vmDir := filepath.Join(vaultDir, ".vaultmind")
	require.NoError(t, os.MkdirAll(vmDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vmDir, "config.yaml"), []byte("types: {}"), 0o644))

	a := filepath.Join(vaultDir, "a.md")
	b := filepath.Join(vaultDir, "b.md")
	require.NoError(t, os.WriteFile(a,
		[]byte("---\nid: note-a\ntype: concept\ntitle: A\ncreated: 2026-04-03\n---\nA"), 0o644))
	require.NoError(t, os.WriteFile(b,
		[]byte("---\nid: note-b\ntype: concept\ntitle: B\ncreated: 2026-04-03\n---\nB"), 0o644))

	dbPath := filepath.Join(t.TempDir(), "index.db")
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultDir, dbPath, cfg)
	first, err := idxr.Rebuild()
	require.NoError(t, err)
	require.Equal(t, 2, first.Indexed)

	require.NoError(t, os.Remove(a))
	require.NoError(t, os.Remove(b))

	second, err := idxr.Rebuild()
	require.NoError(t, err)
	assert.Equal(t, 2, second.Deleted, "rebuild on an emptied vault purges every prior note")

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var total int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notes").Scan(&total))
	assert.Equal(t, 0, total, "index must be empty after rebuilding an emptied vault")
}

// TestRebuild_PurgesNewlyExcludedNotes pins that a full rebuild honors a
// freshly-added vault.exclude entry: a previously-indexed note that now falls
// under an exclude rule must be removed from the index. This is the exact
// reproduction in issue #40 (templates/ indexed, then excluded, --full
// reporting "0 deleted" and leaving the placeholder polluting retrieval).
func TestRebuild_PurgesNewlyExcludedNotes(t *testing.T) {
	vaultDir := t.TempDir()
	vmDir := filepath.Join(vaultDir, ".vaultmind")
	require.NoError(t, os.MkdirAll(vmDir, 0o750))
	configPath := filepath.Join(vmDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("types: {}"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "note.md"),
		[]byte("---\nid: real-note\ntype: concept\ntitle: Real\ncreated: 2026-04-03\n---\nReal body"), 0o644))

	templatesDir := filepath.Join(vaultDir, "templates")
	require.NoError(t, os.MkdirAll(templatesDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(templatesDir, "placeholder.md"),
		[]byte("---\nid: template-placeholder\ntype: source\ntitle: Placeholder\ncreated: 2026-04-03\n---\nPlaceholder body"), 0o644))

	dbPath := filepath.Join(t.TempDir(), "index.db")

	// First rebuild with default excludes (templates/ NOT excluded): both indexed.
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultDir, dbPath, cfg)
	first, err := idxr.Rebuild()
	require.NoError(t, err)
	assert.Equal(t, 2, first.Indexed, "first rebuild indexes the note and the template placeholder")

	// Add templates/ to vault.exclude, reload config, rebuild.
	require.NoError(t, os.WriteFile(configPath,
		[]byte("types: {}\nvault:\n  exclude:\n    - templates\n"), 0o644))
	cfgExcluded, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)
	idxrExcluded := index.NewIndexer(vaultDir, dbPath, cfgExcluded)
	second, err := idxrExcluded.Rebuild()
	require.NoError(t, err)

	assert.Equal(t, 1, second.Deleted, "rebuild must purge the newly-excluded template note")

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var templateCount, realCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notes WHERE id = ?", "template-placeholder").Scan(&templateCount))
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notes WHERE id = ?", "real-note").Scan(&realCount))
	assert.Equal(t, 0, templateCount, "newly-excluded note must be purged from the index")
	assert.Equal(t, 1, realCount, "non-excluded note must remain in the index")
}
