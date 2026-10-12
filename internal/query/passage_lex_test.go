package query

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Vocabulary reuse must produce exactly the keyword tokenizer's complete
// stream, including repeated, accented, case-folded and identifier tokens.
func TestPassageVocabularyMatchesKeywordTokenizer(t *testing.T) {
	db, err := index.Open(filepath.Join(t.TempDir(), "lex.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, raw := range []string{
		"CAFÉ cafe Café résumé NODE_TIMEOUT NODE_TIMEOUT `text_factory`.",
		"cafe\u0301 café cafe\u0301 Ελληνικά ОТКАЗ \ue000token.",
		"## Heading\n\n[docs](https://hidden.example) [[destination|alias]] **bold** _italic_ ![[embed]]\n```go\nignored\n```\nMore docs.",
		"背景背景 検索 かなカナ 한국어 背景背景",
	} {
		text, _ := parser.StripForFTSOffsets(raw)
		bounds := passageTokenBounds(text)
		vocabulary, ordinals, distinct := passageVocabulary(text, bounds)
		compressed, err := passageLex(db, "cafe", vocabulary, "Opening.")
		require.NoError(t, err)
		full, err := passageLex(db, "cafe", text, "Opening.")
		require.NoError(t, err)
		require.Len(t, compressed.body, distinct)
		var expanded []string
		for _, at := range ordinals {
			expanded = append(expanded, compressed.body[at])
		}
		assert.Equal(t, full.body, expanded, "%q", raw)
	}
}
