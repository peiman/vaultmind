package query_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ query.Retriever = (*query.FTSRetriever)(nil)

func buildRetrieverTestDB(t *testing.T) *index.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	vaultPath := "../../vaultmind-vault"
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestFTSRetriever_Search(t *testing.T) {
	db := buildRetrieverTestDB(t)
	retriever := &query.FTSRetriever{DB: db}
	results, total, err := retriever.Search(context.Background(), "memory", 5, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(results), 5)
	assert.GreaterOrEqual(t, total, len(results))
	for _, r := range results {
		assert.NotEmpty(t, r.ID)
		assert.GreaterOrEqual(t, r.Score, 0.0)
		assert.LessOrEqual(t, r.Score, 1.0)
	}
}

func TestFTSRetriever_SearchEmpty(t *testing.T) {
	db := buildRetrieverTestDB(t)
	retriever := &query.FTSRetriever{DB: db}
	results, total, err := retriever.Search(context.Background(), "", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.Equal(t, 0, total)
}
