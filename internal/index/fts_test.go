package index_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rebuildTestIndex(t *testing.T) *index.DB {
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

func TestSearchFTS_FindsByContent(t *testing.T) {
	db := rebuildTestIndex(t)

	results, err := index.SearchFTS(db, "cognitive architecture", 20, 0)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.NoteID
	}
	assert.Contains(t, ids, "concept-act-r")
}

func TestSearchFTS_FindsByTitle(t *testing.T) {
	db := rebuildTestIndex(t)

	results, err := index.SearchFTS(db, "Spreading Activation", 20, 0)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.NoteID
	}
	assert.Contains(t, ids, "concept-spreading-activation")
}

func TestSearchFTS_ReturnsSnippet(t *testing.T) {
	db := rebuildTestIndex(t)

	results, err := index.SearchFTS(db, "forgetting curve", 20, 0)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	assert.NotEmpty(t, results[0].Snippet)
}

func TestSearchFTS_LimitAndOffset(t *testing.T) {
	db := rebuildTestIndex(t)

	all, err := index.SearchFTS(db, "memory", 100, 0)
	require.NoError(t, err)

	limited, err := index.SearchFTS(db, "memory", 2, 0)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(limited), 2)

	if len(all) > 2 {
		offset, err := index.SearchFTS(db, "memory", 2, 2)
		require.NoError(t, err)
		assert.NotEmpty(t, offset)
		// Offset results should differ from first page
		assert.NotEqual(t, limited[0].NoteID, offset[0].NoteID)
	}
}

func TestSearchFTS_NoResults(t *testing.T) {
	db := rebuildTestIndex(t)

	results, err := index.SearchFTS(db, "xyznonexistent", 20, 0)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearchFTS_EmptyQuery(t *testing.T) {
	db := rebuildTestIndex(t)

	results, err := index.SearchFTS(db, "", 20, 0)
	require.NoError(t, err)
	assert.Empty(t, results)
}
