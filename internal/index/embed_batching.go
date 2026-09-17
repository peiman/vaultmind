package index

import "sort"

// Embedding batches are bounded by PADDED cost, not by count alone.
//
// The runtime pads every input in a batch out to the longest one, so the work
// a batch does is batchLen x longest, not the sum of its parts. With fixed
// batches of 8, one 8k-char note beside seven 500-char notes costs 8x8k
// instead of 8k + 7x500 — roughly 8x the compute for the same output. On a
// CPU-only machine with a few long notes that is the difference between
// seconds and minutes, and it recurs on every re-embed (issue #70).
//
// Two bounds, for two different reasons:
//   - the COUNT cap is a memory bound. BGE-M3 peak memory is roughly
//     batch x max_seq_len x hidden_dim, and batch=8 is what keeps a
//     memory-tight machine below the OOM killer (vaultmind#22). It is not an
//     optimisation and is never exceeded.
//   - the PADDED-COST budget is the throughput bound this adds.
//
// Sorting by length groups similar notes together, which does most of the
// work. The cost budget then prevents the failure a naive sort creates on its
// own: every long note landing in ONE batch of 8, which is precisely the
// memory spike the count cap exists to avoid.

// embedPaddedBudgetPerSlot is the per-slot share of a batch's padded cost, in
// body characters.
//
// A batch costs batchLen x longest. Multiplying this by the count cap gives the
// budget, so the bound scales with whatever batch size the caller chose rather
// than being a second magic number that can disagree with it.
//
// 8192 is one BGE-M3 context of characters: a full batch of average-length
// notes passes untouched, and a note an order of magnitude longer than its
// neighbours gets its own batch instead of inflating theirs. Deliberately
// generous — this is a throughput bound, and being wrong here costs some
// batching efficiency, never correctness.
const embedPaddedBudgetPerSlot = 8192

// planEmbedBatches groups note indices into batches by padded cost.
//
// Input is each note's body length. Returns batches of indices into that
// slice; every index appears exactly once. A note whose own length exceeds
// paddedBudget is emitted alone rather than skipped — dropping it would make a
// note silently unembeddable, which is the failure mode this codebase keeps
// finding.
func planEmbedBatches(lengths []int, maxCount, paddedBudget int) [][]int {
	if len(lengths) == 0 {
		return nil
	}
	if maxCount < 1 {
		maxCount = 1
	}

	order := make([]int, len(lengths))
	for i := range order {
		order[i] = i
	}
	// Ascending by length, ties by index so the plan is reproducible.
	sort.SliceStable(order, func(a, b int) bool {
		if lengths[order[a]] != lengths[order[b]] {
			return lengths[order[a]] < lengths[order[b]]
		}
		return order[a] < order[b]
	})

	var batches [][]int
	var cur []int
	curMax := 0

	for _, idx := range order {
		l := lengths[idx]
		nextMax := curMax
		if l > nextMax {
			nextMax = l
		}
		// Fits if the count cap allows it AND the padded cost stays in budget.
		// An empty batch always accepts, so an oversized note is soloed rather
		// than looping forever or being dropped.
		if len(cur) > 0 && (len(cur)+1 > maxCount || (len(cur)+1)*nextMax > paddedBudget) {
			batches = append(batches, cur)
			cur = nil
			nextMax = l
		}
		cur = append(cur, idx)
		curMax = nextMax
	}
	if len(cur) > 0 {
		batches = append(batches, cur)
	}
	return batches
}
