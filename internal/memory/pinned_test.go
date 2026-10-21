package memory_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPinned_ExcerptPreservesOpeningSectionsRegardlessOfType(t *testing.T) {
	for _, noteType := range []string{"concept", "arc"} {
		t.Run(noteType, func(t *testing.T) {
			db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			body := "Intro line.\n\n## Now\n- item A\n- item B\n\n## Later\n" + strings.Repeat("- Future task.\n", 200) + "\n## Principle\nPrefer the decision rule."
			require.Greater(t, memory.EstimateTokens(body), 400)
			_, err = db.Exec(`INSERT INTO notes (id, path, title, type, body_text, hash, mtime) VALUES ('ledger', 'ledger.md', 'Ledger', ?, ?, 'h', 0)`, noteType, body)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO frontmatter_kv (note_id, key, value_json) VALUES ('ledger', 'pinned', 'true')`)
			require.NoError(t, err)

			got, err := memory.Pinned(db, memory.PinnedConfig{ExcerptTokens: 400, Budget: 400})
			require.NoError(t, err)
			require.Len(t, got.Items, 1)
			assert.True(t, strings.HasPrefix(got.Items[0].Excerpt, "Intro line.\n\n## Now\n- item A\n- item B"))
			assert.LessOrEqual(t, memory.EstimateTokens(got.Items[0].Excerpt), 400)
			assert.Equal(t, memory.EstimateTokens(got.Items[0].Excerpt), got.Tokens)
			assert.Empty(t, got.Omitted)

			recited, err := memory.Recite(db, memory.ReciteConfig{Type: noteType, ExcerptTokens: 400})
			require.NoError(t, err)
			require.Len(t, recited.Items, 1)
			assert.Equal(t, "Prefer the decision rule.", recited.Items[0].Excerpt)
		})
	}
}

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
