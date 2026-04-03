package index_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_CreatesNewDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	_, statErr := os.Stat(dbPath)
	assert.NoError(t, statErr)
}

func TestOpen_AppliesFullSchema(t *testing.T) {
	db := openTestDB(t)

	expectedTables := []string{
		"notes", "aliases", "tags", "frontmatter_kv",
		"links", "blocks", "headings", "generated_sections",
	}
	for _, table := range expectedTables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		require.NoError(t, err, "table %q must exist", table)
		assert.Equal(t, table, name)
	}
}

func TestOpen_CreatesFTSTable(t *testing.T) {
	db := openTestDB(t)

	var name string
	err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='fts_notes'",
	).Scan(&name)
	require.NoError(t, err, "fts_notes virtual table must exist")
	assert.Equal(t, "fts_notes", name)
}

func TestOpen_CreatesAllIndexes(t *testing.T) {
	db := openTestDB(t)

	expectedIndexes := []string{
		"idx_aliases_normalized",
		"idx_tags_tag",
		"idx_fmkv_note",
		"idx_links_src",
		"idx_links_dst",
		"idx_links_edge_type",
		"idx_links_confidence",
		"idx_links_src_resolved",
		"idx_links_unique",
		"idx_notes_type",
		"idx_aliases_note",
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

func TestOpen_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	db1, err := index.Open(dbPath)
	require.NoError(t, err)
	db1.Close()

	db2, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db2.Close()
	assert.NotNil(t, db2)
}

func TestOpen_CreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".vaultmind", "index.db")

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, statErr := os.Stat(dbPath)
	assert.NoError(t, statErr)
}

func TestOpen_EnablesWALMode(t *testing.T) {
	db := openTestDB(t)

	var journalMode string
	err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err)
	assert.Equal(t, "wal", journalMode)
}

func TestOpen_EnablesForeignKeys(t *testing.T) {
	db := openTestDB(t)

	var fkEnabled int
	err := db.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled)
	require.NoError(t, err)
	assert.Equal(t, 1, fkEnabled)
}

func TestDB_QueryRow_Accessible(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec("INSERT INTO notes (id, path, hash, mtime) VALUES (?, ?, ?, ?)",
		"test-id", "test.md", "abc123", 1234567890)
	require.NoError(t, err)
}

// openTestDB is a test helper that creates a temp-dir database.
func openTestDB(t *testing.T) *index.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}
