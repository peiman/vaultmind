package embedding

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapSymbolRuns(t *testing.T) {
	require.Equal(t, 16, RepeatedNonAlnumCap)
	dots16 := strings.Repeat(".", 16)
	dots17 := strings.Repeat(".", 17)
	mixed16 := strings.Repeat(".-", 8)
	mixed18 := strings.Repeat("-=", 9)
	longRule := strings.Repeat("|---", 5) + "|"
	require.Equal(t, 16, utf8.RuneCountInString(mixed16))
	require.Greater(t, utf8.RuneCountInString(longRule), RepeatedNonAlnumCap)
	require.LessOrEqual(t, utf8.RuneCountInString("|---|:---:|"), RepeatedNonAlnumCap)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "short query", in: "vaultmind", want: "vaultmind"},
		{name: "normal sentence", in: "where did we park the tokenizer pin", want: "where did we park the tokenizer pin"},
		{name: "run of 16", in: "end" + dots16 + " next", want: "end" + dots16 + " next"},
		{name: "run of 17", in: dots17, want: dots16},
		{name: "run of 17 between letters", in: "aa" + dots17 + "bb", want: "aa" + dots16 + "bb"},
		{name: "adjacent different symbols are one run", in: dots17 + strings.Repeat("=", 20), want: dots16},
		{name: "mixed characters under the cap", in: ".-*=~#+!?:;", want: ".-*=~#+!?:;"},
		{name: "mixed dot hyphen of 16", in: mixed16, want: mixed16},
		{name: "mixed dot hyphen of 17", in: mixed16 + "-", want: mixed16},
		{name: "mixed equals hyphen over the cap", in: "pre" + mixed18 + "post", want: "pre" + mixed18[:16] + "post"},
		{name: "alternating punctuation between words", in: "a" + strings.Repeat(".-", 12) + "b" + strings.Repeat("-=", 10), want: "a" + mixed16 + "b" + strings.Repeat("-=", 8)},
		{name: "unicode punctuation run of 17", in: strings.Repeat("…", 17), want: strings.Repeat("…", 16)},
		{name: "unicode punctuation run of 16", in: strings.Repeat("。", 16), want: strings.Repeat("。", 16)},
		{name: "unicode mixed punctuation over the cap", in: strings.Repeat("…—", 9), want: string([]rune(strings.Repeat("…—", 9))[:16])},
		{name: "letters never touched", in: strings.Repeat("a", 40) + strings.Repeat("日", 20), want: strings.Repeat("a", 40) + strings.Repeat("日", 20)},
		{name: "digits never touched", in: strings.Repeat("0", 30), want: strings.Repeat("0", 30)},
		{name: "spaces never touched", in: "a" + strings.Repeat(" ", 40) + "b", want: "a" + strings.Repeat(" ", 40) + "b"},
		{name: "newlines and tabs never touched", in: strings.Repeat("\n", 30) + strings.Repeat("\t", 20), want: strings.Repeat("\n", 30) + strings.Repeat("\t", 20)},
		{name: "nbsp never touched", in: strings.Repeat("\u00a0", 20), want: strings.Repeat("\u00a0", 20)},
		{name: "space breaks a symbol run", in: strings.Repeat(".", 20) + " " + strings.Repeat("-", 20), want: dots16 + " " + strings.Repeat("-", 16)},
		{name: "url", in: "https://example.com/path/to/note?q=1&x=2#section", want: "https://example.com/path/to/note?q=1&x=2#section"},
		{name: "file path", in: "/usr/local/bin/vaultmind-oss/repeated.go", want: "/usr/local/bin/vaultmind-oss/repeated.go"},
		{name: "not equal", in: "a !== b", want: "a !== b"},
		{name: "arrow", in: "ptr->field", want: "ptr->field"},
		{name: "short markdown table rule", in: "|---|:---:|", want: "|---|:---:|"},
		{name: "spaced markdown table rule", in: "| --- | --- | --- | --- |", want: "| --- | --- | --- | --- |"},
		{name: "long markdown table rule", in: longRule, want: string([]rune(longRule)[:16])},
		{name: "dash rule longer than the cap", in: strings.Repeat("-", 24), want: strings.Repeat("-", 16)},
		{name: "emoji sequence", in: "👍🎉🔥✨", want: "👍🎉🔥✨"},
		{name: "zwj emoji sequence", in: "👨‍👩‍👧‍👦", want: "👨‍👩‍👧‍👦"},
		{name: "emoji run of 16", in: strings.Repeat("🎉", 16), want: strings.Repeat("🎉", 16)},
		{name: "emoji run of 17", in: strings.Repeat("🎉", 17), want: strings.Repeat("🎉", 16)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CapSymbolRuns(tc.in)
			assert.Equal(t, tc.want, got)
			if tc.in == tc.want {
				assert.Equal(t, tc.in, got)
			}
		})
	}
}
