package memory_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPinned_ClosedDBErrors(t *testing.T) {
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = memory.Pinned(db, memory.PinnedConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listing pinned notes")
}

func TestPinned_ANoteThatCannotBeLoadedNamesTheID(t *testing.T) {
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`INSERT INTO notes (id, path, title, type, body_text, hash, mtime) VALUES ('note-a', 'a.md', 'Alpha', 'concept', 'body', 'h', 0)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO frontmatter_kv (note_id, key, value_json) VALUES ('note-a', 'pinned', 'true')`)
	require.NoError(t, err)
	_, err = db.Exec(`DROP TABLE aliases`)
	require.NoError(t, err)

	_, err = memory.Pinned(db, memory.PinnedConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `pinned: loading "note-a"`)
}
