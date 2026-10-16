package index

import (
	"sort"

	"github.com/peiman/vaultmind/internal/embedding"
)

// planBGEM3Batches groups note indices so every batch fits the BGE-M3 memory
// budget (embedding.BGEM3BatchFits, on body lengths): at most one cap-length
// note's worth of padded tokens and embedding.BGEM3MaxBatch notes per
// forward. Memory has a linear part (hidden states per token) and a quadratic
// part (attention); the bound caps both, because a note is never longer than
// the cap, so count × longest² ≤ cap × longest ≤ cap². Shortest first, so
// similar lengths share a batch. A note at the cap runs alone. Every index
// appears exactly once.
func planBGEM3Batches(lengths []int) [][]int {
	if len(lengths) == 0 {
		return nil
	}
	order := make([]int, len(lengths))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		if lengths[order[a]] != lengths[order[b]] {
			return lengths[order[a]] < lengths[order[b]]
		}
		return order[a] < order[b]
	})
	var batches [][]int
	var cur []int
	longest := 0
	for _, idx := range order {
		next := max(longest, lengths[idx])
		if len(cur) > 0 && !embedding.BGEM3BatchFits(len(cur)+1, next) {
			batches = append(batches, cur)
			cur = nil
			next = lengths[idx]
		}
		cur = append(cur, idx)
		longest = next
	}
	if len(cur) > 0 {
		batches = append(batches, cur)
	}
	return batches
}
