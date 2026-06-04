package index_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rebuildTestIndex returns a writable per-test DB seeded from the shared
// vault rebuild. Pre-2026-05-01 this re-ran NewIndexer.Rebuild() against
// the live `vaultmind-vault` (~400 notes) for every caller — ~5s per
// test, with ~30 tests in this package, dominating `task check` runtime
// (88% of total: 284s of ~322s on a representative run, blocking commits
// and pushing pathological runs past 1h). The shared-DB pattern
// (testvault.OpenSharedDB) builds the index ONCE per test process and
// gives each caller a copied, isolated, writable file. Same on-disk
// shape as before; same rebuild path; just amortised across the
// package.
//
// Any caller that mutates schema or content of the DB should keep using
// per-test isolation (which OpenSharedDB provides via copyFile); any
// caller that needs to test the rebuild path itself should NewIndexer +
// Rebuild directly rather than going through this helper.
func rebuildTestIndex(t *testing.T) *index.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db := testvault.OpenSharedDB(t, testVaultPath, dbPath)
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
		ids[i] = r.ID
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
		ids[i] = r.ID
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
		assert.NotEqual(t, limited[0].ID, offset[0].ID)
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

func TestSearchFTS_ScoresNormalized(t *testing.T) {
	db := rebuildTestIndex(t)
	results, err := index.SearchFTS(db, "memory", 100, 0)
	require.NoError(t, err)
	require.Greater(t, len(results), 1, "need multiple results to test normalization")

	for _, r := range results {
		assert.GreaterOrEqual(t, r.Score, 0.0, "score should be >= 0")
		assert.LessOrEqual(t, r.Score, 1.0, "score should be <= 1")
	}
	assert.Equal(t, 1.0, results[0].Score, "top result should have score 1.0")
	if len(results) > 1 {
		assert.Equal(t, 0.0, results[len(results)-1].Score, "worst result should have score 0.0")
	}
}

func TestSearchFTS_SingleResult_ScoreIsOne(t *testing.T) {
	db := rebuildTestIndex(t)
	results, err := index.SearchFTS(db, "Ebbinghaus", 100, 0)
	require.NoError(t, err)
	if len(results) == 1 {
		assert.Equal(t, 1.0, results[0].Score, "single result should have score 1.0")
	}
}

func TestCountFTS_ReturnsTotal(t *testing.T) {
	db := rebuildTestIndex(t)
	// Use a limit large enough that vault growth doesn't cap allResults below
	// CountFTS's true total. The previous limit of 100 silently truncated
	// once the research vault crossed 100 "memory" hits.
	allResults, err := index.SearchFTS(db, "memory", 10000, 0)
	require.NoError(t, err)
	totalExpected := len(allResults)
	require.Greater(t, totalExpected, 3, "need more than 3 results for this test")

	count, err := index.CountFTS(db, "memory")
	require.NoError(t, err)
	assert.Equal(t, totalExpected, count)

	limited, err := index.SearchFTS(db, "memory", 2, 0)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(limited), 2)

	count2, err := index.CountFTS(db, "memory")
	require.NoError(t, err)
	assert.Equal(t, totalExpected, count2)
}

func TestCountFTS_EmptyQuery(t *testing.T) {
	db := rebuildTestIndex(t)
	count, err := index.CountFTS(db, "")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestCountFTS_WithFilters(t *testing.T) {
	db := rebuildTestIndex(t)
	countAll, err := index.CountFTS(db, "memory")
	require.NoError(t, err)
	countFiltered, err := index.CountFTS(db, "memory", index.SearchFilters{Type: "concept"})
	require.NoError(t, err)
	assert.LessOrEqual(t, countFiltered, countAll)
}
