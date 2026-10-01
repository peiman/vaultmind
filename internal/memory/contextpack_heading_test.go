package memory_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Measured on sqlite.org/fts5: the matched section was delivered as its title
// alone, "7.1.1. Synonym Support", 153 of 1,500 tokens used. The excerpt starts
// at the section's prose; the heading travels in HeadingPath.
func TestContextPack_ASectionExcerptIsItsProseNotItsHeading(t *testing.T) {
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	var md strings.Builder
	md.WriteString("## 7.1. Custom Tokenizers\n\n" + strings.Repeat("Tokenizers split text. ", 400) + "\n\n")
	md.WriteString("### 7.1.1. Synonym Support\n\nThere are several ways to approach synonyms. " + strings.Repeat("More words here. ", 400) + "\n\n")
	for md.Len() < 9000*4 {
		md.WriteString("## Filler\n\n" + strings.Repeat("Filler text runs on. ", 400) + "\n\n")
	}
	rec := index.NoteRecord{ID: "ref-fts5", Path: "refs/fts5.md", Title: "FTS5", Type: "reference", BodyText: md.String(), Hash: "h", IsDomain: true}
	rec.Sections = index.SectionsFor(rec.BodyText, rec.BodyText)
	require.NoError(t, index.StoreNote(db, rec))

	res, err := memory.ContextPack(graph.NewResolver(db), db, memory.ContextPackConfig{
		Input: "ref-fts5", Budget: 1500, ExcerptTokens: 80,
		Sections: map[string]string{"ref-fts5": "ref-fts5#7-1-1-synonym-support"},
	})
	require.NoError(t, err)
	require.NotNil(t, res.Target)
	assert.Equal(t, "ref-fts5#7-1-1-synonym-support", res.Target.Section)
	assert.True(t, strings.HasPrefix(res.Target.Body, "There are several ways"), res.Target.Body)
}
