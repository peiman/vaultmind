package memory_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
)

func TestPrefixExcerpt(t *testing.T) {
	tests := []struct {
		name string
		body string
		cap  int
		want string
	}{
		{name: "empty", cap: 400},
		{name: "whitespace", body: " \n\t", cap: 400},
		{name: "zero budget", body: "Intro.", cap: 0},
		{name: "negative budget", body: "Intro.", cap: -1},
		{name: "metadata and comments", body: "---\nid: ledger\n---\n<!-- hidden -->\nIntro.\n\n## Now\n- item A<!-- private -->\n", cap: 400, want: "Intro.\n\n## Now\n- item A"},
		{name: "comments only", body: "<!-- hidden -->", cap: 400},
		{name: "line boundary", body: "Intro.\n## Now\n- item A\n- item B", cap: 6, want: "Intro.\n## Now\n- item A"},
		{name: "first line too long", body: "abcdefghijklmno\n## Now", cap: 3, want: "abcdefghi…"},
		{name: "first line keeps spaces", body: "one  two  three four", cap: 4, want: "one  two  thr…"},
		{name: "unicode boundary", body: "研究は重要である。", cap: 3, want: "研究は…"},
		{name: "no room for content and ellipsis", body: "研究は重要である。", cap: 1},
		{name: "closed block fits", body: "```go\nx := 1\n```", cap: 4, want: "```go\nx := 1\n```"},
		{name: "close cut block within cap", body: "Intro.\n```go\nx := 1\ny := 2\n```\nAfter.", cap: 7, want: "Intro.\n```go\nx := 1\n```"},
		{name: "close already unclosed block", body: "```\nx", cap: 20, want: "```\nx\n```"},
		{name: "opening fence cannot fit", body: "Intro.\n```go\nx := 1\n```", cap: 3, want: "Intro."},
		{name: "first fence cannot fit", body: "```go\nx := 1\n```", cap: 1},
		{name: "long first fence", body: "```language-name\nx\n```", cap: 4, want: "```langua…\n```"},
		{name: "longer fence", body: "````markdown\n```\ntext\n````\nAfter.", cap: 6, want: "````markdown\n```\n````"},
		{name: "backticks in code do not close", body: "```\n```not a closer\nmore\n```", cap: 6, want: "```\n```not a closer\n```"},
		{name: "inline backticks", body: "Use ``` inline.\n## Now", cap: 20, want: "Use ``` inline.\n## Now"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := memory.PrefixExcerpt(tt.body, tt.cap)
			assert.Equal(t, tt.want, got)
			assert.True(t, utf8.ValidString(got))
			if tt.cap > 0 {
				assert.LessOrEqual(t, memory.EstimateTokens(got), tt.cap)
			}
		})
	}
}

func TestPrefixExcerpt_LedgerKeepsHeadingsAndLists(t *testing.T) {
	body := "Intro line.\n\n## Now\n- item A\n- item B\n\n## Later\n" + strings.Repeat("- Future task.\n", 200)
	got := memory.PrefixExcerpt(body, 400)
	assert.True(t, strings.HasPrefix(got, "Intro line.\n\n## Now\n- item A\n- item B\n\n## Later\n"))
	assert.LessOrEqual(t, memory.EstimateTokens(got), 400)
	assert.True(t, strings.HasSuffix(got, "- Future task."), "keep the last whole line")
}
