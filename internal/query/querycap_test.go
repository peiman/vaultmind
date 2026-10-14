package query_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCapRankingQuery pins the only ranking-query shape: the first N words.
// Dropping the slice (fields[:maxWords] → fields) or the off-switch
// (maxWords <= 0 → false) must fail this test.
func TestCapRankingQuery(t *testing.T) {
	const messy = "alpha   beta\n\tgamma"

	tests := []struct {
		name   string
		text   string
		max    int
		want   string
		capped *query.QueryCapped
	}{
		{
			name: "under the cap keeps the original text",
			text: messy,
			max:  10,
			want: messy,
		},
		{
			name: "exactly N words is not a cap",
			text: "one two three",
			max:  3,
			want: "one two three",
		},
		{
			name: "over the cap keeps the first N words",
			text: "one two three four",
			max:  2,
			want: "one two",
			capped: &query.QueryCapped{
				UsedWords:  2,
				TotalWords: 4,
			},
		},
		{
			name: "zero turns the cap off",
			text: "one two three four",
			max:  0,
			want: "one two three four",
		},
		{
			name: "a negative cap is off",
			text: "one two three",
			max:  -3,
			want: "one two three",
		},
		{
			name: "whitespace and newlines collapse only when capped",
			text: "one\n\ntwo   three\tfour",
			max:  3,
			want: "one two three",
			capped: &query.QueryCapped{
				UsedWords:  3,
				TotalWords: 4,
			},
		},
		{
			name: "non-ASCII words are counted as words",
			text: "café 東京 alpha beta",
			max:  2,
			want: "café 東京",
			capped: &query.QueryCapped{
				UsedWords:  2,
				TotalWords: 4,
			},
		},
		{
			name: "non-ASCII spaces split words",
			text: "one\u00a0two\u3000three four",
			max:  2,
			want: "one two",
			capped: &query.QueryCapped{
				UsedWords:  2,
				TotalWords: 4,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, report := query.CapRankingQuery(tt.text, tt.max)
			assert.Equal(t, tt.want, got)
			if tt.capped == nil {
				assert.Nil(t, report)
				return
			}
			require.NotNil(t, report)
			assert.Equal(t, tt.capped.UsedWords, report.UsedWords)
			assert.Equal(t, tt.capped.TotalWords, report.TotalWords)
		})
	}
}
