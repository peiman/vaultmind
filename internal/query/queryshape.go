package query

import (
	"context"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/rs/zerolog/log"
)

// QueryCapped reports that a bound shortened ranking. It is present on an
// ask result only when the forward bound or the salient-term bound applied.
// EmbeddedTokens and SalientTerms are always serialized: zero means that
// bound did not apply. TotalTokens is the counted token total and is omitted
// when the tail was not tokenized. TotalChars is then the query's character
// count, and is omitted when the token total was counted.
type QueryCapped struct {
	EmbeddedTokens int `json:"embedded_tokens"`
	TotalTokens    int `json:"total_tokens,omitempty"`
	TotalChars     int `json:"total_chars,omitempty"`
	SalientTerms   int `json:"salient_terms"`
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

// QueryTokenizer reports content-token byte spans for text, specials excluded.
// forwardTokens is the forward bound in use. <= 0 reads the whole string.
// BGEM3Embedder implements it. An analyzer without it is bounded one whitespace
// word per token.
type QueryTokenizer interface {
	TokenSpans(text string, forwardTokens int) ([][2]int, error)
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

// longRunRunes is the whitespace-word length past which the word is not a
// lexical FTS term. An English word, even one split into several BPE pieces,
// stays one term. A CJK or Thai paragraph is one run with no spaces, and its
// salient tokens contribute their own surface. Exactly this many runes is
// still one word.
const longRunRunes = 64

// ForwardQuery bounds one forward pass to the first maxTokens content tokens.
// spans are byte ranges into text; nil means one span per whitespace word
// (embedding.WhitespaceTokenSpans — the same split the word map uses,
// Principle 7). Off, or already within the bound, returns text unchanged.
// A longer query is the prefix ending at the last kept token, so the embedded
// text is a clean prefix and internal whitespace stays. A span that cannot be
// cut as valid UTF-8 does not apply the bound.
func ForwardQuery(text string, maxTokens int, spans [][2]int) (embedded string, embeddedTokens, totalTokens int, applied bool) {
	if spans == nil {
		spans = embedding.WhitespaceTokenSpans(text)
	}
	totalTokens = len(spans)
	if boundOff(maxTokens) || totalTokens <= maxTokens {
		return text, totalTokens, totalTokens, false
	}
	start, end := spans[maxTokens-1][0], spans[maxTokens-1][1]
	if start < 0 || end <= start || end > len(text) || !utf8.ValidString(text[:end]) {
		return text, totalTokens, totalTokens, false
	}
	return text[:end], maxTokens, totalTokens, true
}

// tokenSpansOf reads content-token spans when the analyzer can tokenize.
// forwardTokens sizes the tokenizer's window. A tokenizer error falls back
// to whitespace spans inside ForwardQuery. The model clamp is what stops an
// oversized forward pass.
func tokenSpansOf(analyzer QueryAnalyzer, text string, forwardTokens int) [][2]int {
	tok, ok := analyzer.(QueryTokenizer)
	if !ok || tok == nil {
		return nil
	}
	spans, err := tok.TokenSpans(text, forwardTokens)
	if err != nil {
		log.Debug().Err(err).Msg("ask query shape: token spans unavailable")
		return nil
	}
	return spans
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

// applyUnderCountCut treats a span window that ended before the text, and
// still held no more spans than the bound, as a long query. The tokens past
// the window were not read. The embedded text is that window.
func applyUnderCountCut(text string, forwardTokens int, spans [][2]int, embed string, embTokens, total int, applied bool) (string, int, int, bool) {
	if applied || boundOff(forwardTokens) || spans == nil || len(spans) > forwardTokens {
		return embed, embTokens, total, applied
	}
	window := embedding.QuerySpanWindow(text, forwardTokens)
	if len(window) >= len(text) {
		return embed, embTokens, total, applied
	}
	log.Debug().
		Int("spans", len(spans)).
		Int("forward_tokens", forwardTokens).
		Int("window_bytes", len(window)).
		Int("text_bytes", len(text)).
		Msg("ask query shape: span window under-counted; cutting at the window")
	// The tokens past the window were not read. Zero says the total is
	// unknown; the caller reports the query's character count instead.
	return window, forwardTokens, 0, true
}

// ShapeQuery is the only place the ranking-query shape is decided.
// The forward bound limits the one embedding. Salient selection, when the
// embedder has per-token weights, limits ColBERT and FTS. A failure to read
// those weights skips salient selection and still applies the forward bound.
func ShapeQuery(ctx context.Context, text string, forwardTokens, salient int, analyzer QueryAnalyzer) ShapedQuery {
	original := text
	// The echoed query is the caller's string. Only the ranking text is capped.
	text = embedding.CapSymbolRuns(text)
	spans := tokenSpansOf(analyzer, text, forwardTokens)
	embed, embTokens, total, forwardApplied := ForwardQuery(text, forwardTokens, spans)
	embed, embTokens, total, forwardApplied = applyUnderCountCut(text, forwardTokens, spans, embed, embTokens, total, forwardApplied)
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
	var fts []string
	if analyzed {
		keep = SelectSalientTokenIndices(tokens.Weights, salient)
		fts = ftsTerms(embed, tokens, keep, salient)
	}
	salientApplied := keep != nil || len(fts) > 0
	out.Keep = keep
	if len(fts) > 0 {
		out.FTSText = strings.Join(fts, " ")
	} else if forwardApplied {
		// Weights may have been read and still produced no FTS terms: too few
		// distinct words, or no word map. An empty FTSText would send the lane
		// the caller's original, unbounded query.
		out.FTSText = embed
	}

	if !forwardApplied && !salientApplied {
		return out
	}
	reported := 0
	if salientApplied {
		reported = salient
	}
	totalChars := 0
	if forwardApplied && total == 0 {
		totalChars = utf8.RuneCountInString(original)
	}
	out.Report = &QueryCapped{
		EmbeddedTokens: embTokens,
		TotalTokens:    total,
		TotalChars:     totalChars,
		SalientTerms:   reported,
	}
	return out
}

// ftsTerms is the one decision for what FTS searches (Principle 7). Spaced
// words stay whole. A salient token whose word is a long no-space run
// contributes that token's surface, not the run. Nil means the caller falls
// back to the forward prefix rather than an empty search string.
func ftsTerms(text string, tokens embedding.QueryTokens, keep []int, salient int) []string {
	picked := SelectSalientWords(tokens.Words, tokens.Word, tokens.Weights, salient)
	if !keepHasLongRun(tokens, keep) {
		return picked
	}
	terms := tokenSurfaces(text, tokens, keep)
	if len(terms) == 0 {
		return nil
	}
	return terms
}

func keepHasLongRun(tokens embedding.QueryTokens, keep []int) bool {
	if keep == nil {
		return false
	}
	for _, i := range keep {
		if longNoSpaceRun(wordAt(tokens, i)) {
			return true
		}
	}
	return false
}

func longNoSpaceRun(word string) bool {
	return utf8.RuneCountInString(word) > longRunRunes
}

func wordAt(tokens embedding.QueryTokens, i int) string {
	if i < 0 || i >= len(tokens.Word) {
		return ""
	}
	w := tokens.Word[i]
	if w < 0 || w >= len(tokens.Words) {
		return ""
	}
	return tokens.Words[w]
}

func tokenSurfaces(text string, tokens embedding.QueryTokens, keep []int) []string {
	seen := make(map[string]struct{}, len(keep))
	var out []string
	for _, i := range keep {
		term := ftsTermAt(text, tokens, i)
		if term == "" {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		out = append(out, term)
	}
	return out
}

func ftsTermAt(text string, tokens embedding.QueryTokens, i int) string {
	word := wordAt(tokens, i)
	if word == "" {
		return ""
	}
	if !longNoSpaceRun(word) {
		return word
	}
	if i < 0 || i >= len(tokens.Offsets) {
		return ""
	}
	return tokenSurface(text, tokens.Offsets[i])
}

func tokenSurface(text string, off [2]int) string {
	if off[0] < 0 || off[1] > len(text) || off[1] <= off[0] {
		return ""
	}
	s := text[off[0]:off[1]]
	if !utf8.ValidString(s) {
		return ""
	}
	return s
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
