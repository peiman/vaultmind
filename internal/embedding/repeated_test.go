package embedding

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollapseRepeatedNonAlnum(t *testing.T) {
	require.Equal(t, 16, RepeatedNonAlnumCap)
	dots16 := strings.Repeat(".", 16)
	dots17 := strings.Repeat(".", 17)
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
		{name: "two long runs", in: dots17 + strings.Repeat("=", 20), want: dots16 + strings.Repeat("=", 16)},
		{name: "mixed characters under the cap", in: ".-*=~#+!?:;", want: ".-*=~#+!?:;"},
		{name: "unicode punctuation run of 17", in: strings.Repeat("…", 17), want: strings.Repeat("…", 16)},
		{name: "unicode punctuation run of 16", in: strings.Repeat("。", 16), want: strings.Repeat("。", 16)},
		{name: "letters never touched", in: strings.Repeat("a", 40) + strings.Repeat("日", 20), want: strings.Repeat("a", 40) + strings.Repeat("日", 20)},
		{name: "digits never touched", in: strings.Repeat("0", 30), want: strings.Repeat("0", 30)},
		{name: "spaces", in: "a" + strings.Repeat(" ", 19) + "b", want: "a" + strings.Repeat(" ", 16) + "b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CollapseRepeatedNonAlnum(tc.in)
			assert.Equal(t, tc.want, got)
			if tc.in == tc.want {
				assert.Equal(t, tc.in, got)
			}
		})
	}
}
