package index_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rebuildAndOpenDB(t *testing.T) *index.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

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

func TestQueryNoteByID_Found(t *testing.T) {
	db := rebuildAndOpenDB(t)

	row, err := db.QueryNoteByID("concept-act-r")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "concept-act-r", row.ID)
	assert.Equal(t, "concept", row.Type)
	assert.Equal(t, "ACT-R", row.Title)
}

func TestQueryNoteByID_NotFound(t *testing.T) {
	db := rebuildAndOpenDB(t)

	row, err := db.QueryNoteByID("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, row)
}

func TestQueryNoteByPath_Found(t *testing.T) {
	db := rebuildAndOpenDB(t)

	row, err := db.QueryNoteByPath("concepts/act-r.md")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "concept-act-r", row.ID)
}

func TestQueryNotesByTitle_Exact(t *testing.T) {
	db := rebuildAndOpenDB(t)

	rows, err := db.QueryNotesByTitle("ACT-R", false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "concept-act-r", rows[0].ID)
}

func TestQueryNotesByTitle_CaseInsensitive(t *testing.T) {
	db := rebuildAndOpenDB(t)

	rows, err := db.QueryNotesByTitle("act-r", true)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(rows), 1)
}

func TestQueryNotesByAlias_Exact(t *testing.T) {
	db := rebuildAndOpenDB(t)

	// concept-act-r has alias "ACT-R Architecture"
	rows, err := db.QueryNotesByAlias("ACT-R Architecture", false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "concept-act-r", rows[0].ID)
}

func TestQueryNotesByAlias_Normalized(t *testing.T) {
	db := rebuildAndOpenDB(t)

	rows, err := db.QueryNotesByAlias("act-r architecture", true)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(rows), 1)
}

func TestQueryFullNote_Found(t *testing.T) {
	db := rebuildAndOpenDB(t)

	note, err := db.QueryFullNote("concept-act-r")
	require.NoError(t, err)
	require.NotNil(t, note)
	assert.Equal(t, "concept-act-r", note.ID)
	assert.Equal(t, "concept", note.Type)
	assert.Equal(t, "ACT-R", note.Title)
	assert.NotEmpty(t, note.Body)
	assert.True(t, note.IsDomain)
}

func TestQueryFullNote_WithHeadings(t *testing.T) {
	db := rebuildAndOpenDB(t)

	note, err := db.QueryFullNote("concept-act-r")
	require.NoError(t, err)
	require.NotNil(t, note)
	assert.NotEmpty(t, note.Headings)
}

func TestQueryFullNote_WithFrontmatter(t *testing.T) {
	db := rebuildAndOpenDB(t)

	note, err := db.QueryFullNote("concept-act-r")
	require.NoError(t, err)
	require.NotNil(t, note)
	assert.NotEmpty(t, note.Aliases)
	assert.NotEmpty(t, note.Tags)
}

func TestQueryFullNote_NotFound(t *testing.T) {
	db := rebuildAndOpenDB(t)

	note, err := db.QueryFullNote("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, note)
}
