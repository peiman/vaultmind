package experiment_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertRestricted checks the database AND its WAL sidecars. The sidecars are
// the half the first version of this test missed: it covered fresh creation
// only, where they happen to come out right, and reported the property as held.
func assertRestricted(t *testing.T, dbPath string) {
	t.Helper()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		path := dbPath + suffix
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue // sidecars are absent at rest; only present ones can leak
		}
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
			"%s is readable by other accounts and holds the same query text as the database",
			filepath.Base(path))
	}
}

// writeSensitive puts a known string through the real write path, which is what
// creates the WAL in the first place.
func writeSensitive(t *testing.T, db *experiment.DB, sentinel string) {
	t.Helper()
	sid, err := db.StartSession("/vault/" + sentinel)
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID: sid, Type: experiment.EventSearch,
		VaultPath: "/vault/" + sentinel, QueryText: sentinel,
	})
	require.NoError(t, err)
}

// A brand-new log: SQLite would create it 0644.
func TestOpen_FreshDatabaseAndSidecarsAreRestricted(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "experiments.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	writeSensitive(t, db, "fresh-sentinel")
	assertRestricted(t, dbPath)
}

// The upgrade path, and the one that was broken: a database created by an
// older version at 0644. Tightening the main file is not enough — SQLite
// derives the sidecars' mode from what it saw when it opened, so a chmod that
// runs after the open leaves -wal and -shm world-readable with the same
// query text inside them.
func TestOpen_PreExistingWorldReadableDatabaseIsTightenedWithItsSidecars(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "experiments.db")
	older, err := experiment.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, older.Close())
	require.NoError(t, os.Chmod(dbPath, 0o644), "simulate a database from before the fix")

	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	writeSensitive(t, db, "upgrade-sentinel")
	assertRestricted(t, dbPath)
}

// The reason this matters, stated as a test: the WAL really does contain the
// sensitive material, so its permissions are not a technicality.
func TestOpen_WALHoldsTheSameSensitiveDataAsTheDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "experiments.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	const sentinel = "wal-sentinel-9c31"
	writeSensitive(t, db, sentinel)

	wal, err := os.ReadFile(dbPath + "-wal")
	if os.IsNotExist(err) {
		t.Skip("no WAL at this moment; the permission tests above still bind")
	}
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(wal), sentinel),
		"the WAL carries query text and vault paths — which is why its mode is asserted, not assumed")
}

// The in-memory log has no file to restrict; it must not try to create one.
func TestOpenMemory_CreatesNoFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	db, err := experiment.OpenMemory()
	require.NoError(t, err)
	require.NoError(t, db.Close())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "an in-memory log must not leave a file named :memory: behind")
}
