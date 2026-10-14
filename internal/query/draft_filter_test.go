package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The in-memory lanes do not share FTS SQL. They drop a draft through the
// same ExcludesDrafts policy, before the limit, or a hybrid search would
// rank it while keyword search hid it.
func TestSparseRetriever_DropsADraftThatWouldMatch(t *testing.T) {
	db := buildRetrieverTestDB(t)
	draft, published := taggedPair(t, db)
	_, err := db.Exec(`UPDATE notes SET status = ? WHERE id = ?`, index.StatusDraft, draft)
	require.NoError(t, err)
	require.NoError(t, index.StoreSparseEmbedding(db, draft, map[int32]float32{100: 5}))
	require.NoError(t, index.StoreSparseEmbedding(db, published, map[int32]float32{100: 1}))
	embed := func(context.Context, string) (map[int32]float32, error) { return map[int32]float32{100: 1}, nil }
	r := &query.SparseRetriever{DB: db, EmbedSparse: embed}

	got, total, err := r.Search(context.Background(), "q", 1, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.NotContains(t, resultIDs(got), draft)
	assert.Contains(t, resultIDs(got), published, "the published note fills the page the draft would have taken")
	assert.Equal(t, len(got), total)

	with, _, err := r.Search(context.Background(), "q", 10, 0, index.SearchFilters{IncludeDrafts: true})
	require.NoError(t, err)
	assert.Contains(t, resultIDs(with), draft)
}
