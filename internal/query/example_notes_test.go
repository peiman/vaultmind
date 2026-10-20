package query_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListExampleNotes_NamesNotesTaggedExample(t *testing.T) {
	db := buildRetrieverTestDB(t)
	var id, path string
	require.NoError(t, db.QueryRow(`SELECT id, path FROM notes ORDER BY path LIMIT 1`).Scan(&id, &path))
	_, err := db.Exec(`INSERT INTO tags (note_id, tag) VALUES (?, ?)`, id, "example")
	require.NoError(t, err)

	got, err := query.ListExampleNotes(db)
	require.NoError(t, err)
	require.Equal(t, 1, got.Count)
	require.Len(t, got.Notes, 1)
	assert.Equal(t, id, got.Notes[0].ID)
	assert.Equal(t, path, got.Notes[0].Path)
}

func TestListExampleNotes_EmptyWhenNothingIsTagged(t *testing.T) {
	db := buildRetrieverTestDB(t)
	got, err := query.ListExampleNotes(db)
	require.NoError(t, err)
	assert.Equal(t, 0, got.Count)
	assert.Empty(t, got.Notes)
}

func TestListExampleNotes_DatabaseFailureIsReported(t *testing.T) {
	db := buildRetrieverTestDB(t)
	require.NoError(t, db.Close())
	_, err := query.ListExampleNotes(db)
	require.ErrorContains(t, err, "listing example notes")
}
