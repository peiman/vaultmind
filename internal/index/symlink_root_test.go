package index_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// indexVault is a two-note vault with its index already built from the real
// directory. dbPath is the vault's own .vaultmind/index.db.
func indexVault(t *testing.T) (dir, dbPath string, cfg *vault.Config) {
	t.Helper()
	dir = t.TempDir()
	vm := filepath.Join(dir, ".vaultmind")
	require.NoError(t, os.MkdirAll(vm, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vm, "config.yaml"), []byte("types: {}\n"), 0o644))
	writeIndexedNote(t, dir, "keep.md", "keep-note", "Keep")
	writeIndexedNote(t, dir, "drop.md", "drop-note", "Drop")

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath = filepath.Join(dir, cfg.Index.DBPath)
	first, err := index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	require.Equal(t, 2, first.Indexed)
	require.Equal(t, 2, indexedNoteCount(t, dbPath))
	return dir, dbPath, cfg
}

func writeIndexedNote(t *testing.T, dir, name, id, title string) {
	t.Helper()
	body := fmt.Sprintf("---\nid: %s\ntype: concept\ntitle: %s\ncreated: 2026-10-12\n---\n%s body\n", id, title, title)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
}

func indexedNoteCount(t *testing.T, dbPath string) int {
	t.Helper()
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notes").Scan(&n))
	return n
}

func reindex(t *testing.T, idxr *index.Indexer, mode string) (*index.IndexResult, error) {
	t.Helper()
	if mode == "rebuild" {
		return idxr.Rebuild()
	}
	return idxr.Incremental()
}

// A vault named through a symlink (the usual stable pointer, such as
// ~/vaultmind-identity -> the real directory) must index that directory.
// filepath.WalkDir lstats its root and does not descend when the root itself
// is a link, so the scan used to see zero files and the orphan sweep then
// deleted every note.
func TestIndex_SymlinkedVaultRootDoesNotWipeNotes(t *testing.T) {
	for _, mode := range []string{"incremental", "rebuild"} {
		t.Run(mode, func(t *testing.T) {
			real, dbPath, cfg := indexVault(t)
			outside := filepath.Join(t.TempDir(), "id_rsa")
			const secret = "PRIVATE KEY MATERIAL sentinel-symlink-root"
			require.NoError(t, os.WriteFile(outside, []byte(secret), 0o600))
			require.NoError(t, os.Symlink(outside, filepath.Join(real, "secrets.md")))

			link := filepath.Join(t.TempDir(), "vault-link")
			require.NoError(t, os.Symlink(real, link))

			res, err := reindex(t, index.NewIndexer(link, dbPath, cfg), mode)
			require.NoError(t, err)
			assert.Equal(t, 0, res.Deleted, "a symlinked vault root must not be treated as an empty vault")
			assert.Equal(t, 2, res.Skipped+res.Indexed,
				"the walk must see both real notes through the symlink (skipped=%d indexed=%d deleted=%d)",
				res.Skipped, res.Indexed, res.Deleted)
			assert.Equal(t, 2, indexedNoteCount(t, dbPath))

			db, err := index.Open(dbPath)
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			var leaked int
			require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notes WHERE body_text LIKE ?", "%"+secret+"%").Scan(&leaked))
			assert.Equal(t, 0, leaked, "resolving the vault root must not start following links inside the vault")
		})
	}
}

// An empty scan while the index still holds notes is how the symlink wipe
// happened, and the same shape occurs when an exclude list matches every
// note. The files are still on disk. Nothing may be deleted.
func TestIndex_EmptyScanWithNotesDeletesNothing(t *testing.T) {
	for _, mode := range []string{"incremental", "rebuild"} {
		t.Run(mode, func(t *testing.T) {
			dir, dbPath, _ := indexVault(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`types: {}
vault:
  exclude:
    - keep.md
    - drop.md
`), 0o644))
			cfg, err := vault.LoadConfig(dir)
			require.NoError(t, err)

			_, err = reindex(t, index.NewIndexer(dir, dbPath, cfg), mode)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "nothing was deleted")
			assert.Contains(t, err.Error(), "empty or unreadable")
			assert.Equal(t, 2, indexedNoteCount(t, dbPath), "notes must survive an empty scan")
		})
	}
}

// A vault the operator really emptied is indistinguishable from a scan that
// failed to see the files. The guard wins: the index is kept, and the error
// says to delete .vaultmind/index.db to reset it. There is no --force.
func TestIndex_EmptiedVaultKeepsTheIndex(t *testing.T) {
	for _, mode := range []string{"incremental", "rebuild"} {
		t.Run(mode, func(t *testing.T) {
			dir, dbPath, cfg := indexVault(t)
			require.NoError(t, os.Remove(filepath.Join(dir, "keep.md")))
			require.NoError(t, os.Remove(filepath.Join(dir, "drop.md")))

			_, err := reindex(t, index.NewIndexer(dir, dbPath, cfg), mode)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "nothing was deleted")
			assert.Contains(t, err.Error(), ".vaultmind/index.db")
			assert.Equal(t, 2, indexedNoteCount(t, dbPath))
		})
	}
}
