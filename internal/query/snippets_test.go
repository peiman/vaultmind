package query

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPopulateRawSnippets_RemovedNote(t *testing.T) {
	db, err := index.Open(filepath.Join(t.TempDir(), "idx.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	results := []retrieval.ScoredResult{{ID: "removed"}}
	require.NoError(t, populateRawSnippets(db, results))
	assert.Empty(t, results[0].Snippet)
}
