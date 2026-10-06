package experiment

import (
	"math"
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

// computeRetrievalReference is ComputeRetrieval as it was, verbatim.
func computeRetrievalReference(accessTimes []time.Time, now time.Time, windows []SessionWindow, gamma, d float64) float64 {
	if len(accessTimes) == 0 {
		return 0.0
	}
	var sum float64
	for _, at := range accessTimes {
		active, idle := partitionTimeReference(at, now, windows)
		effective := CompressedElapsed(active, idle, gamma)
		hours := effective.Hours()
		if hours <= 0 {
			hours = MinElapsedHours
		}
		sum += math.Pow(hours, -d)
	}
	if sum <= 0 {
		return 0.0
	}
	return math.Log(sum)
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

// A batch scored with the windows merged once gives exactly the scores and
// features of scoring each note with its own per-access merging.
func TestScoreFromData_EqualsPerAccessMerging(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	for trial := 0; trial < 50; trial++ {
		windows := randomWindows(r, now, r.Intn(101))
		ids := []string{"never-read"}
		access := map[string][]time.Time{}
		for n := 0; n < 20; n++ {
			id := string(rune('a' + n))
			ids = append(ids, id)
			for i := 0; i < 1+r.Intn(30); i++ {
				access[id] = append(access[id], now.Add(-time.Duration(r.Int63n(int64(40*24*time.Hour)))))
			}
		}
		params := DefaultActivationParamsWithSimilarity(0.1)
		sims := map[string]float64{"a": 0.7, "never-read": 0.4}
		wantScores, wantFeatures := scoreFromDataReference(ids, access, windows, now, params, sims)
		scores, features := ScoreFromData(ids, access, windows, now, params, sims)
		require.Equal(t, wantScores, scores)
		require.Equal(t, wantFeatures, features)
	}
}

// scoreFromDataReference is ScoreFromData as it was, verbatim, scoring each
// note through the per-access merging reference.
func scoreFromDataReference(noteIDs []string, accessMap map[string][]time.Time, windows []SessionWindow, now time.Time, params ActivationParams, similarities map[string]float64) (map[string]float64, map[string]map[string]float64) {
	scores := make(map[string]float64, len(noteIDs))
	features := make(map[string]map[string]float64, len(noteIDs))
	for _, noteID := range noteIDs {
		accessTimes := accessMap[noteID]
		sim := 0.0
		if similarities != nil {
			sim = similarities[noteID]
		}
		if len(accessTimes) == 0 {
			scores[noteID] = CombinedScore(0.0, 0.0, sim, params.Alpha, params.Beta, params.Delta)
			features[noteID] = map[string]float64{"retrieval_strength": 0.0, "storage_strength": 0.0, "similarity": sim, "access_count": 0.0}
			continue
		}
		retrieval := computeRetrievalReference(accessTimes, now, windows, params.Gamma, params.D)
		storage := ComputeStorage(len(accessTimes))
		score := CombinedScore(retrieval, storage, sim, params.Alpha, params.Beta, params.Delta)
		scores[noteID] = score
		features[noteID] = map[string]float64{"retrieval_strength": retrieval, "storage_strength": storage, "similarity": sim, "access_count": float64(len(accessTimes))}
	}
	return scores, features
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
