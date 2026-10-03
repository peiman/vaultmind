package query_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func overviewDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func addNote(t *testing.T, db *index.DB, path, typ string, mtime int64) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO notes (id, path, title, type, body_text, hash, mtime) VALUES (?, ?, ?, ?, '', 'h', ?)`,
		path, path, path, typ, mtime)
	require.NoError(t, err)
}

// A folder with many notes and no overview is named; a small one, the vault
// root and a folder that has an overview are not.
func TestCheckOverviews_NamesBigFoldersWithoutOne(t *testing.T) {
	db := overviewDB(t)
	for i := 0; i < 25; i++ {
		addNote(t, db, fmt.Sprintf("concepts/c%02d.md", i), "concept", 100)
		addNote(t, db, fmt.Sprintf("sources/s%02d.md", i), "source", 100)
		addNote(t, db, fmt.Sprintf("root%02d.md", i), "concept", 100)
	}
	addNote(t, db, "sources/overview.md", "overview", 200)
	for i := 0; i < 5; i++ {
		addNote(t, db, fmt.Sprintf("people/p%d.md", i), "person", 100)
	}

	h, err := query.CheckOverviews(db)
	require.NoError(t, err)
	assert.Equal(t, []query.FolderCount{{Folder: "concepts", Notes: 25}}, h.Missing)
	assert.Empty(t, h.Stale)
}

// An overview is stale once many of its folder's notes changed after it: at
// least 10, and at least a quarter of the folder.
func TestCheckOverviews_NamesAStaleOverview(t *testing.T) {
	db := overviewDB(t)
	addNote(t, db, "sources/overview.md", "overview", 100)
	for i := 0; i < 30; i++ {
		mtime := int64(50)
		if i < 12 {
			mtime = 500 // changed after the overview
		}
		addNote(t, db, fmt.Sprintf("sources/s%02d.md", i), "source", mtime)
	}
	addNote(t, db, "people/overview.md", "overview", 100)
	for i := 0; i < 20; i++ {
		mtime := int64(50)
		if i < 9 { // 9 of 20 changed: past the share, under the floor of 10
			mtime = 500
		}
		addNote(t, db, fmt.Sprintf("people/p%02d.md", i), "person", mtime)
	}
	addNote(t, db, "decisions/overview.md", "overview", 100)
	for i := 0; i < 60; i++ {
		mtime := int64(50)
		if i < 12 { // 12 of 60 changed: past the floor, under a quarter
			mtime = 500
		}
		addNote(t, db, fmt.Sprintf("decisions/d%02d.md", i), "decision", mtime)
	}

	h, err := query.CheckOverviews(db)
	require.NoError(t, err)
	assert.Equal(t, []query.StaleOverview{{Overview: "sources/overview.md", Changed: 12, Notes: 30}}, h.Stale)
}
