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

// An exclude list that matches every note still visits those files. The walk
// saw entries and found nothing to index. That is an emptied vault: the rows
// are purged.
func TestIndex_ExcludingEveryNotePurgesThem(t *testing.T) {
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

			res, err := reindex(t, index.NewIndexer(dir, dbPath, cfg), mode)
			require.NoError(t, err)
			assert.Equal(t, 2, res.Deleted)
			assert.Equal(t, 0, indexedNoteCount(t, dbPath))
		})
	}
}

// indexOutside builds the same two notes as indexVault, with the database
// outside the vault. A truly empty root is then possible: nothing under the
// vault directory, and the rows still exist to be wiped.
func indexOutside(t *testing.T) (dir, dbPath string, cfg *vault.Config) {
	t.Helper()
	dir = t.TempDir()
	writeIndexedNote(t, dir, "keep.md", "keep-note", "Keep")
	writeIndexedNote(t, dir, "drop.md", "drop-note", "Drop")
	var err error
	cfg, err = vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath = filepath.Join(t.TempDir(), "index.db")
	first, err := index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	require.Equal(t, 2, first.Indexed)
	return dir, dbPath, cfg
}

func removeAllEntries(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	for _, e := range entries {
		require.NoError(t, os.RemoveAll(filepath.Join(dir, e.Name())))
	}
}

func assertWipeRefused(t *testing.T, err error, dbPath, root string) {
	t.Helper()
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "nothing was deleted")
	assert.Contains(t, msg, "empty or unwalkable")
	assert.NotContains(t, msg, "unreadable")
	assert.Contains(t, msg, dbPath, "the reset must name the database that was opened")
	assert.Contains(t, msg, root, "the error must name the root the walk used")
	assert.NotContains(t, msg, ".vaultmind/index.db")
	assert.Equal(t, 2, indexedNoteCount(t, dbPath))
}

// A README is what `vaultmind init` leaves behind, and it is a default
// exclude. Deleting the notes and leaving that file is an emptied vault.
// The walk saw the file. The rows are purged.
func TestIndex_ReadmeOnlyVaultIsPurged(t *testing.T) {
	for _, mode := range []string{"incremental", "rebuild"} {
		t.Run(mode, func(t *testing.T) {
			dir, dbPath, cfg := indexOutside(t)
			require.NoError(t, os.Remove(filepath.Join(dir, "keep.md")))
			require.NoError(t, os.Remove(filepath.Join(dir, "drop.md")))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Vault\n"), 0o644))

			res, err := reindex(t, index.NewIndexer(dir, dbPath, cfg), mode)
			require.NoError(t, err)
			assert.Equal(t, 2, res.Deleted)
			assert.Equal(t, 0, indexedNoteCount(t, dbPath))
		})
	}
}

// .trash is a default exclude. Moving every note there is an emptied vault,
// not a scan that saw nothing: the directory itself was visited.
func TestIndex_NotesMovedToTrashArePurged(t *testing.T) {
	for _, mode := range []string{"incremental", "rebuild"} {
		t.Run(mode, func(t *testing.T) {
			dir, dbPath, cfg := indexOutside(t)
			trash := filepath.Join(dir, ".trash")
			require.NoError(t, os.Mkdir(trash, 0o750))
			require.NoError(t, os.Rename(filepath.Join(dir, "keep.md"), filepath.Join(trash, "keep.md")))
			require.NoError(t, os.Rename(filepath.Join(dir, "drop.md"), filepath.Join(trash, "drop.md")))

			res, err := reindex(t, index.NewIndexer(dir, dbPath, cfg), mode)
			require.NoError(t, err)
			assert.Equal(t, 2, res.Deleted)
			assert.Equal(t, 0, indexedNoteCount(t, dbPath))
		})
	}
}

// Zero entries under the root is the scan that used to wipe a vault addressed
// through a symlink. The rows stay, and the error names this database.
func TestIndex_ZeroEntriesRefusesToWipe(t *testing.T) {
	for _, mode := range []string{"incremental", "rebuild"} {
		t.Run(mode, func(t *testing.T) {
			dir, dbPath, cfg := indexOutside(t)
			removeAllEntries(t, dir)
			root, err := filepath.Abs(dir)
			require.NoError(t, err)

			_, err = reindex(t, index.NewIndexer(dir, dbPath, cfg), mode)
			assertWipeRefused(t, err, dbPath, root)
		})
	}
}

// A broken symlink is not a directory the walk can enter. Same refusal.
func TestIndex_BrokenSymlinkRootRefusesToWipe(t *testing.T) {
	for _, mode := range []string{"incremental", "rebuild"} {
		t.Run(mode, func(t *testing.T) {
			_, dbPath, cfg := indexOutside(t)
			link := filepath.Join(t.TempDir(), "broken")
			require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "missing"), link))
			root, err := filepath.Abs(link)
			require.NoError(t, err)

			_, err = reindex(t, index.NewIndexer(link, dbPath, cfg), mode)
			assertWipeRefused(t, err, dbPath, root)
		})
	}
}

// index.db_path is not always .vaultmind/index.db. The reset must name the
// file that still holds the rows.
func TestIndex_CustomDBPathIsNamedWhenTheScanSawNothing(t *testing.T) {
	dir := t.TempDir()
	vm := filepath.Join(dir, ".vaultmind")
	require.NoError(t, os.MkdirAll(vm, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vm, "config.yaml"), []byte("types: {}\nindex:\n  db_path: .vaultmind/custom.db\n"), 0o644))
	writeIndexedNote(t, dir, "keep.md", "keep-note", "Keep")
	writeIndexedNote(t, dir, "drop.md", "drop-note", "Drop")
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath, err := vault.ResolveInside(dir, cfg.Index.DBPath)
	require.NoError(t, err)
	require.Contains(t, dbPath, "custom.db")
	first, err := index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	require.Equal(t, 2, first.Indexed)

	empty := t.TempDir()
	root, err := filepath.Abs(empty)
	require.NoError(t, err)
	_, err = index.NewIndexer(empty, dbPath, cfg).Incremental()
	assertWipeRefused(t, err, dbPath, root)
}

// A symlinked parent (/link/vault, where /link is the symlink and vault is a
// real directory) is not the final-element case. The walk still sees the notes.
func TestIndex_SymlinkedParentDoesNotWipeNotes(t *testing.T) {
	real, dbPath, cfg := indexVault(t)
	parentLink := filepath.Join(t.TempDir(), "parent-link")
	require.NoError(t, os.Symlink(filepath.Dir(real), parentLink))
	via := filepath.Join(parentLink, filepath.Base(real))

	res, err := index.NewIndexer(via, dbPath, cfg).Incremental()
	require.NoError(t, err)
	assert.Equal(t, 0, res.Deleted)
	assert.Equal(t, 2, res.Skipped+res.Indexed)
	assert.Equal(t, 2, indexedNoteCount(t, dbPath))
}

// An in-vault directory symlink is not followed, and a symlink loop finishes.
// Notes outside the vault stay outside the index.
func TestIndex_DirectorySymlinkIsNotFollowed(t *testing.T) {
	real, dbPath, cfg := indexVault(t)
	outside := t.TempDir()
	const secret = "SECRET sentinel-dir-symlink"
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.md"), []byte("---\nid: secret-note\ntype: concept\ntitle: Secret\ncreated: 2026-10-12\n---\n"+secret+"\n"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(real, "linked")))
	require.NoError(t, os.Symlink(".", filepath.Join(real, "loop")))

	res, err := index.NewIndexer(real, dbPath, cfg).Incremental()
	require.NoError(t, err)
	assert.Equal(t, 0, res.Deleted)
	assert.Equal(t, 2, indexedNoteCount(t, dbPath))

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var leaked int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notes WHERE body_text LIKE ?", "%"+secret+"%").Scan(&leaked))
	assert.Equal(t, 0, leaked)
}

// Deleting the notes and leaving .vaultmind is not a walk that saw nothing.
// The directory is an entry. The rows are purged. A truly empty root is
// TestIndex_ZeroEntriesRefusesToWipe, and that database lives outside the vault.
func TestIndex_EmptiedOfNotesWithEntriesRemainingIsPurged(t *testing.T) {
	for _, mode := range []string{"incremental", "rebuild"} {
		t.Run(mode, func(t *testing.T) {
			dir, dbPath, cfg := indexVault(t)
			require.NoError(t, os.Remove(filepath.Join(dir, "keep.md")))
			require.NoError(t, os.Remove(filepath.Join(dir, "drop.md")))

			res, err := reindex(t, index.NewIndexer(dir, dbPath, cfg), mode)
			require.NoError(t, err)
			assert.Equal(t, 2, res.Deleted)
			assert.Equal(t, 0, indexedNoteCount(t, dbPath))
		})
	}
}
