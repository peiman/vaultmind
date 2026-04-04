package index_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRebuild_DatesStoredAsISO8601(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// ACT-R has created: 2026-04-03
	var created string
	require.NoError(t, db.QueryRow("SELECT created FROM notes WHERE id = ?", "concept-act-r").Scan(&created))

	// Must be ISO 8601 date, NOT Go time.Time string
	assert.Equal(t, "2026-04-03", created, "dates must be stored as ISO 8601, not time.Time.String()")
	assert.NotContains(t, created, "00:00:00", "must not contain time component")
	assert.NotContains(t, created, "UTC", "must not contain timezone")
}
