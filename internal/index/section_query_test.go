package index_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// One section by id: its note, heading path and text. A stale section (its
// note re-indexed by an older binary) or an unknown id is not found — never
// a part of a note's old text served as current.
func TestQuerySection(t *testing.T) {
	db := openTestDB(t)
	require.NoError(t, index.StoreNote(db, longRecord("ref-long", "refs/long.md", "Alpha", "Beta")))

	s, err := index.QuerySection(db, "ref-long#beta")
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "ref-long#beta", s.ID)
	assert.Equal(t, "ref-long", s.NoteID)
	assert.Equal(t, "Beta", s.HeadingPath)
	assert.NotEmpty(t, s.Body)

	s, err = index.QuerySection(db, "ref-long#no-such-anchor")
	require.NoError(t, err)
	assert.Nil(t, s)

	_, err = db.Exec(`UPDATE notes SET hash = 'edited-elsewhere' WHERE id = 'ref-long'`)
	require.NoError(t, err)
	s, err = index.QuerySection(db, "ref-long#beta")
	require.NoError(t, err)
	assert.Nil(t, s, "a stale section is not served")
}
