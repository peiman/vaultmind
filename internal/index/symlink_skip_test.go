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

// writeSymlinkVault builds a vault holding one real note and one *.md symlink
// pointing at a file OUTSIDE the vault — the shape of an untrusted vault a user
// cloned. Returns the vault dir, the db path, and the secret's content.
func writeSymlinkVault(t *testing.T) (vaultDir, dbPath, secret string) {
	t.Helper()
	outside := t.TempDir()
	secret = "PRIVATE KEY MATERIAL sentinel-8f2a"
	secretFile := filepath.Join(outside, "id_rsa")
	require.NoError(t, os.WriteFile(secretFile, []byte(secret), 0o600))

	vaultDir = t.TempDir()
	vmDir := filepath.Join(vaultDir, ".vaultmind")
	require.NoError(t, os.MkdirAll(vmDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vmDir, "config.yaml"), []byte("types: {}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "real.md"),
		[]byte("---\nid: real-note\ntype: concept\ntitle: Real\ncreated: 2026-08-17\n---\nBody"), 0o644))
	require.NoError(t, os.Symlink(secretFile, filepath.Join(vaultDir, "secrets.md")))

	return vaultDir, filepath.Join(t.TempDir(), "index.db"), secret
}

// The end-to-end shape of the read primitive: indexing a vault must not pull a
// file from outside it into the database, and must say which path it passed over.
func TestRebuild_DoesNotIndexSymlinkedNotes(t *testing.T) {
	vaultDir, dbPath, secret := writeSymlinkVault(t)
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)

	result, err := index.NewIndexer(vaultDir, dbPath, cfg).Rebuild()
	require.NoError(t, err)

	assert.Equal(t, []string{"secrets.md"}, result.SkippedSymlinks,
		"the operator needs the path: a skipped note and a missing note look identical")

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var bodies int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notes WHERE body_text LIKE ?", "%"+secret+"%").Scan(&bodies))
	assert.Equal(t, 0, bodies, "content from outside the vault must never reach the index")

	var paths int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notes WHERE path = ?", "secrets.md").Scan(&paths))
	assert.Equal(t, 0, paths)
}

// Incremental is a separate walk with its own duplicate-id bookkeeping, so it
// gets its own proof rather than inheriting Rebuild's.
func TestIncremental_DoesNotIndexSymlinkedNotes(t *testing.T) {
	vaultDir, dbPath, secret := writeSymlinkVault(t)
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)

	result, err := index.NewIndexer(vaultDir, dbPath, cfg).Incremental()
	require.NoError(t, err)

	assert.Equal(t, []string{"secrets.md"}, result.SkippedSymlinks)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var bodies int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notes WHERE body_text LIKE ?", "%"+secret+"%").Scan(&bodies))
	assert.Equal(t, 0, bodies)
}

// An index built BEFORE this guard already holds symlinked rows, and upgrading
// must not leave them sitting in FTS and the embeddings forever. It does not
// need a migration: once the scanner stops returning the path, the row is an
// orphan and the existing sweep deletes it. This asserts that rather than
// assuming it — the sweep runs after the stores, and "it should be handled"
// is how the duplicate-id flip-flop survived three fixes.
func TestIncremental_SweepsSymlinkRowsLeftByAnOlderIndex(t *testing.T) {
	vaultDir, dbPath, secret := writeSymlinkVault(t)
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)

	// Simulate the pre-guard index: the symlinked note is a normal row.
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	_, err = db.Exec(
		`INSERT INTO notes (id, path, title, type, body_text, hash, mtime, is_domain)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 1)`,
		"leaked-note", "secrets.md", "Leaked", "concept", secret, "deadbeef", 1)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = index.NewIndexer(vaultDir, dbPath, cfg).Incremental()
	require.NoError(t, err)

	db2, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db2.Close() }()

	var rows int
	require.NoError(t, db2.QueryRow("SELECT COUNT(*) FROM notes WHERE path = ?", "secrets.md").Scan(&rows))
	assert.Equal(t, 0, rows,
		"a row left by an older index must be swept once the file stops being scanned")
}
