package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doctorNotesOutside indexes two notes into a database that is not inside
// the vault, so the vault directory can be emptied without deleting the rows
// doctor is looking at.
func doctorNotesOutside(t *testing.T) (dir string, db *index.DB) {
	t.Helper()
	dir = t.TempDir()
	body := func(id, title string) string {
		return "---\nid: " + id + "\ntype: concept\ntitle: " + title + "\ncreated: 2026-10-12\n---\n" + title + "\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.md"), []byte(body("keep-note", "Keep")), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "drop.md"), []byte(body("drop-note", "Drop")), 0o644))
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(t.TempDir(), "index.db")
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	db, err = index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return dir, db
}

func doctorText(t *testing.T, db *index.DB, dir string) string {
	t.Helper()
	res, err := query.Doctor(db, dir, nil)
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, writeDoctorHuman(&buf, res, false))
	return buf.String()
}

// A root the walk did not enter still has rows. Prescribing `vaultmind index`
// hands the operator a command that refuses to delete them.
func TestDoctor_EmptyRootDoesNotPrescribeIndex(t *testing.T) {
	dir, db := doctorNotesOutside(t)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		require.NoError(t, os.RemoveAll(filepath.Join(dir, e.Name())))
	}

	out := doctorText(t, db, dir)
	assert.Contains(t, out, "empty or unwalkable")
	root, err := filepath.Abs(dir)
	require.NoError(t, err)
	assert.Contains(t, out, root)
	assert.NotContains(t, out, staleIndexRemedy)
}

// A README is an entry. The notes are gone, and index will purge them.
// Doctor still names that command.
func TestDoctor_ReadmeOnlyStillPrescribesIndex(t *testing.T) {
	dir, db := doctorNotesOutside(t)
	require.NoError(t, os.Remove(filepath.Join(dir, "keep.md")))
	require.NoError(t, os.Remove(filepath.Join(dir, "drop.md")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Vault\n"), 0o644))

	out := doctorText(t, db, dir)
	assert.Contains(t, out, staleIndexRemedy)
	assert.NotContains(t, out, "empty or unwalkable")
}
