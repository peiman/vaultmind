package memory

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isCitation exists because isProse accepted a bare path as "the decision rule":
// a filename contains dots, and dots are sentence enders, so
// `~/.claude/projects/…/….jsonl` scored as a sentence and got delivered as an
// excerpt.
//
// The rejection works. What did not work is what happens NEXT: leadParagraph
// keeps the first non-prose block as a fallback so a bullet-list note never
// excerpts to nothing — and it kept citations too. A note that opens with a
// bare URL therefore delivered the URL anyway, with the bullet list that
// followed passed over. The check rejected the citation and the fallback
// handed it back.
func TestExcerpt_ALeadingBareURLLosesToRealContent(t *testing.T) {
	body := "https://example.com/papers/anderson-1991-act-r.pdf\n\n" +
		"- spreading activation decays with fan\n" +
		"- base-level activation decays with time"

	got := Excerpt(body, 60)

	assert.NotContains(t, got, "https://",
		"the URL was correctly rejected as prose, then reinstated as the fallback")
	assert.Contains(t, got, "spreading activation",
		"the bullet list is the content; a citation is a pointer to content")
}

// The invariant still holds: rejecting a citation must never empty a note whose
// ONLY content is one. A pointer beats nothing.
func TestExcerpt_ACitationOnlyNoteStillDeliversTheCitation(t *testing.T) {
	body := "https://example.com/papers/anderson-1991-act-r.pdf"

	got := Excerpt(body, 60)

	require.NotEmpty(t, got,
		"a non-empty body must never excerpt to nothing — that turns a cap into content loss")
	assert.Contains(t, got, "example.com")
}

// "no whitespace and contains a slash" is a description of a file path only in
// languages that space-separate words. Japanese, Chinese and Korean prose has
// no spaces, so any CJK line containing a slash was classified as a citation
// and dropped — in a tool whose sentence-ender constant was deliberately
// extended to "。！？" precisely so CJK notes would work.
func TestExcerpt_CJKProseWithASlashIsNotACitation(t *testing.T) {
	body := "検索/取得は記憶の中心である\n\n- 補足事項"

	got := Excerpt(body, 60)

	assert.Contains(t, got, "検索/取得",
		"a slash inside CJK prose is punctuation, not a path separator")
}

// The original defect, still guarded: a bare path must not be served as though
// it were the note's decision rule.
func TestExcerpt_BarePathIsStillRejected(t *testing.T) {
	body := "~/.claude/projects/-Users-peiman/788d34b2.jsonl\n\n" +
		"The transcript is the source of truth for every arc in this vault."

	got := Excerpt(body, 60)

	assert.Contains(t, got, "source of truth")
	assert.NotContains(t, got, ".jsonl",
		"a filename full of dots is not a sentence, however many sentence enders it contains")
}

func TestIsCitation_Table(t *testing.T) {
	cases := []struct {
		text string
		want bool
		why  string
	}{
		{"https://example.com/a", true, "a bare URL"},
		{"~/.claude/x/y.jsonl", true, "a bare path"},
		{"internal/query/format.go", true, "a repo-relative path"},
		{"see https://example.com/a", false, "a sentence that mentions a URL is prose"},
		{"検索/取得は記憶の中心である", false, "CJK prose has no spaces; the slash is not a separator"},
		{"记忆/检索", false, "same for Chinese"},
		{"no-slash-here", false, "without a slash there is nothing path-shaped about it"},
		{"a/b c/d", false, "whitespace means it is a phrase, not a single token"},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			assert.Equal(t, tc.want, isCitation(tc.text), tc.why)
		})
	}
}

// Guard against the fallback quietly widening again: whatever leadParagraph
// returns for a mixed body, it must not be the citation while real prose sits
// further down.
func TestLeadParagraph_PrefersProseOverEverythingElse(t *testing.T) {
	body := strings.Join([]string{
		"docs/reviews/session-07/summary.md",
		"- a bullet",
		"This is the actual sentence that carries the rule.",
	}, "\n\n")

	assert.Equal(t, "This is the actual sentence that carries the rule.", leadParagraph(body))
}
