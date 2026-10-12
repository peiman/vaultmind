package index_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withIndexLog captures indexer warnings. Index tests do not run in parallel,
// so replacing the process logger for one test does not cross another test.
func withIndexLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := log.Logger
	log.Logger = zerolog.New(&buf).Level(zerolog.InfoLevel)
	t.Cleanup(func() { log.Logger = old })
	return &buf
}

func writeSingleNoteVault(t *testing.T, name, body string) (vaultDir, dbPath, notePath string) {
	t.Helper()
	vaultDir = t.TempDir()
	vmDir := filepath.Join(vaultDir, ".vaultmind")
	require.NoError(t, os.MkdirAll(vmDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vmDir, "config.yaml"), []byte("types: {}"), 0o600))
	notePath = filepath.Join(vaultDir, name)
	require.NoError(t, os.WriteFile(notePath, []byte(body), 0o600))
	dbPath = filepath.Join(t.TempDir(), "index.db")
	return vaultDir, dbPath, notePath
}

// A rename updates the row's path before the orphan sweep looks at the old
// one. The old path is already gone. That is the outcome the sweep wanted:
// no warning, and it is not listed as a skipped file.
func TestIncremental_AlreadyGoneOrphanIsNotAnError(t *testing.T) {
	logs := withIndexLog(t)
	vaultDir, dbPath, oldPath := writeSingleNoteVault(t, "before.md",
		"---\nid: moving-note\ntype: concept\ntitle: Moving\ncreated: 2026-08-16\n---\nBody")
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultDir, dbPath, cfg)

	_, err = idxr.Incremental()
	require.NoError(t, err)

	require.NoError(t, os.Rename(oldPath, filepath.Join(vaultDir, "after.md")))
	logs.Reset()
	result, err := idxr.Incremental()
	require.NoError(t, err)

	assert.Zero(t, result.Errors, "a row that is already gone is not a delete failure")
	assert.Empty(t, result.ErrorDetails, "it must not be listed as a skipped file")
	assert.Zero(t, result.Skipped, "the missing path is not an unchanged file")
	assert.Zero(t, result.Deleted, "nothing was deleted; the row moved to the new path")
	assert.NotContains(t, logs.String(), "failed to delete orphaned note")
	assert.NotContains(t, logs.String(), "no rows in result set")
}

// A delete that fails for a reason other than "already gone" is still a
// failure the operator has to see.
//
// A second note stays on disk so the walk still visits an entry. A walk that
// visits nothing is the wipe guard and returns before any delete.
func TestIncremental_RealOrphanDeleteErrorStillWarns(t *testing.T) {
	logs := withIndexLog(t)
	vaultDir, dbPath, notePath := writeSingleNoteVault(t, "doomed.md",
		"---\nid: doomed\ntype: concept\ntitle: Doomed\ncreated: 2026-08-16\n---\nBody")
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "keep.md"), []byte(
		"---\nid: kept\ntype: concept\ntitle: Kept\ncreated: 2026-08-16\n---\nStays"), 0o600))
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultDir, dbPath, cfg)

	_, err = idxr.Incremental()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TRIGGER abort_note_delete BEFORE DELETE ON notes
BEGIN SELECT RAISE(ABORT, 'forced delete failure'); END;`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	require.NoError(t, os.Remove(notePath))
	logs.Reset()
	result, err := idxr.Incremental()
	require.NoError(t, err)

	assert.Equal(t, 1, result.Errors)
	require.Len(t, result.ErrorDetails, 1)
	assert.Equal(t, "delete", result.ErrorDetails[0].Kind)
	assert.Contains(t, result.ErrorDetails[0].Error, "forced delete failure")
	assert.NotContains(t, result.ErrorDetails[0].Error, "no rows in result set")
	assert.Contains(t, logs.String(), "failed to delete orphaned note")
	assert.Contains(t, logs.String(), "forced delete failure")
	assert.Zero(t, result.Deleted, "the row is still there when the delete aborts")
}
