package experiment_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openTestExpDB is a test helper that creates a temp-dir experiment database.
func openTestExpDB(t *testing.T) *experiment.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "experiment.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpen_CreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "experiment.db")

	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	_, statErr := os.Stat(dbPath)
	assert.NoError(t, statErr)
}

func TestOpen_CreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "xdg", "vaultmind", "experiments", "experiment.db")

	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, statErr := os.Stat(dbPath)
	assert.NoError(t, statErr)
}

func TestOpen_AppliesSchema(t *testing.T) {
	db := openTestExpDB(t)

	expectedTables := []string{"sessions", "events", "outcomes"}
	for _, table := range expectedTables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		require.NoError(t, err, "table %q must exist", table)
		assert.Equal(t, table, name)
	}
}

func TestOpen_SchemaVersion(t *testing.T) {
	db := openTestExpDB(t)

	var version int
	err := db.QueryRow("PRAGMA user_version").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 1, version)
}

func TestOpen_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "experiment.db")

	db1, err := experiment.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, db1.Close())

	db2, err := experiment.Open(dbPath)
	require.NoError(t, err)
	defer db2.Close()
	assert.NotNil(t, db2)

	// Schema version should still be 1 after reopen.
	var version int
	err = db2.QueryRow("PRAGMA user_version").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 1, version)
}

func TestOpen_WALMode(t *testing.T) {
	db := openTestExpDB(t)

	var journalMode string
	err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err)
	assert.Equal(t, "wal", journalMode)
}

func TestOpen_ForeignKeysEnabled(t *testing.T) {
	db := openTestExpDB(t)

	var fkEnabled int
	err := db.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled)
	require.NoError(t, err)
	assert.Equal(t, 1, fkEnabled)
}

func TestOpen_EventsIndexes(t *testing.T) {
	db := openTestExpDB(t)

	expectedIndexes := []string{
		"idx_events_session",
		"idx_events_type",
		"idx_events_timestamp",
		"idx_outcomes_event",
		"idx_outcomes_note",
	}
	for _, idx := range expectedIndexes {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='index' AND name=?", idx,
		).Scan(&name)
		require.NoError(t, err, "index %q must exist", idx)
		assert.Equal(t, idx, name)
	}
}
