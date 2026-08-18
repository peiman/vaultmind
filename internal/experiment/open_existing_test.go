package experiment_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `experiments.telemetry: off` promises nothing is written. A read command that
// creates the database as a side effect of reporting on it breaks that promise
// with a file on disk — and the reader would never know, because the report it
// prints is empty either way.
func TestOpenExisting_DoesNotCreateTheLog(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "experiments.db")

	db, err := experiment.OpenExisting(dbPath)
	require.Error(t, err)
	assert.Nil(t, db)
	assert.ErrorIs(t, err, experiment.ErrNoUsageLog)
	assert.Contains(t, err.Error(), dbPath, "name the path that is absent")

	_, statErr := os.Stat(dbPath)
	assert.True(t, os.IsNotExist(statErr),
		"asking what is in the log must not be the thing that creates one")
}

// Turning telemetry off is a decision about NEW data. An operator must still be
// able to read what was already collected — and to export it, which is how they
// would get it out before deleting it.
func TestOpenExisting_ReadsALogThatIsAlreadyThere(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "experiments.db")
	created, err := experiment.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, created.Close())

	db, err := experiment.OpenExisting(dbPath)
	require.NoError(t, err)
	require.NotNil(t, db)
	assert.NoError(t, db.Close())
}

// errors.Is is the interface: callers distinguish "no log yet" from a real
// failure so they can print a plain sentence instead of a stack of wrapping.
func TestOpenExisting_SentinelIsMatchable(t *testing.T) {
	_, err := experiment.OpenExisting(filepath.Join(t.TempDir(), "nope.db"))
	assert.True(t, errors.Is(err, experiment.ErrNoUsageLog))
}
