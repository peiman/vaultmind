package query_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMget_ReturnsMultipleNotes(t *testing.T) {
	db := buildIndexedDB(t)

	result, err := query.Mget(db, []string{"concept-act-r", "concept-rag"}, true)
	require.NoError(t, err)

	assert.Len(t, result.Notes, 2)
	assert.Empty(t, result.NotFound)
	assert.Equal(t, 2, result.Total)
}

func TestMget_TracksNotFound(t *testing.T) {
	db := buildIndexedDB(t)

	result, err := query.Mget(db, []string{"concept-act-r", "nonexistent-id"}, true)
	require.NoError(t, err)

	assert.Len(t, result.Notes, 1)
	assert.Contains(t, result.NotFound, "nonexistent-id")
	assert.Equal(t, 1, result.Total)
}

func TestMget_FrontmatterOnly(t *testing.T) {
	db := buildIndexedDB(t)

	result, err := query.Mget(db, []string{"concept-act-r"}, true)
	require.NoError(t, err)
	require.Len(t, result.Notes, 1)

	// Frontmatter-only: body should be empty
	assert.Empty(t, result.Notes[0].Body)
	assert.Empty(t, result.Notes[0].Headings)
}

func TestMget_WithBody(t *testing.T) {
	db := buildIndexedDB(t)

	result, err := query.Mget(db, []string{"concept-act-r"}, false)
	require.NoError(t, err)
	require.Len(t, result.Notes, 1)

	assert.NotEmpty(t, result.Notes[0].Body)
}

func TestMget_EmptyIDs(t *testing.T) {
	db := buildIndexedDB(t)

	result, err := query.Mget(db, []string{}, true)
	require.NoError(t, err)
	assert.Empty(t, result.Notes)
	assert.Equal(t, 0, result.Total)
}

func TestMget_AllNotFound(t *testing.T) {
	db := buildIndexedDB(t)

	result, err := query.Mget(db, []string{"nope-1", "nope-2"}, true)
	require.NoError(t, err)
	assert.Empty(t, result.Notes)
	assert.Len(t, result.NotFound, 2)
	assert.Equal(t, 0, result.Total)
}
