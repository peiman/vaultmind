package index_test

import (
	"strings"
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

// insertFTSNote adds a note the path-prefix tests can search for. The fixture
// vault has no folder whose name contains a LIKE wildcard.
func insertFTSNote(t *testing.T, db *index.DB, id, path, body string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO notes (id, path, title, type, body_text, hash, mtime) VALUES (?, ?, ?, 'note', ?, 'h', 0)`,
		id, path, id, body)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO fts_notes (note_id, title, body_text) VALUES (?, ?, ?)`, id, id, body)
	require.NoError(t, err)
}

func TestSearchFTS_FilterByPathPrefix(t *testing.T) {
	db := rebuildTestIndex(t)

	all, err := index.SearchFTS(db, "memory", 10000, 0)
	require.NoError(t, err)
	var outside bool
	for _, r := range all {
		if !strings.HasPrefix(r.Path, "concepts/") {
			outside = true
			break
		}
	}
	require.True(t, outside,
		"the fixture query must hit a note outside concepts/, or the filter asserts nothing")

	results, err := index.SearchFTS(db, "memory", 10000, 0, index.SearchFilters{PathPrefix: "concepts/"})
	require.NoError(t, err)
	require.NotEmpty(t, results,
		"the fixture must return hits under concepts/, or the loop below asserts nothing")
	for _, r := range results {
		assert.True(t, strings.HasPrefix(r.Path, "concepts/"),
			"%s was returned by a search filtered to concepts/ but its path is %s", r.ID, r.Path)
	}
	count, err := index.CountFTS(db, "memory", index.SearchFilters{PathPrefix: "concepts/"})
	require.NoError(t, err)
	assert.Equal(t, len(results), count, "the count query must apply the same path filter")
	assert.Less(t, count, len(all), "the path filter drops notes outside the prefix")

	// '_' is a LIKE wildcard. A prefix that contains one must match that
	// character, not any character: a_b/ matches, aXb/ does not.
	const probe = "literalprefixprobe"
	insertFTSNote(t, db, "path-prefix-ab", "a_b/x.md", probe)
	insertFTSNote(t, db, "path-prefix-axb", "aXb/y.md", probe)

	literal, err := index.SearchFTS(db, probe, 20, 0, index.SearchFilters{PathPrefix: "a_b/"})
	require.NoError(t, err)
	require.Len(t, literal, 1, "prefix a_b/ must not match aXb/ as a wildcard")
	assert.Equal(t, "a_b/x.md", literal[0].Path)
	literalCount, err := index.CountFTS(db, probe, index.SearchFilters{PathPrefix: "a_b/"})
	require.NoError(t, err)
	assert.Equal(t, 1, literalCount)

	other, err := index.SearchFTS(db, probe, 20, 0, index.SearchFilters{PathPrefix: "aXb/"})
	require.NoError(t, err)
	require.Len(t, other, 1, "aXb/ is indexed, so a miss above is the escape and not a missing row")
	assert.Equal(t, "aXb/y.md", other[0].Path)
}

func TestSearchFTS_NoFilters(t *testing.T) {
	db := rebuildTestIndex(t)

	// Should work the same as before with empty filters
	results, err := index.SearchFTS(db, "cognitive architecture", 20, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.NotEmpty(t, results)
}
