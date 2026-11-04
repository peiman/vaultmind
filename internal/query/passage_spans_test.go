package query

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestPassageWindowsSmallCaps(t *testing.T) {
	body := "## Deprecated\n\n" + strings.Repeat("背景", 180) + "検索は重要である。"
	tokenStart := strings.Index(body, "背景")
	tokens := []passageToken{{term: body[tokenStart : len(body)-3], start: tokenStart, end: len(body) - 3}}
	for limit := 1; limit <= 160; limit++ {
		for _, w := range passageWindows(body, "", limit, tokens, []string{"検索"}) {
			text := w.deliver(body, tokens, []string{"検索"})
			assert.True(t, utf8.ValidString(text))
			assert.LessOrEqual(t, len(text), limit*4)
			assert.Contains(t, text, "## Deprecated")
		}
	}
	assert.False(t, passageCJK("NODE_TIMEOUT"))
	assert.False(t, passageCJK(""))
	assert.True(t, passageCJK("検索かなカナ한국"))
}
