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

// IndexFile takes a vault-relative path and joins it onto the vault root. Its
// //nolint:gosec comment asserted "relPath is vault-relative" — an invariant
// nothing enforced. Every current caller happens to pass a validated path,
// which is exactly the state H3/H4 were in before someone added a caller that
// did not.
func TestIndexFile_RefusesAPathThatEscapesTheVault(t *testing.T) {
	parent := t.TempDir()
	vaultDir := filepath.Join(parent, "vault")
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, ".vaultmind", "config.yaml"), []byte("types: {}"), 0o644))

	secret := "PRIVATE KEY MATERIAL sentinel-7b4d"
	require.NoError(t, os.WriteFile(filepath.Join(parent, "outside.md"),
		[]byte("---\nid: outside-note\ntype: concept\ntitle: Outside\n---\n"+secret+"\n"), 0o600))

	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)
	dbPath := filepath.Join(t.TempDir(), "index.db")
	idxr := index.NewIndexer(vaultDir, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	err = idxr.IndexFile(filepath.Join("..", "outside.md"))
	require.Error(t, err, "a relPath that escapes the vault must be refused, not read")
	assert.ErrorIs(t, err, vault.ErrEscapesVault)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM notes WHERE body_text LIKE ?", "%"+secret+"%").Scan(&n))
	assert.Equal(t, 0, n, "content from outside the vault must never reach the index")
}
