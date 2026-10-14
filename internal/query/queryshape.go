package query

import (
	"context"
	"sort"
	"strings"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/rs/zerolog/log"
)

// QueryCapped reports that a bound shortened ranking. It is present on an
// ask result only when the forward bound or the salient-term bound applied.
// The ints are always serialized: a zero would otherwise vanish, and zero
// means "that bound did not apply."
type QueryCapped struct {
	EmbeddedWords int `json:"embedded_words"`
	TotalWords    int `json:"total_words"`
	SalientTerms  int `json:"salient_terms"`
}

// QueryAnalyzer reads per-token sparse weights from the one forward pass.
// BGEM3Embedder implements it. Embedders without those weights do not, and
// salient selection does not run for them.
type QueryAnalyzer interface {
	AnalyzeQuery(ctx context.Context, text string) (embedding.QueryTokens, error)
}

// AnalyzerFrom returns emb when it can supply sparse weights.
func AnalyzerFrom(emb embedding.Embedder) QueryAnalyzer {
	if emb == nil {
		return nil
	}
	a, _ := emb.(QueryAnalyzer)
	return a
}

// ShapedQuery is the one plan every ranking lane reads.
// EmbedText is what dense, sparse, ColBERT and the cosine embed.
// FTSText is empty when FTS should search the caller's original string.
// Keep is nil when ColBERT should score every query token.
type ShapedQuery struct {
	EmbedText string
	FTSText   string
	Keep      []int
	Report    *QueryCapped
}

// boundOff reports that a bound is turned off. 0 and negative both mean off.
// One function, so the off-switch cannot drift between the two bounds.
func boundOff(n int) bool { return n <= 0 }

// higherSparseWeight reports whether a outranks b. Ties are not greater:
// the caller keeps the earlier index. One comparison, so "highest" has one
// definition (Principle 7).
func higherSparseWeight(a, b float32) bool { return a > b }

// ForwardQuery bounds the text of one forward pass to the first maxWords
// words. Off, or already within the bound, returns text unchanged,
// whitespace included. A longer query is the first maxWords words joined
// by a single space.
func ForwardQuery(text string, maxWords int) (embedded string, embeddedWords, totalWords int, applied bool) {
	fields := strings.Fields(text)
	totalWords = len(fields)
	if boundOff(maxWords) || totalWords <= maxWords {
		return text, totalWords, totalWords, false
	}
	return strings.Join(fields[:maxWords], " "), maxWords, totalWords, true
}

// SelectSalientTokenIndices returns the k highest-weight token positions,
// in their original order. Nil means "use every token": the bound is off,
// or there are at most k tokens. Ties keep the earlier position.
func SelectSalientTokenIndices(weights []float32, k int) []int {
	if boundOff(k) || len(weights) <= k {
		return nil
	}
	idx := make([]int, len(weights))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool {
		if weights[idx[i]] == weights[idx[j]] {
			return idx[i] < idx[j]
		}
		return higherSparseWeight(weights[idx[i]], weights[idx[j]])
	})
	chosen := append([]int(nil), idx[:k]...)
	sort.Ints(chosen)
	return chosen
}

// SelectSalientWords returns the k highest-scoring words, in first-occurrence
// order. A word's score is the highest sparse weight among its tokens.
// Nil means the bound is off, the tokens were not mapped onto words, or
// there are at most k distinct words. The same word is emitted once.
func SelectSalientWords(words []string, tokenWord []int, weights []float32, k int) []string {
	if boundOff(k) || len(words) == 0 || len(tokenWord) == 0 {
		return nil
	}
	type scored struct {
		word  string
		score float32
		first int
	}
	best := make(map[string]int)
	var list []scored
	for i, w := range tokenWord {
		if w < 0 || w >= len(words) || i >= len(weights) {
			continue
		}
		word := words[w]
		if word == "" {
			continue
		}
		wt := weights[i]
		if j, ok := best[word]; ok {
			if higherSparseWeight(wt, list[j].score) {
				list[j].score = wt
			}
			continue
		}
		best[word] = len(list)
		list = append(list, scored{word: word, score: wt, first: w})
	}
	if len(list) <= k {
		return nil
	}
	order := make([]int, len(list))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		ia, ib := order[a], order[b]
		if list[ia].score == list[ib].score {
			return list[ia].first < list[ib].first
		}
		return higherSparseWeight(list[ia].score, list[ib].score)
	})
	picked := order[:k]
	sort.Slice(picked, func(a, b int) bool {
		return list[picked[a]].first < list[picked[b]].first
	})
	out := make([]string, len(picked))
	for i, p := range picked {
		out[i] = list[p].word
	}
	return out
}

// ShapeQuery is the only place the ranking-query shape is decided.
// The forward bound limits the one embedding. Salient selection, when the
// embedder has per-token weights, limits ColBERT and FTS. A failure to read
// those weights skips salient selection and still applies the forward bound.
func ShapeQuery(ctx context.Context, text string, forwardWords, salient int, analyzer QueryAnalyzer) ShapedQuery {
	embed, embWords, total, forwardApplied := ForwardQuery(text, forwardWords)
	out := ShapedQuery{EmbedText: embed}

	var tokens embedding.QueryTokens
	analyzed := false
	if !boundOff(salient) && analyzer != nil {
		var err error
		tokens, err = analyzer.AnalyzeQuery(ctx, embed)
		if err != nil {
			log.Debug().Err(err).Msg("ask query shape: salient weights unavailable")
		} else {
			analyzed = true
		}
	}

	var keep []int
	var picked []string
	if analyzed {
		keep = SelectSalientTokenIndices(tokens.Weights, salient)
		picked = SelectSalientWords(tokens.Words, tokens.Word, tokens.Weights, salient)
	}
	salientApplied := keep != nil || picked != nil
	out.Keep = keep
	if picked != nil {
		out.FTSText = strings.Join(picked, " ")
	} else if forwardApplied && !analyzed {
		out.FTSText = embed
	}

	if !forwardApplied && !salientApplied {
		return out
	}
	terms := 0
	if salientApplied {
		terms = salient
	}
	out.Report = &QueryCapped{
		EmbeddedWords: embWords,
		TotalWords:    total,
		SalientTerms:  terms,
	}
	return out
}

// shapedPlan is carried on the context so each lane can keep its string
// Search signature. An absent plan means "search the query you were given."
type shapedPlan struct {
	embed string
	fts   string
	keep  []int
}

type shapedPlanKey struct{}

func contextWithPlan(ctx context.Context, p shapedPlan) context.Context {
	return context.WithValue(ctx, shapedPlanKey{}, p)
}

func planFrom(ctx context.Context) (shapedPlan, bool) {
	p, ok := ctx.Value(shapedPlanKey{}).(shapedPlan)
	return p, ok
}

func embedTextFrom(ctx context.Context, query string) string {
	if p, ok := planFrom(ctx); ok && p.embed != "" {
		return p.embed
	}
	return query
}

func ftsTextFrom(ctx context.Context, query string) string {
	if p, ok := planFrom(ctx); ok && p.fts != "" {
		return p.fts
	}
	return query
}

func colbertKeepFrom(ctx context.Context) []int {
	if p, ok := planFrom(ctx); ok {
		return p.keep
	}
	return nil
}
