package index_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchFTS_FilterByType(t *testing.T) {
	db := rebuildTestIndex(t)

	// "memory" appears in many note types. Filter to concepts only.
	results, err := index.SearchFTS(db, "memory", 20, 0, index.SearchFilters{Type: "concept"})
	require.NoError(t, err)
	require.NotEmpty(t, results,
		"the fixture must return concept hits, or the loop below asserts nothing")

	for _, r := range results {
		assert.Equal(t, "concept", r.Type, "all results must be concepts when filtered by type")
	}
}

// The tag-filter tests below check the tag. That sounds like a tautology; it
// was not true until 2026-08-20. The comment said "all results should have the
// tag" and the assertion said `NotEmpty(r.ID)` — true of every row the query
// could possibly return, under any filter or none. Deleting the tag clause from
// the production SQL at both call sites left this test green.
//
// Two separate holes, and closing one without the other leaves the test hollow:
// the loop asserted nothing about tags, AND an empty result set skipped the loop
// entirely. So each test now guards the fixture produced hits, and then checks
// the property the filter exists to provide.
const filterTag = "cognitive-science"

// filterTagNarrow exists because filterTag cannot prove anything once the type
// filter is also on: every concept note in the fixture matching "memory" carries
// cognitive-science, so removing the tag clause changes the result set not at
// all, and the test passes either way. memory-systems is carried by some of
// those concepts and not others, which is the property that makes the combined
// filter observable. A fixture where the broken and the fixed implementation
// agree is not a weaker test — it is not a test.
const filterTagNarrow = "memory-systems"

func assertCarriesTag(t *testing.T, db *index.DB, id, tag string) {
	t.Helper()

	var n int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM tags WHERE note_id = ? AND tag = ?`, id, tag).Scan(&n))
	assert.Positive(t, n,
		"%s was returned by a search filtered to tag %q but does not carry it", id, tag)
}

func TestSearchFTS_FilterByTag(t *testing.T) {
	db := rebuildTestIndex(t)

	results, err := index.SearchFTS(db, "memory", 20, 0, index.SearchFilters{Tag: filterTag})
	require.NoError(t, err)
	require.NotEmpty(t, results,
		"the fixture must return tagged hits, or the loop below asserts nothing")

	for _, r := range results {
		assertCarriesTag(t, db, r.ID, filterTag)
	}
}

func TestSearchFTS_FilterByTypeAndTag(t *testing.T) {
	db := rebuildTestIndex(t)

	results, err := index.SearchFTS(db, "memory", 20, 0, index.SearchFilters{Type: "concept", Tag: filterTagNarrow})
	require.NoError(t, err)
	require.NotEmpty(t, results,
		"the fixture must return hits matching both filters, or the loop below asserts nothing")

	for _, r := range results {
		assert.Equal(t, "concept", r.Type)
		assertCarriesTag(t, db, r.ID, filterTagNarrow)
	}
}

func TestSearchFTS_NoFilters(t *testing.T) {
	db := rebuildTestIndex(t)

	// Should work the same as before with empty filters
	results, err := index.SearchFTS(db, "cognitive architecture", 20, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.NotEmpty(t, results)
}
