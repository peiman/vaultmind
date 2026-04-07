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

	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "note-a.md"), []byte("---\nid: dupe-id\ntype: concept\ntitle: Note A\ncreated: 2026-04-03\nvm_updated: 2026-04-03\n---\nBody A"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "note-b.md"), []byte("---\nid: dupe-id\ntype: concept\ntitle: Note B\ncreated: 2026-04-03\nvm_updated: 2026-04-03\n---\nBody B"), 0o644))

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
