package query_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errWriter rejects every write. Pins the contract that RunSelf
// returns the writer's error rather than swallowing it — agent-noisy
// failures must surface, not silently truncate.
type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) { return 0, errors.New("write rejected") }

func TestRunSelf_PropagatesWriterError(t *testing.T) {
	now := time.Date(2026, 4, 29, 20, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	db, err := index.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime, access_count, last_accessed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"a", "a.md", "concept", "A", "h", 0, 1, now.UTC().Format(time.RFC3339Nano))
	require.NoError(t, err)

	err = query.RunSelf(db, query.SelfConfig{Now: now}, errWriter{})
	require.Error(t, err, "writer error must propagate")
	assert.Contains(t, err.Error(), "write rejected")
}

// Empty-vault writer error too — distinct code path.
func TestRunSelf_PropagatesWriterErrorOnEmptyVault(t *testing.T) {
	dir := t.TempDir()
	db, err := index.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	err = query.RunSelf(db, query.SelfConfig{}, errWriter{})
	require.Error(t, err, "writer error on empty vault must propagate")
}

// Stale rendering path — exercise the "drifting away" branch.
func TestRunSelf_PropagatesWriterErrorOnStalePath(t *testing.T) {
	now := time.Date(2026, 4, 29, 20, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	db, err := index.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime, access_count, last_accessed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"old", "old.md", "concept", "Old", "h", 0, 1, now.Add(-30*24*time.Hour).UTC().Format(time.RFC3339Nano))
	require.NoError(t, err)

	err = query.RunSelf(db, query.SelfConfig{Now: now, StaleThreshold: 7 * 24 * time.Hour}, errWriter{})
	require.Error(t, err)
}
