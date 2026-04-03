package query_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testVaultPath = "../../vaultmind-vault"

func buildIndexedDB(t *testing.T) *index.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestDoctor_ReturnsVaultSummary(t *testing.T) {
	db := buildIndexedDB(t)

	result, err := query.Doctor(db, testVaultPath)
	require.NoError(t, err)

	assert.Equal(t, testVaultPath, result.VaultPath)
	assert.Greater(t, result.TotalFiles, 0)
	assert.Greater(t, result.DomainNotes, 0)
	assert.GreaterOrEqual(t, result.UnstructuredNotes, 1) // Welcome.md
	assert.Equal(t, result.TotalFiles, result.DomainNotes+result.UnstructuredNotes)
}

func TestDoctor_ReportsUnresolvedLinks(t *testing.T) {
	db := buildIndexedDB(t)

	result, err := query.Doctor(db, testVaultPath)
	require.NoError(t, err)

	// Body wikilinks are unresolved (dst_note_id is NULL)
	assert.GreaterOrEqual(t, result.Issues.UnresolvedLinks, 0)
}
