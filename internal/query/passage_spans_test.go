package query

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestPassageSentenceBoundaries(t *testing.T) {
	previous := regexp.MustCompile(`[.!?][ \t]+|\n[ \t]*(?:[-*+] |[0-9]+\. )`)
	for _, text := range []string{"", "One. Two!\t Three?  End.", "A. B. C. D. E. ", "Number 1.23 intact.", "Intro\n  - bullet\n\t* next\n+ last", "Intro\n  123. bullet\n1. next", "No.\nEnd", "Intro\n -\n\t2.\n3", "Intro\n + odd. Next.", "背景検索は重要である。"} {
		want := []int{0}
		for _, loc := range previous.FindAllStringIndex(text, -1) {
			at := loc[1]
			if text[loc[0]] == '\n' {
				at = loc[0] + 1
			}
			want = append(want, at)
		}
		assert.Equal(t, want, passageSentenceStarts(text), "%q", text)
	}
}

func TestPassageWindowsSmallCaps(t *testing.T) {
	body := "## Deprecated\n\n" + strings.Repeat("背景", 180) + "検索は重要である。"
	tokenStart := strings.Index(body, "背景")
	tokens := []passageToken{{term: body[tokenStart : len(body)-3], start: tokenStart, end: len(body) - 3}}
	for limit := 1; limit <= 160; limit++ {
		for _, w := range passageWindows(body, limit, tokens, []string{"検索"}) {
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
