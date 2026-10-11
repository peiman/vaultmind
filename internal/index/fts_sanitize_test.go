package index_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchFTS_AnyQueryWordRanksMoreMatchesFirst(t *testing.T) {
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	// Equal-length notes isolate word coverage from BM25's length penalty.
	for _, note := range []struct{ id, body string }{
		{"more", "memory consolidation sleep"},
		{"fewer", "memory unrelated filler"},
		{"unrelated", "unrelated filler content"},
	} {
		_, err := db.Exec(`INSERT INTO notes (id, path, title, body_text, hash, mtime)
			VALUES (?, ?, ?, ?, 'h', 0)`, note.id, note.id+".md", note.id, note.body)
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO fts_notes (note_id, title, body_text) VALUES (?, ?, ?)`,
			note.id, note.id, note.body)
		require.NoError(t, err)
	}

	query := "how does memory consolidation work during sleep"
	results, err := index.SearchFTS(db, query, 10, 0)
	require.NoError(t, err)
	require.Len(t, results, 2, "notes matching only some question words must be retrieved")
	assert.Equal(t, "more", results[0].ID)
	assert.Equal(t, "fewer", results[1].ID)
	assert.Greater(t, results[0].Score, results[1].Score)

	count, err := index.CountFTS(db, query)
	require.NoError(t, err)
	assert.Equal(t, len(results), count, "pagination counts use the same any-word semantics")
}

func TestSearchFTS_SpecialCharsDoNotCrash(t *testing.T) {
	db := rebuildTestIndex(t)

	tests := []string{
		`"unclosed quote`,
		`(unclosed paren`,
		`colon:in:query`,
		`dash-in-query`,
		`AND OR NOT`,
		`*wildcard`,
		`query with "quotes"`,
	}

	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			results, err := index.SearchFTS(db, q, 10, 0)
			require.NoError(t, err, "query %q must not crash", q)
			_ = results // may be empty, that's fine
		})
	}
}

func TestSearchFTS_UserSyntaxRemainsLiteral(t *testing.T) {
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	insertFTSNote(t, db, "more", "more.md", "memory consolidation sleep")
	insertFTSNote(t, db, "fewer", "fewer.md", "memory unrelated filler")
	insertFTSNote(t, db, "operators", "operators.md", "AND OR NOT NEAR")
	insertFTSNote(t, db, "prefix", "prefix.md", "memorybank unrelated filler")

	for _, tt := range []struct {
		query string
		ids   []string
	}{
		{"AND", []string{"operators"}},
		{"OR", []string{"operators"}},
		{"NOT", []string{"operators"}},
		{"memory AND consolidation", []string{"more", "fewer", "operators"}},
		{"memory NOT consolidation", []string{"more", "fewer", "operators"}},
		{`memory" OR "consolidation`, []string{"more", "fewer", "operators"}},
		{"memory*", []string{"more", "fewer"}},
		{"body_text:memory", nil},
	} {
		t.Run(tt.query, func(t *testing.T) {
			results, err := index.SearchFTS(db, tt.query, 10, 0)
			require.NoError(t, err)
			var ids []string
			for _, result := range results {
				ids = append(ids, result.ID)
			}
			assert.ElementsMatch(t, tt.ids, ids)
		})
	}
}
