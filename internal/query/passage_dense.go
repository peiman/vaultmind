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
	ranked := rankDensePassages(raw, units, queryVec, vectors)
	if len(ranked) == 0 {
		return "", false
	}
	for len(ranked) > 0 {
		best := renderDensePassages(raw, title, units, ranked)
		if memory.EstimateTokens(best) <= limit {
			return best, best != ""
		}
		if len(ranked) == 1 {
			block := ranked[0]
			u := units[block.best]
			heading := capHeadingTokens(nearestGoverningHeading(u.Heading, title), firstBlockHeadingTokens)
			body := raw[u.Start:units[block.last].End]
			if block.best != block.first {
				body = "…" + body
			}
			text := memory.Excerpt(body, limit)
			if heading != "" {
				text = fitHeadingAndBlock(heading, body, limit)
			}
			if text != "" && memory.EstimateTokens(text) <= limit {
				return text, true
			}
			return "", false
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

type densePassageBlock struct {
	first, last, best, index int
	score                    float64
}

func rankDensePassages(raw string, units []passage.Unit, queryVec []float32, vectors []index.PassageVector) []densePassageBlock {
	var blocks []densePassageBlock
	for i, v := range vectors {
		score := CosineSimilarity(queryVec, v.Dense)
		if math.IsNaN(score) || math.IsInf(score, 0) {
			return nil
		}
		if i == 0 || units[i].Section != units[i-1].Section || passage.BlankLine.MatchString(raw[units[i-1].End:units[i].Start]) {
			blocks = append(blocks, densePassageBlock{first: i, last: i, best: i, index: len(blocks), score: score})
			continue
		}
		block := &blocks[len(blocks)-1]
		block.last = i
		if score > block.score {
			block.score, block.best = score, i
		}
	}
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].score > blocks[j].score })
	return blocks[:min(3, len(blocks))]
}

func renderDensePassages(raw, title string, units []passage.Unit, ranked []densePassageBlock) string {
	order := append([]densePassageBlock(nil), ranked...)
	sort.Slice(order, func(i, j int) bool { return order[i].index < order[j].index })
	var out strings.Builder
	previousHeading := ""
	for i, block := range order {
		first, last := units[block.first], units[block.last]
		if i > 0 {
			previous := order[i-1]
			if block.index == previous.index+1 && first.Section == units[previous.first].Section {
				out.WriteString("\n\n")
			} else {
				out.WriteString("\n…\n")
			}
		}
		heading := capHeadingTokens(nearestGoverningHeading(first.Heading, title), firstBlockHeadingTokens)
		if heading != "" && heading != previousHeading {
			out.WriteString(heading + "\n\n")
		}
		out.WriteString(strings.TrimSpace(raw[first.Start:last.End]))
		previousHeading = heading
	}
	return out.String()
}
