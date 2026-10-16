package embedding

import (
	"errors"
	"fmt"
)

// BGE-M3 forward memory grows with the padded tokens in a batch, and faster
// than linearly in a note's length. Measured on an ORT build: batches of 32
// long notes took one process past 100 GB; one note at the old 8190 cap
// passed 8 GB alone. So a batch holds at most one cap-length note's worth of
// padded tokens, and short notes still share a batch up to the count cap.
// This file is the one definition of that budget: the indexer plans batches
// with it, and the embedders refuse a batch outside it.

// BGEM3MaxBatch is the most texts one BGE-M3 batch may hold.
const BGEM3MaxBatch = 8

// bgem3CharsPerToken converts characters to an upper estimate of tokens.
// English runs near four characters per token, so two over-counts it: a batch
// may come out smaller than it could be, never larger than memory allows.
// Text with fewer characters per token (CJK) is under-counted by up to 2x;
// the cap still truncates every text to BGEM3EmbedMaxTokens.
const bgem3CharsPerToken = 2

// ErrBatchOverMemoryBudget is returned for a batch BGEM3BatchFits rejects.
var ErrBatchOverMemoryBudget = errors.New("BGE-M3 batch is over the memory budget")

// BGEM3TokenEstimate is the upper estimate of tokens for a text of chars
// bytes, capped at BGEM3EmbedMaxTokens (the embedder truncates there).
func BGEM3TokenEstimate(chars int) int {
	t := (chars + bgem3CharsPerToken - 1) / bgem3CharsPerToken
	return min(t, BGEM3EmbedMaxTokens)
}

// BGEM3BatchFits reports whether count texts, the longest of longestChars
// bytes, fit one batch: at most BGEM3MaxBatch texts, and count × the longest
// text's estimated tokens at most BGEM3EmbedMaxTokens. One text always fits.
func BGEM3BatchFits(count, longestChars int) bool {
	return count > 0 && count <= BGEM3MaxBatch && count*BGEM3TokenEstimate(longestChars) <= BGEM3EmbedMaxTokens
}

// checkBGEM3Batch returns ErrBatchOverMemoryBudget for texts that do not fit
// one batch. An empty batch passes: there is nothing to run.
func checkBGEM3Batch(texts []string) error {
	if len(texts) == 0 {
		return nil
	}
	longest := 0
	for _, t := range texts {
		longest = max(longest, len(t))
	}
	if !BGEM3BatchFits(len(texts), longest) {
		return fmt.Errorf("%w: %d texts, longest %d bytes (at most %d texts and %d padded tokens; plan batches with BGEM3BatchFits)",
			ErrBatchOverMemoryBudget, len(texts), longest, BGEM3MaxBatch, BGEM3EmbedMaxTokens)
	}
	return nil
}
