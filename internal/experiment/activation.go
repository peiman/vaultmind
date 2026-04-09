package experiment

import (
	"math"
	"sort"
	"time"
)

// SessionWindow represents a period when VaultMind was actively in use.
type SessionWindow struct {
	Start time.Time
	End   time.Time
}

// PartitionTime splits the duration [start, end] into active (overlapping sessions)
// and idle (gaps between sessions). Windows need not be sorted.
func PartitionTime(start, end time.Time, windows []SessionWindow) (active, idle time.Duration) {
	total := end.Sub(start)
	if total <= 0 {
		return 0, 0
	}
	sorted := make([]SessionWindow, len(windows))
	copy(sorted, windows)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Start.Before(sorted[j].Start)
	})
	for _, w := range sorted {
		wStart := w.Start
		if wStart.Before(start) {
			wStart = start
		}
		wEnd := w.End
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

// CompressedElapsed returns tj_effective = active + gamma * idle.
func CompressedElapsed(active, idle time.Duration, gamma float64) time.Duration {
	effectiveNanos := float64(active) + gamma*float64(idle)
	return time.Duration(effectiveNanos)
}

// ComputeRetrieval computes Bi = ln(sum(tj_effective^(-d))).
// Uses session windows to partition each access-to-now interval.
// Returns 0.0 for empty accessTimes.
func ComputeRetrieval(accessTimes []time.Time, now time.Time, windows []SessionWindow, gamma, d float64) float64 {
	if len(accessTimes) == 0 {
		return 0.0
	}
	var sum float64
	for _, at := range accessTimes {
		active, idle := PartitionTime(at, now, windows)
		effective := CompressedElapsed(active, idle, gamma)
		hours := effective.Hours()
		if hours <= 0 {
			hours = 1.0 / 3600.0 // minimum 1 second
		}
		sum += math.Pow(hours, -d)
	}
	if sum <= 0 {
		return 0.0
	}
	return math.Log(sum)
}

// ComputeStorage returns Si = ln(1 + access_count). Never decays.
func ComputeStorage(accessCount int) float64 {
	return math.Log(1.0 + float64(accessCount))
}

// CombinedScore returns score = alpha * retrieval + beta * storage.
func CombinedScore(retrieval, storage, alpha, beta float64) float64 {
	return alpha*retrieval + beta*storage
}
