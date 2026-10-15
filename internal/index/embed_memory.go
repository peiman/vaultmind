package index

import (
	"sort"

	"github.com/peiman/vaultmind/internal/embedding"
)

// BGE-M3 forward memory grows with the padded tokens in a batch, and faster
// than linearly in a note's length. Measured on an ORT build: batches of 32
// long notes took a resident process to 56 GB; one note at the old 8190 cap
// passed 8 GB alone. So a batch holds at most one cap-length note's worth of
// padded tokens, and short notes still share a batch up to the count cap.

// bgem3EmbedTokens is the per-note token cap the embedder truncates to. One
// definition: the embedding package's.
const bgem3EmbedTokens = embedding.BGEM3EmbedMaxTokens

// bgem3MaxBatch is the count cap for a BGE-M3 batch.
const bgem3MaxBatch = 8

// bgem3CharsPerToken converts body characters to an upper estimate of tokens.
// English runs near four characters per token, so two over-counts it: a batch
// may come out smaller than it could be, never larger than memory allows.
// Text with fewer characters per token (CJK) is under-counted by up to 2x;
// the cap still truncates every note to bgem3EmbedTokens.
const bgem3CharsPerToken = 2

func bgem3TokenEstimate(chars int) int {
	t := (chars + bgem3CharsPerToken - 1) / bgem3CharsPerToken
	if t > bgem3EmbedTokens {
		return bgem3EmbedTokens
	}
	return t
}

// planBGEM3Batches groups note indices so every batch satisfies
// count × longest ≤ bgem3EmbedTokens (tokens estimated from characters) and
// count ≤ bgem3MaxBatch: at most one cap-length note's worth of padded
// tokens per forward. Memory has a linear part (hidden states per token) and
// a quadratic part (attention); this bound caps both, because a note is never
// longer than the cap, so count × longest² ≤ cap × longest ≤ cap². Shortest first, so similar lengths share a
// batch. A note at the cap runs alone. Every index appears exactly once.
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
	most := 0
	for _, idx := range order {
		t := bgem3TokenEstimate(lengths[idx])
		next := most
		if t > next {
			next = t
		}
		if len(cur) > 0 && (len(cur)+1 > bgem3MaxBatch || (len(cur)+1)*next > bgem3EmbedTokens) {
			batches = append(batches, cur)
			cur = nil
			next = t
		}
		cur = append(cur, idx)
		most = next
	}
	if len(cur) > 0 {
		batches = append(batches, cur)
	}
	return batches
}
