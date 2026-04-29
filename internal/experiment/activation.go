package experiment

import (
	"math"
	"sort"
	"time"
)

// MinElapsedHours is the minimum effective-elapsed value substituted when
// a note access's compressed elapsed time rounds to zero or below.
// Expressed as 1 second in hours so it's a soft floor that still makes
// the log(time) term in retrieval strength well-defined. Changing this
// shifts the retrieval score for very recent accesses.
const MinElapsedHours = 1.0 / 3600.0

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

	// Merge overlapping windows to avoid double-counting.
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
			hours = MinElapsedHours
		}
		sum += math.Pow(hours, -d)
	}
	if sum <= 0 {
		return 0.0
	}
	return math.Log(sum)
}

// ComputeStorage returns Si = ln(1 + access_count). Never decays.
// Returns 0.0 for non-positive counts.
func ComputeStorage(accessCount int) float64 {
	if accessCount <= 0 {
		return 0.0
	}
	return math.Log(1.0 + float64(accessCount))
}

// ComputeApproxRetrieval is the live-retrieval-path approximation of
// ComputeRetrieval. The notes table stores only (access_count,
// last_accessed_at) — a scalar count and a single timestamp — not the
// full per-event access history that ComputeRetrieval consumes. To use
// the same ACT-R math without a per-event access-times table, this
// approximator treats every recorded access as if it had happened at
// last_accessed_at, then defers to ComputeRetrieval. With all N
// timestamps equal to t, sum(t_k^(-d)) reduces to N*t^(-d), so the
// score collapses to ln(N) - d*ln(t) — preserving both the
// count-amplifies and elapsed-decays monotonic properties the ranking
// layer needs (Track A.4, slice 5b'). Returns 0 for the degenerate
// cases (no accesses, or no timestamp) so the caller doesn't have to
// guard against -Inf from log of zero or non-positive elapsed.
func ComputeApproxRetrieval(accessCount int, lastAccessedAt, now time.Time, windows []SessionWindow, gamma, d float64) float64 {
	if accessCount <= 0 || lastAccessedAt.IsZero() {
		return 0.0
	}
	accessTimes := make([]time.Time, accessCount)
	for i := range accessTimes {
		accessTimes[i] = lastAccessedAt
	}
	return ComputeRetrieval(accessTimes, now, windows, gamma, d)
}

// CombinedScore returns score = alpha * retrieval + beta * storage + delta * similarity.
// This implements the full ACT-R model: base-level activation (retrieval),
// dual-strength storage (Bjork), and spreading activation (similarity).
func CombinedScore(retrieval, storage, similarity, alpha, beta, delta float64) float64 {
	return alpha*retrieval + beta*storage + delta*similarity
}
