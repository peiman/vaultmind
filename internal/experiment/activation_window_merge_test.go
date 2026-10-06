package experiment

import (
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// partitionTimeReference is PartitionTime as it was before the windows were
// merged once per scoring call: sort and merge on every call. Kept verbatim
// so the faster path is held to exactly the old answer.
func partitionTimeReference(start, end time.Time, windows []SessionWindow) (active, idle time.Duration) {
	total := end.Sub(start)
	if total <= 0 {
		return 0, 0
	}
	sorted := make([]SessionWindow, len(windows))
	copy(sorted, windows)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start.Before(sorted[j].Start) })
	merged := make([]SessionWindow, 0, len(sorted))
	for _, w := range sorted {
		if len(merged) > 0 && !w.Start.After(merged[len(merged)-1].End) {
			if w.End.After(merged[len(merged)-1].End) {
				merged[len(merged)-1].End = w.End
			}
		} else {
			merged = append(merged, w)
		}
	}
	for _, w := range merged {
		wStart, wEnd := w.Start, w.End
		if wStart.Before(start) {
			wStart = start
		}
		if wEnd.After(end) {
			wEnd = end
		}
		if wStart.Before(wEnd) {
			active += wEnd.Sub(wStart)
		}
	}
	idle = total - active
	if idle < 0 {
		idle = 0
	}
	return active, idle
}

func computeRetrievalReference(accessTimes []time.Time, now time.Time, windows []SessionWindow, gamma, d float64) float64 {
	return retrievalFrom(accessTimes, now, gamma, d, func(at time.Time) (time.Duration, time.Duration) {
		return partitionTimeReference(at, now, windows)
	})
}

// randomWindows returns n windows in random order, some overlapping, some
// nested, some touching, spread over the 30 days before now.
func randomWindows(r *rand.Rand, now time.Time, n int) []SessionWindow {
	ws := make([]SessionWindow, n)
	for i := range ws {
		start := now.Add(-time.Duration(r.Int63n(int64(30 * 24 * time.Hour))))
		ws[i] = SessionWindow{Start: start, End: start.Add(time.Duration(r.Int63n(int64(6 * time.Hour))))}
	}
	if n > 2 {
		ws[1] = SessionWindow{Start: ws[0].End, End: ws[0].End.Add(time.Hour)} // touching
	}
	return ws
}

// Merging the windows once per call gives exactly the old scores: the same
// merged list, the same order of summation, integer durations.
func TestComputeRetrieval_EqualsPerAccessMerging(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	for trial := 0; trial < 300; trial++ {
		windows := randomWindows(r, now, r.Intn(101))
		accesses := make([]time.Time, 1+r.Intn(40))
		for i := range accesses {
			accesses[i] = now.Add(-time.Duration(r.Int63n(int64(40 * 24 * time.Hour))))
		}
		accesses = append(accesses, now, now.Add(time.Minute)) // zero and negative spans
		for _, gamma := range []float64{0, 0.1, 1} {
			want := computeRetrievalReference(accesses, now, windows, gamma, 0.5)
			got := ComputeRetrieval(accesses, now, windows, gamma, 0.5)
			require.Equal(t, want, got, "trial %d gamma %v", trial, gamma)
		}
		for _, at := range accesses {
			wa, wi := partitionTimeReference(at, now, windows)
			ga, gi := PartitionTime(at, now, windows)
			require.Equal(t, wa, ga)
			require.Equal(t, wi, gi)
		}
	}
}

// The caller's windows are left as they were: merging works on a copy.
func TestComputeRetrieval_LeavesTheWindowsUnchanged(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	windows := randomWindows(r, now, 50)
	before := append([]SessionWindow(nil), windows...)
	ComputeRetrieval([]time.Time{now.Add(-48 * time.Hour)}, now, windows, 0.1, 0.5)
	require.Equal(t, before, windows)
}

func BenchmarkComputeRetrieval(b *testing.B) {
	r := rand.New(rand.NewSource(1))
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	windows := randomWindows(r, now, 100)
	accesses := make([]time.Time, 20)
	for i := range accesses {
		accesses[i] = now.Add(-time.Duration(r.Int63n(int64(40 * 24 * time.Hour))))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeRetrieval(accesses, now, windows, 0.1, 0.5)
	}
}
