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

func TestRebuild_DetectsDuplicateIDs(t *testing.T) {
	// Create a temp vault with two files sharing the same ID
	vaultDir := t.TempDir()
	vmDir := filepath.Join(vaultDir, ".vaultmind")
	require.NoError(t, os.MkdirAll(vmDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vmDir, "config.yaml"), []byte("types: {}"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "note-a.md"), []byte("---\nid: dupe-id\ntype: concept\ntitle: Note A\ncreated: 2026-04-03\n---\nBody A"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "note-b.md"), []byte("---\nid: dupe-id\ntype: concept\ntitle: Note B\ncreated: 2026-04-03\n---\nBody B"), 0o644))

	dbPath := filepath.Join(t.TempDir(), "index.db")
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)

	idxr := index.NewIndexer(vaultDir, dbPath, cfg)
	result, err := idxr.Rebuild()
	require.NoError(t, err)

	// Must detect and report the duplicate
	assert.Greater(t, result.DuplicateIDs, 0, "must detect duplicate IDs")

	// Only the first file is indexed; the second is skipped with a warning
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notes WHERE id = ?", "dupe-id").Scan(&count))
	assert.Equal(t, 1, count, "duplicate ID results in one row (first file wins)")
}

// writeDupeVault builds a vault whose two notes claim one id, plus the config
// the indexer needs. Returns the vault dir and the db path.
func writeDupeVault(t *testing.T) (string, string) {
	t.Helper()
	vaultDir := t.TempDir()
	vmDir := filepath.Join(vaultDir, ".vaultmind")
	require.NoError(t, os.MkdirAll(vmDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vmDir, "config.yaml"), []byte("types: {}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "note-a.md"),
		[]byte("---\nid: dupe-id\ntype: concept\ntitle: Note A\ncreated: 2026-08-16\n---\nBody A"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "note-b.md"),
		[]byte("---\nid: dupe-id\ntype: concept\ntitle: Note B\ncreated: 2026-08-16\n---\nBody B"), 0o600))
	return vaultDir, filepath.Join(t.TempDir(), "index.db")
}

func titleForID(t *testing.T, dbPath, id string) string {
	t.Helper()
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var title string
	require.NoError(t, db.QueryRow("SELECT title FROM notes WHERE id = ?", id).Scan(&title))
	return title
}

// Rebuild skips a second file claiming an id; Incremental had no such guard, so
// it upserted with ON CONFLICT(id) DO UPDATE SET path=… and the row changed
// hands. Because stored hashes are keyed by PATH, the dispossessed file then
// looked new on the following run and took the id back — so the note behind a
// stable id alternated on every `vaultmind index`, forever.
//
// For a memory system that is the worst available failure: what the agent
// recalls under one id silently changes, and `doctor` reports zero duplicate
// ids because notes.id is UNIQUE and the row count is structurally one.
func TestIncremental_DuplicateIDDoesNotFlipFlop(t *testing.T) {
	vaultDir, dbPath := writeDupeVault(t)
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultDir, dbPath, cfg)

	seen := make([]string, 0, 3)
	for range 3 {
		result, incErr := idxr.Incremental()
		require.NoError(t, incErr)
		assert.Positive(t, result.DuplicateIDs, "every run must report the collision, not just the first")
		seen = append(seen, titleForID(t, dbPath, "dupe-id"))
	}

	assert.Equal(t, []string{seen[0], seen[0], seen[0]}, seen,
		"the note behind the id must not change hands between runs")
	assert.Equal(t, "Note A", seen[0], "and the winner is the first in scan order, as Rebuild already does")
}

// The guard must not mistake a MOVE for a collision. Incremental upserts before
// it sweeps orphans, so at the moment the moved file is stored the old path
// still owns the row — a naive "another path holds this id" check would reject
// every rename and leave the note stranded at its old path.
func TestIncremental_RenamedNoteKeepsItsIDAtTheNewPath(t *testing.T) {
	vaultDir := t.TempDir()
	vmDir := filepath.Join(vaultDir, ".vaultmind")
	require.NoError(t, os.MkdirAll(vmDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vmDir, "config.yaml"), []byte("types: {}"), 0o600))
	oldPath := filepath.Join(vaultDir, "before.md")
	require.NoError(t, os.WriteFile(oldPath,
		[]byte("---\nid: moving-note\ntype: concept\ntitle: Moving\ncreated: 2026-08-16\n---\nBody"), 0o600))

	dbPath := filepath.Join(t.TempDir(), "index.db")
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultDir, dbPath, cfg)

	_, err = idxr.Incremental()
	require.NoError(t, err)

	require.NoError(t, os.Rename(oldPath, filepath.Join(vaultDir, "after.md")))
	result, err := idxr.Incremental()
	require.NoError(t, err)
	assert.Zero(t, result.DuplicateIDs, "a move is not a duplicate")

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var path string
	require.NoError(t, db.QueryRow("SELECT path FROM notes WHERE id = ?", "moving-note").Scan(&path))
	assert.Equal(t, "after.md", path, "the id follows the file to its new path")
}

func TestRebuild_DuplicateID_FirstFileWins(t *testing.T) {
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, "concepts"), 0o755))

	note1 := "---\nid: concept-duplicate\ntype: concept\ntitle: First\ncreated: 2026-04-07\n---\nFirst body"
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "concepts", "first.md"), []byte(note1), 0o644))

	note2 := "---\nid: concept-duplicate\ntype: concept\ntitle: Second\ncreated: 2026-04-07\n---\nSecond body"
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "concepts", "second.md"), []byte(note2), 0o644))

	configDir := filepath.Join(vaultDir, ".vaultmind")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"),
		[]byte("index:\n  db_path: .vaultmind/index.db\n"), 0o644))

	dbPath := filepath.Join(vaultDir, ".vaultmind", "index.db")
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)

	idxr := index.NewIndexer(vaultDir, dbPath, cfg)
	result, err := idxr.Rebuild()
	require.NoError(t, err)

	assert.Equal(t, 1, result.DuplicateIDs, "should detect 1 duplicate ID")

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var noteCount int
	err = db.QueryRow("SELECT COUNT(*) FROM notes WHERE id = ?", "concept-duplicate").Scan(&noteCount)
	require.NoError(t, err)
	assert.Equal(t, 1, noteCount, "should have exactly one note with the duplicate ID")

	var title string
	err = db.QueryRow("SELECT title FROM notes WHERE id = ?", "concept-duplicate").Scan(&title)
	require.NoError(t, err)
	assert.Equal(t, "First", title, "first file should win — second file must be skipped")
}
