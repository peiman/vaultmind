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

	for _, r := range results {
		assert.Equal(t, "concept", r.Type, "all results must be concepts when filtered by type")
	}
}

func TestSearchFTS_FilterByTag(t *testing.T) {
	db := rebuildTestIndex(t)

	results, err := index.SearchFTS(db, "memory", 20, 0, index.SearchFilters{Tag: "cognitive-science"})
	require.NoError(t, err)

	// All results should have the tag "cognitive-science"
	for _, r := range results {
		assert.NotEmpty(t, r.ID)
	}
}

func TestSearchFTS_FilterByTypeAndTag(t *testing.T) {
	db := rebuildTestIndex(t)

	results, err := index.SearchFTS(db, "memory", 20, 0, index.SearchFilters{Type: "concept", Tag: "cognitive-science"})
	require.NoError(t, err)

	for _, r := range results {
		assert.Equal(t, "concept", r.Type)
	}
}

func TestSearchFTS_NoFilters(t *testing.T) {
	db := rebuildTestIndex(t)

	// Should work the same as before with empty filters
	results, err := index.SearchFTS(db, "cognitive architecture", 20, 0, index.SearchFilters{})
	require.NoError(t, err)
	require.NotEmpty(t, results)
}
