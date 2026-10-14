package query_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// weightAnalyzer is a stub embedder (ADR-003). It hands back the sparse
// weights the test supplies. No mock.
type weightAnalyzer struct {
	tokens embedding.QueryTokens
	err    error
	calls  int
	seen   string
}

func (a *weightAnalyzer) AnalyzeQuery(_ context.Context, text string) (embedding.QueryTokens, error) {
	a.calls++
	a.seen = text
	return a.tokens, a.err
}

// TestShapeQuery_KeepsTheHighestWeightsInOrder pins salient selection.
// Flipping the comparison (a > b → a < b) must fail this test: the kept
// tokens and the FTS words would be the lowest weights.
func TestShapeQuery_KeepsTheHighestWeightsInOrder(t *testing.T) {
	// Tokens, in order: alpha 0.2, beta 9, gamma 0.1, alpha again 0.5.
	// The two highest are beta (index 1) and the second alpha (index 3).
	// Ties on a weight keep the earlier index.
	analyzer := &weightAnalyzer{tokens: embedding.QueryTokens{
		Weights: []float32{0.2, 9, 0.1, 0.5},
		Word:    []int{0, 1, 2, 0},
		Words:   []string{"alpha", "beta", "gamma"},
	}}
	const original = "alpha   beta\ngamma alpha"

	shaped := query.ShapeQuery(context.Background(), original, 0, 2, analyzer)

	assert.Equal(t, []int{1, 3}, shaped.Keep, "highest weights, original token order")
	assert.Equal(t, "alpha beta", shaped.FTSText, "highest words, first-occurrence order")
	assert.Equal(t, original, shaped.EmbedText, "salient selection does not shorten the embedded text")
	assert.Equal(t, 1, analyzer.calls)
	assert.Equal(t, original, analyzer.seen)
	require.NotNil(t, shaped.Report)
	assert.Equal(t, 4, shaped.Report.EmbeddedWords)
	assert.Equal(t, 4, shaped.Report.TotalWords)
	assert.Equal(t, 2, shaped.Report.SalientTerms)

	tied := query.SelectSalientTokenIndices([]float32{5, 5, 1}, 1)
	assert.Equal(t, []int{0}, tied, "an equal weight keeps the earlier token")

	// Under the count, nothing is dropped: nil means the lane uses every token.
	assert.Nil(t, query.SelectSalientTokenIndices([]float32{1, 9, 3}, 3))
	assert.Nil(t, query.SelectSalientTokenIndices([]float32{1, 9, 3}, 4))
	assert.Nil(t, query.SelectSalientWords(
		[]string{"alpha", "beta"}, []int{0, 1}, []float32{1, 9}, 2))
}

// TestForwardQuery_AppliesTheWordBound pins the forward cut.
// Skipping it (fields[:maxWords] → fields) must fail this test.
func TestForwardQuery_AppliesTheWordBound(t *testing.T) {
	const messy = "alpha   beta\n\tgamma"

	tests := []struct {
		name    string
		text    string
		max     int
		want    string
		words   int
		total   int
		applied bool
	}{
		{name: "under the bound keeps the original text", text: messy, max: 10, want: messy, words: 3, total: 3},
		{name: "exactly N words is not a bound", text: "one two three", max: 3, want: "one two three", words: 3, total: 3},
		{name: "over the bound keeps the first N words", text: "one two three four", max: 2, want: "one two", words: 2, total: 4, applied: true},
		{name: "whitespace collapses only when the bound applies", text: "one\n\ntwo   three\tfour", max: 3, want: "one two three", words: 3, total: 4, applied: true},
		{name: "non-ASCII words are counted as words", text: "café 東京 alpha beta", max: 2, want: "café 東京", words: 2, total: 4, applied: true},
		{name: "non-ASCII spaces split words", text: "one\u00a0two\u3000three four", max: 2, want: "one two", words: 2, total: 4, applied: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, words, total, applied := query.ForwardQuery(tt.text, tt.max)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.words, words)
			assert.Equal(t, tt.total, total)
			assert.Equal(t, tt.applied, applied)
		})
	}
}

// TestQueryBound_ZeroTurnsItOff pins both off switches.
// Ignoring the off switch (n <= 0 → n < 0) must fail this test.
func TestQueryBound_ZeroTurnsItOff(t *testing.T) {
	const text = "one two three four"
	weights := []float32{0.1, 5, 0.2, 4}
	words := []string{"one", "two", "three", "four"}
	wordOf := []int{0, 1, 2, 3}

	for _, n := range []int{0, -3} {
		got, _, _, applied := query.ForwardQuery(text, n)
		assert.Equal(t, text, got)
		assert.False(t, applied, "forward %d", n)
		assert.Nil(t, query.SelectSalientTokenIndices(weights, n), "tokens %d", n)
		assert.Nil(t, query.SelectSalientWords(words, wordOf, weights, n), "words %d", n)
	}

	analyzer := &weightAnalyzer{tokens: embedding.QueryTokens{Weights: weights, Word: wordOf, Words: words}}
	shaped := query.ShapeQuery(context.Background(), text, 0, 0, analyzer)
	assert.Equal(t, 0, analyzer.calls, "both bounds off does not ask for weights")
	assert.Nil(t, shaped.Report)
	assert.Nil(t, shaped.Keep)
	assert.Empty(t, shaped.FTSText)
	assert.Equal(t, text, shaped.EmbedText)

	// Salient off, forward on: the embed text is bounded and FTS sees it,
	// because there are no weights to pick terms from.
	forwardOnly := query.ShapeQuery(context.Background(), text, 2, 0, analyzer)
	assert.Equal(t, 0, analyzer.calls)
	assert.Equal(t, "one two", forwardOnly.EmbedText)
	assert.Equal(t, "one two", forwardOnly.FTSText)
	assert.Nil(t, forwardOnly.Keep)
	require.NotNil(t, forwardOnly.Report)
	assert.Equal(t, 2, forwardOnly.Report.EmbeddedWords)
	assert.Equal(t, 4, forwardOnly.Report.TotalWords)
	assert.Equal(t, 0, forwardOnly.Report.SalientTerms)
}

func TestShapeQuery_ShortQueryIsUnchanged(t *testing.T) {
	const original = "alpha   beta"
	analyzer := &weightAnalyzer{tokens: embedding.QueryTokens{
		Weights: []float32{0.2, 3},
		Word:    []int{0, 1},
		Words:   []string{"alpha", "beta"},
	}}

	shaped := query.ShapeQuery(context.Background(), original, 512, 64, analyzer)

	assert.Equal(t, original, shaped.EmbedText)
	assert.Empty(t, shaped.FTSText, "FTS keeps the caller's string, whitespace included")
	assert.Nil(t, shaped.Keep)
	assert.Nil(t, shaped.Report)
	assert.Equal(t, 1, analyzer.calls, "the token count still comes from the one forward pass")
}

func TestShapeQuery_AnalyzerErrorStillBoundsTheForwardPass(t *testing.T) {
	analyzer := &weightAnalyzer{err: errors.New("no weights")}
	shaped := query.ShapeQuery(context.Background(), "one two three four", 2, 1, analyzer)

	assert.Equal(t, "one two", shaped.EmbedText)
	assert.Equal(t, "one two", shaped.FTSText)
	assert.Nil(t, shaped.Keep)
	require.NotNil(t, shaped.Report)
	assert.Equal(t, 0, shaped.Report.SalientTerms)
}

func TestShapeQuery_NoAnalyzerBoundsFTSToTheForwardText(t *testing.T) {
	shaped := query.ShapeQuery(context.Background(), "one two three four", 2, 64, nil)

	assert.Equal(t, "one two", shaped.EmbedText)
	assert.Equal(t, "one two", shaped.FTSText, "without weights, FTS must not search the unbounded query")
	assert.Nil(t, shaped.Keep)
	require.NotNil(t, shaped.Report)
	assert.Equal(t, 0, shaped.Report.SalientTerms)
}

func TestShapeQuery_MissingWordMapDoesNotInventTerms(t *testing.T) {
	analyzer := &weightAnalyzer{tokens: embedding.QueryTokens{
		Weights: []float32{0.1, 9, 0.2, 8, 0.3},
		Word:    []int{-1, -1, -1, -1, -1},
	}}
	shaped := query.ShapeQuery(context.Background(), "one two three four five", 0, 2, analyzer)

	assert.Equal(t, []int{1, 3}, shaped.Keep)
	assert.Empty(t, shaped.FTSText, "no word alignment means FTS keeps the original query")
	require.NotNil(t, shaped.Report)
	assert.Equal(t, 2, shaped.Report.SalientTerms)
}

func TestQueryCapped_ZeroSalientTermsStaysInJSON(t *testing.T) {
	raw, err := json.Marshal(query.QueryCapped{EmbeddedWords: 512, TotalWords: 912, SalientTerms: 0})
	require.NoError(t, err)
	assert.JSONEq(t, `{"embedded_words":512,"total_words":912,"salient_terms":0}`, string(raw))
}
