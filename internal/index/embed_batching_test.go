package index

import (
	"testing"
)

// Batching by PADDED cost, not by count.
//
// The runtime pads every input in a batch out to the longest one, so a single
// long note drags its neighbours up to full length and the machine does work
// proportional to batchLen x longest. Fixed batches of 8 make that the common
// case: one 8k-char note beside seven 500-char notes costs 8x8k instead of
// 8k+7x500.
//
// Sorting by length groups similar notes together, and the padded-cost cap
// stops the other failure the naive sort creates — all the long notes landing
// in ONE batch, which is the memory spike batch_size=8 exists to prevent.
func TestPlanEmbedBatches_LongNoteDoesNotDragShortOnes(t *testing.T) {
	lens := []int{8000, 500, 500, 500, 500, 500, 500, 500}

	batches := planEmbedBatches(lens, 8, 16000)

	// The 8000 must not sit in a batch that pads seven 500s up to 8000.
	for _, b := range batches {
		maxLen, n := 0, len(b)
		for _, idx := range b {
			if lens[idx] > maxLen {
				maxLen = lens[idx]
			}
		}
		if n*maxLen > 16000 && n > 1 {
			t.Errorf("batch of %d padded to %d = %d exceeds the budget; a long note dragged short ones",
				n, maxLen, n*maxLen)
		}
	}
}

// Every note must appear exactly once — batching may reorder, never drop.
func TestPlanEmbedBatches_CoversEveryNoteExactlyOnce(t *testing.T) {
	lens := []int{100, 9000, 250, 4000, 80, 12000, 300, 75, 640, 1200}

	batches := planEmbedBatches(lens, 8, 16000)

	seen := make([]int, len(lens))
	for _, b := range batches {
		for _, idx := range b {
			seen[idx]++
		}
	}
	for i, c := range seen {
		if c != 1 {
			t.Errorf("note %d appeared %d times, want exactly 1 — batching must not drop or duplicate", i, c)
		}
	}
}

// A note larger than the whole budget still gets embedded, alone. Dropping it
// would be the silent-loss failure this project keeps finding.
func TestPlanEmbedBatches_OversizedNoteIsSoloedNotDropped(t *testing.T) {
	lens := []int{100, 999999, 100}

	batches := planEmbedBatches(lens, 8, 16000)

	var soloed bool
	total := 0
	for _, b := range batches {
		total += len(b)
		if len(b) == 1 && lens[b[0]] == 999999 {
			soloed = true
		}
	}
	if total != len(lens) {
		t.Fatalf("covered %d of %d notes", total, len(lens))
	}
	if !soloed {
		t.Error("a note above the whole budget must be embedded alone, never skipped")
	}
}

// The hard count cap still holds — it is the memory bound, not an optimisation.
func TestPlanEmbedBatches_NeverExceedsTheCountCap(t *testing.T) {
	lens := make([]int, 40)
	for i := range lens {
		lens[i] = 10
	}

	batches := planEmbedBatches(lens, 8, 1_000_000)

	for _, b := range batches {
		if len(b) > 8 {
			t.Errorf("batch of %d exceeds the count cap of 8 — that cap bounds peak memory", len(b))
		}
	}
}
