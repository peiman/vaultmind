package query

import (
	"context"
	"math"
	"sort"
	"strings"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/passage"
)

// Any unavailable or stale input falls back to the established lexical path.
func selectDensePassages(ctx context.Context, db *index.DB, cfg AskConfig, targetID, rankingQuery, raw, title string, limit int) (string, bool) {
	if cfg.Embedder == nil || limit <= 0 {
		return "", false
	}
	vectors, err := index.LoadPassageVectors(db, targetID)
	raw = parser.StripHTMLComments(raw)
	units := passage.Units(raw, title)
	if err != nil || !matchingPassageSpans(units, vectors, cfg.Embedder.Dims()) {
		return "", false
	}
	queryVec, err := cfg.Embedder.Embed(ctx, rankingQuery)
	if err != nil || len(queryVec) != cfg.Embedder.Dims() {
		return "", false
	}
	ranked := rankDensePassages(queryVec, vectors)
	if len(ranked) == 0 {
		return "", false
	}
	for len(ranked) > 0 {
		best := renderDensePassages(raw, title, units, ranked)
		if memory.EstimateTokens(best) <= limit {
			return best, best != ""
		}
		if len(ranked) == 1 {
			u := units[ranked[0]]
			heading := capHeadingTokens(nearestGoverningHeading(u.Heading, title), min(firstBlockHeadingTokens, limit))
			text := memory.Excerpt(raw[u.Start:u.End], limit)
			if heading != "" {
				text = fitHeadingAndBlock(heading, raw[u.Start:u.End], limit)
			}
			return text, text != "" && memory.EstimateTokens(text) <= limit
		}
		ranked = ranked[:len(ranked)-1] // discard the lowest cosine, not the last in document order
	}
	return "", false
}

func matchingPassageSpans(units []passage.Unit, vectors []index.PassageVector, dims int) bool {
	if len(units) == 0 || len(units) != len(vectors) || dims <= 0 {
		return false
	}
	for i, u := range units {
		v := vectors[i]
		if v.UnitIdx != i || v.Start != u.Start || v.End != u.End || len(v.Dense) != dims {
			return false
		}
	}
	return true
}

func rankDensePassages(queryVec []float32, vectors []index.PassageVector) []int {
	order, scores := make([]int, len(vectors)), make([]float64, len(vectors))
	for i, v := range vectors {
		order[i], scores[i] = i, CosineSimilarity(queryVec, v.Dense)
		if math.IsNaN(scores[i]) || math.IsInf(scores[i], 0) {
			return nil
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return scores[order[i]] > scores[order[j]] })
	return order[:min(3, len(order))]
}

func renderDensePassages(raw, title string, units []passage.Unit, ranked []int) string {
	order := append([]int(nil), ranked...)
	sort.Ints(order)
	var spans []string
	for i := 0; i < len(order); {
		first, last := units[order[i]], units[order[i]]
		j := i + 1
		// Heading and fence boundaries remain separate spans even when their
		// units are consecutive: each must carry its own qualification.
		for j < len(order) && order[j] == order[j-1]+1 && units[order[j]].Section == first.Section {
			last = units[order[j]]
			j++
		}
		text := strings.TrimSpace(raw[first.Start:last.End])
		heading := capHeadingTokens(nearestGoverningHeading(first.Heading, title), firstBlockHeadingTokens)
		if heading != "" {
			text = heading + "\n\n" + text
		}
		spans = append(spans, text)
		i = j
	}
	return strings.Join(spans, "\n…\n")
}
