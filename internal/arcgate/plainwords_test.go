package arcgate

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckPlainWords_OneRuleAtATime(t *testing.T) {
	baseline := "I recommend the smaller change. It keeps the decision clear."
	tests := []struct {
		name string
		msg  string
		want []string
	}{
		{name: "word count", msg: "I recommend this change. " + strings.Repeat("word ", 251), want: []string{"255 words, over 250"}},
		{name: "decision cue", msg: "The change is small. It is easy to review.", want: []string{"no decision in the first two sentences"}},
		{name: "jargon", msg: "I recommend parseConfig and HookRunner now. It keeps the decision clear.", want: []string{"18% of words are code-like, over 8%"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, CheckPlainWords(tc.msg))
		})
	}
	assert.Empty(t, CheckPlainWords(baseline))
}

func TestCheckPlainWords_WordCountBoundary(t *testing.T) {
	for _, n := range []int{250, 251} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			msg := "I recommend this. " + strings.Repeat("word ", n-5) + "done now."
			flags := CheckPlainWords(msg)
			if n == 250 {
				assert.NotContains(t, flags, "250 words, over 250")
			} else {
				assert.Contains(t, flags, "251 words, over 250")
			}
		})
	}
}

func TestCheckPlainWords_JargonBoundary(t *testing.T) {
	base := "I recommend this. " + strings.Repeat("plain ", 89)
	assert.NotContains(t, CheckPlainWords(base+strings.Repeat("parseConfig ", 8)), "8% of words are code-like, over 8%")
	assert.Contains(t, CheckPlainWords(base+strings.Repeat("parseConfig ", 9)), "9% of words are code-like, over 8%")
}

func TestCheckPlainWords_EmptyMessagePasses(t *testing.T) {
	assert.Empty(t, CheckPlainWords(" \n\t"))
}

func TestCheckPlainWords_ImperativeAndListCues(t *testing.T) {
	assert.Empty(t, CheckPlainWords("The change is small. Do ship it today."))
	assert.Empty(t, CheckPlainWords("- Choose the smaller change.\n- Keep the output short."))
}

func TestIsCodeLike_AllCapsAllowlistAndShortWordsArePlain(t *testing.T) {
	for _, token := range []string{"OK", "PR", "CI", "API", "US", "HTTPS"} {
		assert.False(t, isCodeLike(token), token)
	}
}

func TestCheckPlainWords_AllCapsAllowlistDoesNotTriggerJargon(t *testing.T) {
	msg := "I recommend OK PR CI API US HTTPS. It keeps the decision clear."
	assert.NotContains(t, strings.Join(CheckPlainWords(msg), "\n"), "code-like")
}

func TestCheckPlainWords_CodeLikeFormsStillTriggerJargon(t *testing.T) {
	msg := "I recommend SSOT HookRunner parseConfig snake_case. It keeps the decision clear."
	assert.Contains(t, CheckPlainWords(msg), "36% of words are code-like, over 8%")
}
