package experiment

import (
	"fmt"
	"sort"
)

// UsageSummary reports aggregate memory-usage metrics derived from the
// experiment DB — intended as the "weekly readout" view for someone
// dogfooding VaultMind: which notes are being recalled, how often, and how
// sessions are spaced over time.
type UsageSummary struct {
	TotalSessions       int
	RetrievalEventCount int
	UniqueNotesRecalled int
	TopNotes            []NoteStat
	GapStats            GapStats
}

// GapStats summarizes inter-session intervals (seconds). Count is the number
// of gaps — always TotalSessions-1, or 0 when fewer than 2 sessions exist.
type GapStats struct {
	Count         int
	MedianSeconds int64
	P90Seconds    int64
	MaxSeconds    int64
}

// UsageSummary computes the aggregate metrics in one pass across the DB.
// topN caps the TopNotes slice; 0 means "all recalled notes." topN is applied
// after the per_note_stats view's own ordering (count desc, recency desc).
func (d *DB) UsageSummary(topN int) (*UsageSummary, error) {
	out := &UsageSummary{}

	if err := d.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&out.TotalSessions); err != nil {
		return nil, fmt.Errorf("counting sessions: %w", err)
	}
	if err := d.db.QueryRow(
		`SELECT COUNT(*) FROM events WHERE event_type IN ('search', 'ask', 'context_pack')`,
	).Scan(&out.RetrievalEventCount); err != nil {
		return nil, fmt.Errorf("counting retrieval events: %w", err)
	}

	notes, err := d.PerNoteStats()
	if err != nil {
		return nil, err
	}
	out.UniqueNotesRecalled = len(notes)
	if topN > 0 && len(notes) > topN {
		out.TopNotes = notes[:topN]
	} else {
		out.TopNotes = notes
	}

	gaps, err := d.SessionGaps()
	if err != nil {
		return nil, err
	}
	out.GapStats = computeGapStats(gaps)

	return out, nil
}

// computeGapStats pulls gap_seconds from SessionGaps rows, filters out the
// first-session NULL, and computes median / p90 / max. Empty input and
// single-session input both produce a zero-valued GapStats.
func computeGapStats(gaps []SessionGap) GapStats {
	values := make([]int64, 0, len(gaps))
	for _, g := range gaps {
		if g.GapSeconds.Valid {
			values = append(values, g.GapSeconds.Int64)
		}
	}
	if len(values) == 0 {
		return GapStats{}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return GapStats{
		Count:         len(values),
		MedianSeconds: percentile(values, 0.5),
		P90Seconds:    percentile(values, 0.9),
		MaxSeconds:    values[len(values)-1],
	}
}

// percentile returns the p-th percentile of a pre-sorted slice using the
// nearest-rank method (ceil). Caller guarantees len(sorted) >= 1.
func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int((p * float64(len(sorted))) + 0.9999999999) // ceil
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}
