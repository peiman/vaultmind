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

	// Both files should still be indexed (last one wins, but warning emitted)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notes WHERE id = ?", "dupe-id").Scan(&count))
	assert.Equal(t, 1, count, "duplicate ID results in one row (upsert)")
}
