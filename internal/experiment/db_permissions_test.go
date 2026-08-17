package experiment_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This database is the most identifying artifact VaultMind writes: full query
// text, vault paths, note ids, and caller metadata ($USER, hostname,
// CLAUDE_PROJECT_DIR). SQLite creates it 0644 — readable by every account on
// the machine — while the far less sensitive telemetry fingerprint file was
// already 0600.
func TestOpen_DatabaseIsNotWorldReadable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "experiments.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	info, err := os.Stat(dbPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"query text and hostname must not be readable by other accounts on the machine")
}
