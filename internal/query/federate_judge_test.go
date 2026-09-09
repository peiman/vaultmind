package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingRetriever wraps a real retriever and counts Search calls, because
// "how many times did we search this vault?" is the property under test and
// no amount of reading the code proved it.
type countingRetriever struct {
	inner retrieval.Retriever
	calls int
}

func (c *countingRetriever) Search(ctx context.Context, q string, limit, offset int, f index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	c.calls++
	return c.inner.Search(ctx, q, limit, offset, f)
}

// flatFloor is a FloorResolver with fixed values — the calibration lookup is
// the caller's business, not this function's.
func flatFloor(floor, sigma float64) query.FloorResolver {
	return func(int) (float64, float64) { return floor, sigma }
}

// judgeFixture builds a DB with two notes whose embeddings are known, so a
// 3-dim mock embedder produces a real, checkable similarity.
func judgeFixture(t *testing.T) (*index.DB, *mockEmbedder) {
	t.Helper()
	db := buildRetrieverTestDB(t)
	row1, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row1)
	row2, err := db.QueryNoteByPath("concepts/episodic-memory.md")
	require.NoError(t, err)
	require.NotNil(t, row2)
	require.NoError(t, index.StoreEmbedding(db, row1.ID, []float32{1, 0, 0}))
	require.NoError(t, index.StoreEmbedding(db, row2.ID, []float32{0, 1, 0}))
	return db, &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3}
}

// ONE retrieval per vault, not two.
//
// The first federation searched every vault twice: once for the hits, once
// again inside the verdict pass that judges the vault against its own floor.
// Measured live on three vaults, that doubling cost 6.7 seconds and pushed a
// federated query to 15.9s — past the recall hook's 15s timeout, so the hook
// was killed and injected NOTHING while exiting 0. The feature worked at the
// CLI and was dead at the only place it mattered.
//
// Counting the searches is the only way this stays fixed: the duplicate was
// invisible in review precisely because both call sites read as reasonable.
func TestSearchAndJudge_SearchesTheVaultExactlyOnce(t *testing.T) {
	db, emb := judgeFixture(t)
	ret := &countingRetriever{inner: &query.FTSRetriever{DB: db}}

	hits, verdict, _, err := query.SearchAndJudge(
		context.Background(), ret, emb, db, flatFloor(0.1, 0.05), "spreading activation", 5)

	require.NoError(t, err)
	require.NotEmpty(t, hits, "fixture must return hits or the count below proves nothing")
	require.NotEmpty(t, verdict, "the verdict pass must actually have run, not been skipped")
	assert.Equal(t, 1, ret.calls,
		"a federated vault gets ONE retrieval; the verdict must reuse those hits, not search again")
}

// A vault with no embedder cannot be judged, and unmeasured must not be
// reported as a number — it is kept in the merge precisely because absence of
// a measurement is not evidence of irrelevance.
func TestSearchAndJudge_NoEmbedderIsUnmeasuredNotZeroRelevance(t *testing.T) {
	db, _ := judgeFixture(t)
	ret := &countingRetriever{inner: &query.FTSRetriever{DB: db}}

	hits, verdict, z, err := query.SearchAndJudge(
		context.Background(), ret, nil, db, flatFloor(0.1, 0.05), "spreading activation", 5)

	require.NoError(t, err)
	assert.NotEmpty(t, hits, "keyword search still works without an embedder")
	assert.Empty(t, verdict, "no embedder means no verdict — not a confident no_match")
	assert.Zero(t, z)
	assert.Equal(t, 1, ret.calls, "still exactly one retrieval")
}

// The verdict must come from the vault's OWN calibration. A federated
// judgement that disagreed with `ask --vault X` would make the gate arbitrary.
func TestSearchAndJudge_UsesTheSuppliedFloorNotADefault(t *testing.T) {
	db, emb := judgeFixture(t)
	ret := &countingRetriever{inner: &query.FTSRetriever{DB: db}}

	// Same query, same hits, two different calibrations. A floor far above the
	// top hit's cosine must judge it no_match; a floor far below must not.
	_, strictVerdict, strictZ, err := query.SearchAndJudge(
		context.Background(), ret, emb, db, flatFloor(0.99, 0.01), "spreading activation", 5)
	require.NoError(t, err)
	_, looseVerdict, looseZ, err := query.SearchAndJudge(
		context.Background(), ret, emb, db, flatFloor(-0.5, 0.01), "spreading activation", 5)
	require.NoError(t, err)

	assert.Greater(t, looseZ, strictZ,
		"the supplied floor must move the verdict; ignoring it would make both identical")
	assert.NotEqual(t, looseVerdict, strictVerdict)
}
