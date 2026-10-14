package query

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ftsTextRetriever is a stub retriever (ADR-003). Search records the string
// the FTS lane actually searches, which is ftsTextFrom, not the caller's query.
type ftsTextRetriever struct {
	got string
}

func (r *ftsTextRetriever) Search(ctx context.Context, q string, limit, _ int, _ index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	r.got = ftsTextFrom(ctx, q)
	if limit <= 0 {
		return nil, 0, nil
	}
	return []retrieval.ScoredResult{{ID: "n", Title: "N", Score: 1}}, 1, nil
}

// spanAnalyzer is a stub analyzer (ADR-003). perRune makes TokenSpans one
// span per rune, which is how a scriptio-continua tokenizer cuts.
type spanAnalyzer struct {
	tokens  embedding.QueryTokens
	perRune bool
	seen    string
}

func (a *spanAnalyzer) AnalyzeQuery(_ context.Context, text string) (embedding.QueryTokens, error) {
	a.seen = text
	if a.perRune {
		return runeTokens(text), nil
	}
	return a.tokens, nil
}

func (a *spanAnalyzer) TokenSpans(text string, _ int) ([][2]int, error) {
	if !a.perRune {
		return nil, nil
	}
	return runeSpans(text), nil
}

func runeSpans(text string) [][2]int {
	var spans [][2]int
	for i := 0; i < len(text); {
		_, size := utf8.DecodeRuneInString(text[i:])
		spans = append(spans, [2]int{i, i + size})
		i += size
	}
	return spans
}

// runeTokens builds one token per rune of text, all mapped onto one word
// (the whole text). Four positions that are not the first four carry the
// highest weights, so salient selection cannot be "the first K".
func runeTokens(text string) embedding.QueryTokens {
	runes := []rune(text)
	n := len(runes)
	weights := make([]float32, n)
	word := make([]int, n)
	offsets := make([][2]int, n)
	b := 0
	for i, r := range runes {
		weights[i] = 0.01
		size := utf8.RuneLen(r)
		offsets[i] = [2]int{b, b + size}
		b += size
	}
	for rank, i := range []int{10, 20, 40, 70} {
		if i < n {
			weights[i] = float32(9 - rank)
		}
	}
	return embedding.QueryTokens{
		Weights: weights,
		Word:    word,
		Words:   []string{text},
		Offsets: offsets,
	}
}

func cjkParagraph(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteRune(rune(0x4E00 + i))
	}
	return b.String()
}

// Restoring `&& !analyzed` on the forward FTS fallback must fail this test:
// the weights were read, and fifty distinct words is not more than K, so no
// word list is chosen. FTS still has to search the forward prefix.
func TestAskHits_RepeatedWordsFTSGetsTheForwardPrefix(t *testing.T) {
	words := make([]string, 600)
	wordOf := make([]int, 600)
	weights := make([]float32, 600)
	distinct := make([]string, 50)
	for i := range distinct {
		distinct[i] = fmt.Sprintf("w%02d", i)
	}
	for i := range words {
		words[i] = distinct[i%50]
		wordOf[i] = i % 50
		weights[i] = float32(600 - i)
	}
	text := strings.Join(words, " ")
	lane := &ftsTextRetriever{}

	res, err := AskHits(context.Background(), lane, text, 5, 32, 64, &spanAnalyzer{
		tokens: embedding.QueryTokens{Weights: weights, Word: wordOf, Words: distinct},
	})
	require.NoError(t, err)
	want := strings.Join(words[:32], " ")
	assert.Equal(t, want, lane.got)
	assert.NotEqual(t, text, lane.got)
	require.NotNil(t, res.QueryCapped)
	assert.Equal(t, 32, res.QueryCapped.EmbeddedTokens)
	assert.Equal(t, 600, res.QueryCapped.TotalTokens)
}

// The word map is missing and the weights were read. Restoring `&& !analyzed`
// leaves FTSText empty, and the lane then searches the unbounded query.
func TestAskHits_MissingWordMapFTSGetsTheForwardPrefix(t *testing.T) {
	const text = "one two three four five"
	lane := &ftsTextRetriever{}
	_, err := AskHits(context.Background(), lane, text, 5, 2, 64, &spanAnalyzer{
		tokens: embedding.QueryTokens{
			Weights: []float32{0.1, 9, 0.2, 8, 0.3},
			Word:    []int{-1, -1, -1, -1, -1},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "one two", lane.got)
	assert.NotEqual(t, text, lane.got)
}

// Counting whitespace words instead of tokens must fail this test. The
// paragraph is one word, so a word bound of 80 does not engage, and FTS
// would search the whole run rather than four token surfaces.
func TestAskHits_CJKParagraphBoundsByTokens(t *testing.T) {
	const (
		runes   = 200
		forward = 80
		salient = 4
	)
	text := cjkParagraph(runes)
	analyzer := &spanAnalyzer{perRune: true}
	lane := &ftsTextRetriever{}

	res, err := AskHits(context.Background(), lane, text, 5, forward, salient, analyzer)
	require.NoError(t, err)

	require.NotNil(t, res.QueryCapped)
	assert.Equal(t, forward, res.QueryCapped.EmbeddedTokens)
	assert.Equal(t, runes, res.QueryCapped.TotalTokens)
	assert.Equal(t, salient, res.QueryCapped.SalientTerms)
	require.NotEmpty(t, analyzer.seen)
	assert.True(t, strings.HasPrefix(text, analyzer.seen))
	assert.Less(t, len(analyzer.seen), len(text))
	assert.Equal(t, forward, utf8.RuneCountInString(analyzer.seen))

	chars := []rune(analyzer.seen)
	want := strings.Join([]string{
		string(chars[10]),
		string(chars[20]),
		string(chars[40]),
		string(chars[70]),
	}, " ")
	assert.Equal(t, want, lane.got, "FTS searches the salient token surfaces")
	assert.NotEqual(t, text, lane.got)
	assert.NotEqual(t, analyzer.seen, lane.got)
	assert.Len(t, strings.Fields(lane.got), salient)
}
