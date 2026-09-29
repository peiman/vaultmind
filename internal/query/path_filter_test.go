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

// Every lane must honour the path filter. A lane that ignores it feeds notes
// from outside the folder into the fusion, so `--path` returns them. In the
// fixture, spreading-activation lives under concepts/ and decision-example
// does not; each lane is given data for both.
func TestRetrievers_EveryLaneHonoursThePathFilter(t *testing.T) {
	lanes := map[string]func(t *testing.T) (retrieval.Retriever, string, string){
		"sparse": func(t *testing.T) (retrieval.Retriever, string, string) {
			db := buildRetrieverTestDB(t)
			inside, outside := folderPair(t, db)
			require.NoError(t, index.StoreSparseEmbedding(db, inside, map[int32]float32{100: 1}))
			require.NoError(t, index.StoreSparseEmbedding(db, outside, map[int32]float32{100: 1}))
			embed := func(context.Context, string) (map[int32]float32, error) { return map[int32]float32{100: 1}, nil }
			return &query.SparseRetriever{DB: db, EmbedSparse: embed}, inside, outside
		},
		"colbert": func(t *testing.T) (retrieval.Retriever, string, string) {
			db := buildRetrieverTestDB(t)
			inside, outside := folderPair(t, db)
			require.NoError(t, index.StoreColBERTEmbedding(db, inside, [][]float32{{1, 0}}))
			require.NoError(t, index.StoreColBERTEmbedding(db, outside, [][]float32{{1, 0}}))
			embed := func(context.Context, string) ([][]float32, error) { return [][]float32{{1, 0}}, nil }
			return &query.ColBERTRetriever{DB: db, EmbedColBERT: embed, Dims: 2}, inside, outside
		},
		"activation": func(t *testing.T) (retrieval.Retriever, string, string) {
			idxDB, expDB, _ := buildActivationTestStack(t)
			inside, outside := folderPair(t, idxDB)
			sess, err := expDB.StartSession("/test")
			require.NoError(t, err)
			recordAccessForActivationTest(t, idxDB, expDB, sess, inside)
			recordAccessForActivationTest(t, idxDB, expDB, sess, outside)
			return &query.ActivationRetriever{DB: idxDB, ExpDB: expDB, Params: experiment.DefaultActivationParams(0.5)}, inside, outside
		},
	}
	for name, build := range lanes {
		t.Run(name, func(t *testing.T) {
			r, inside, outside := build(t)

			all, _, err := r.Search(context.Background(), "q", 10, 0, index.SearchFilters{})
			require.NoError(t, err)
			require.Contains(t, resultIDs(all), outside, "unfiltered, the lane returns the note outside the folder")

			got, total, err := r.Search(context.Background(), "q", 10, 0, index.SearchFilters{PathPrefix: "concepts/"})
			require.NoError(t, err)
			ids := resultIDs(got)
			assert.Contains(t, ids, inside)
			assert.NotContains(t, ids, outside, "a note outside the prefix is filtered out")
			assert.Equal(t, len(got), total, "the total counts only notes that pass the filter")
		})
	}
}

func folderPair(t *testing.T, db *index.DB) (inside, outside string) {
	t.Helper()
	a, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	b, err := db.QueryNoteByPath("decisions/decision-example.md")
	require.NoError(t, err)
	require.NotNil(t, a)
	require.NotNil(t, b)
	return a.ID, b.ID
}
