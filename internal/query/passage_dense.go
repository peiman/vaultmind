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

// densePassageBlocks is how many top-ranked dense blocks are candidates to
// join the lexical pick.
const densePassageBlocks = 3

// selectDensePassages renders the top three paragraph blocks by cosine,
// dropping any that lex already contains or that do not fit within limit
// tokens. A dropped block is not replaced by a lower-ranked one: a weak
// match would only pad the body. It returns "" when nothing is added; any
// unavailable or stale input leaves the lexical pick alone.
func selectDensePassages(ctx context.Context, db *index.DB, cfg AskConfig, targetID, rankingQuery, raw, title, lex string, limit int) string {
	if cfg.Embedder == nil || limit <= 0 {
		return ""
	}
	vectors, err := index.LoadPassageVectors(db, targetID)
	raw = parser.StripHTMLComments(raw)
	units := passage.Units(raw, title)
	if err != nil || !matchingPassageSpans(units, vectors, cfg.Embedder.Dims()) {
		return ""
	}
	queryVec, err := cfg.Embedder.Embed(ctx, rankingQuery)
	if err != nil || len(queryVec) != cfg.Embedder.Dims() {
		return ""
	}
	seen := strings.Join(strings.Fields(lex), " ")
	var chosen []densePassageBlock
	ranked := rankDensePassages(raw, units, queryVec, vectors)
	for _, block := range ranked[:min(densePassageBlocks, len(ranked))] {
		if strings.Contains(seen, strings.Join(strings.Fields(raw[units[block.first].Start:units[block.last].End]), " ")) {
			continue
		}
		if memory.EstimateTokens(renderDensePassages(raw, title, units, append(chosen, block))) > limit {
			continue
		}
		chosen = append(chosen, block)
	}
	return renderDensePassages(raw, title, units, chosen)
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
	first, last, index int
	score              float64
}

func rankDensePassages(raw string, units []passage.Unit, queryVec []float32, vectors []index.PassageVector) []densePassageBlock {
	var blocks []densePassageBlock
	for i, v := range vectors {
		score := CosineSimilarity(queryVec, v.Dense)
		if math.IsNaN(score) || math.IsInf(score, 0) {
			return nil
		}
		if i == 0 || units[i].Section != units[i-1].Section || passage.BlankLine.MatchString(raw[units[i-1].End:units[i].Start]) {
			blocks = append(blocks, densePassageBlock{first: i, last: i, index: len(blocks), score: score})
			continue
		}
		block := &blocks[len(blocks)-1]
		block.last = i
		block.score = math.Max(block.score, score)
	}
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].score > blocks[j].score })
	return blocks
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
				out.WriteString(passageGap)
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
