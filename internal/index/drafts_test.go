package index_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDraftIDs_IsTheStatusDraftSet(t *testing.T) {
	db := openTestDB(t)
	_, err := db.Exec(`INSERT INTO notes (id, path, hash, mtime, status) VALUES (?, ?, ?, ?, ?)`,
		"draft-1", "d.md", "h", 1, index.StatusDraft)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, status) VALUES (?, ?, ?, ?, ?)`,
		"live-1", "l.md", "h", 1, "active")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime) VALUES (?, ?, ?, ?)`,
		"blank-1", "b.md", "h", 1)
	require.NoError(t, err)

	set, err := index.DraftIDs(db)
	require.NoError(t, err)
	assert.True(t, set.Contains("draft-1"))
	assert.False(t, set.Contains("live-1"))
	assert.False(t, set.Contains("blank-1"), "a missing status is not a draft")
	assert.False(t, set.Visible("draft-1", false))
	assert.True(t, set.Visible("draft-1", true), "the retrieval opt-in delivers the draft")
	assert.True(t, set.Visible("live-1", false))

	empty, err := index.DraftIDs(nil)
	require.NoError(t, err)
	assert.False(t, empty.Contains("draft-1"))
	assert.True(t, empty.Visible("draft-1", false), "an unloaded set hides nothing")

	require.NoError(t, db.Close())
	_, err = index.DraftIDs(db)
	require.Error(t, err)
}
