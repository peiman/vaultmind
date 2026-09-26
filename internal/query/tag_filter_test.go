package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every lane of the hybrid retriever must honour the tag filter. A lane that
// ignores it feeds untagged notes into the fusion, so `--tag` returns notes
// that do not carry the tag. In the fixture, spreading-activation is tagged
// "retrieval" and episodic-memory is not; each lane is given data for both.
func TestRetrievers_EveryLaneHonoursTheTagFilter(t *testing.T) {
	lanes := map[string]func(t *testing.T) (retrieval.Retriever, string, string){
		"sparse": func(t *testing.T) (retrieval.Retriever, string, string) {
			db := buildRetrieverTestDB(t)
			tagged, untagged := taggedPair(t, db)
			require.NoError(t, index.StoreSparseEmbedding(db, tagged, map[int32]float32{100: 1}))
			require.NoError(t, index.StoreSparseEmbedding(db, untagged, map[int32]float32{100: 1}))
			embed := func(context.Context, string) (map[int32]float32, error) { return map[int32]float32{100: 1}, nil }
			return &query.SparseRetriever{DB: db, EmbedSparse: embed}, tagged, untagged
		},
		"colbert": func(t *testing.T) (retrieval.Retriever, string, string) {
			db := buildRetrieverTestDB(t)
			tagged, untagged := taggedPair(t, db)
			require.NoError(t, index.StoreColBERTEmbedding(db, tagged, [][]float32{{1, 0}}))
			require.NoError(t, index.StoreColBERTEmbedding(db, untagged, [][]float32{{1, 0}}))
			embed := func(context.Context, string) ([][]float32, error) { return [][]float32{{1, 0}}, nil }
			return &query.ColBERTRetriever{DB: db, EmbedColBERT: embed, Dims: 2}, tagged, untagged
		},
		"activation": func(t *testing.T) (retrieval.Retriever, string, string) {
			idxDB, expDB, _ := buildActivationTestStack(t)
			tagged, untagged := taggedPair(t, idxDB)
			sess, err := expDB.StartSession("/test")
			require.NoError(t, err)
			recordAccessForActivationTest(t, idxDB, expDB, sess, tagged)
			recordAccessForActivationTest(t, idxDB, expDB, sess, untagged)
			return &query.ActivationRetriever{DB: idxDB, ExpDB: expDB, Params: experiment.DefaultActivationParams(0.5)}, tagged, untagged
		},
	}
	for name, build := range lanes {
		t.Run(name, func(t *testing.T) {
			r, tagged, untagged := build(t)

			all, _, err := r.Search(context.Background(), "q", 10, 0, index.SearchFilters{})
			require.NoError(t, err)
			require.Contains(t, resultIDs(all), untagged, "unfiltered, the lane returns the untagged note")

			got, total, err := r.Search(context.Background(), "q", 10, 0, index.SearchFilters{Tag: "retrieval"})
			require.NoError(t, err)
			ids := resultIDs(got)
			assert.Contains(t, ids, tagged)
			assert.NotContains(t, ids, untagged, "a note without the tag is filtered out")
			assert.Equal(t, len(got), total, "the total counts only notes that pass the filter")
		})
	}
}

func taggedPair(t *testing.T, db *index.DB) (tagged, untagged string) {
	t.Helper()
	a, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	b, err := db.QueryNoteByPath("concepts/episodic-memory.md")
	require.NoError(t, err)
	require.NotNil(t, a)
	require.NotNil(t, b)
	return a.ID, b.ID
}

func resultIDs(rs []retrieval.ScoredResult) []string {
	ids := make([]string, len(rs))
	for i, r := range rs {
		ids[i] = r.ID
	}
	return ids
}
