package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ query.CandidateSearcher = (*query.ColBERTRetriever)(nil)

// SearchAmong scores only the named notes and reads only their vectors —
// ColBERT is ~99% of a BGE-M3 index, so reading all of it was the other half
// of the cost.
func TestColBERTRetriever_SearchAmongScoresOnlyTheNamedNotes(t *testing.T) {
	db := buildRetrieverTestDB(t)
	row1, _ := db.QueryNoteByPath("concepts/spreading-activation.md")
	row2, _ := db.QueryNoteByPath("concepts/episodic-memory.md")
	require.NotNil(t, row1)
	require.NotNil(t, row2)
	require.NoError(t, index.StoreColBERTEmbedding(db, row1.ID, [][]float32{{1, 0}}))
	require.NoError(t, index.StoreColBERTEmbedding(db, row2.ID, [][]float32{{1, 0}}))
	embed := func(context.Context, string) ([][]float32, error) { return [][]float32{{1, 0}}, nil }
	r := &query.ColBERTRetriever{DB: db, EmbedColBERT: embed, Dims: 2}

	got, total, err := r.SearchAmong(context.Background(), "q", 10, index.SearchFilters{}, []string{row2.ID, "no-such-note"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, row2.ID, got[0].ID)
	assert.Equal(t, 1, total)

	loaded, err := index.LoadColBERTEmbeddingsFor(db, 2, []string{row1.ID})
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.Equal(t, row1.ID, loaded[0].NoteID)
	empty, err := index.LoadColBERTEmbeddingsFor(db, 2, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// Filters still apply among candidates.
func TestColBERTRetriever_SearchAmongHonoursTheTagFilter(t *testing.T) {
	db := buildRetrieverTestDB(t)
	tagged, _ := db.QueryNoteByPath("concepts/spreading-activation.md")
	untagged, _ := db.QueryNoteByPath("concepts/episodic-memory.md")
	require.NoError(t, index.StoreColBERTEmbedding(db, tagged.ID, [][]float32{{1, 0}}))
	require.NoError(t, index.StoreColBERTEmbedding(db, untagged.ID, [][]float32{{1, 0}}))
	embed := func(context.Context, string) ([][]float32, error) { return [][]float32{{1, 0}}, nil }
	r := &query.ColBERTRetriever{DB: db, EmbedColBERT: embed, Dims: 2}

	got, _, err := r.SearchAmong(context.Background(), "q", 10, index.SearchFilters{Tag: "retrieval"}, []string{tagged.ID, untagged.ID})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, tagged.ID, got[0].ID)
}
