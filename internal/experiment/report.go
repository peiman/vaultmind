package experiment

import (
	"database/sql"
	"fmt"
)

// VariantMetrics holds computed metrics for a single variant.
type VariantMetrics struct {
	HitAtK     float64 `json:"hit_at_k"`
	MRR        float64 `json:"mrr"`
	EventCount int     `json:"event_count"`
}

// ReportResult holds the full experiment report.
type ReportResult struct {
	SessionCount int                       `json:"session_count"`
	EventCount   int                       `json:"event_count"`
	OutcomeCount int                       `json:"outcome_count"`
	K            int                       `json:"k"`
	Variants     map[string]VariantMetrics `json:"variants"`
}

// Report computes Hit@K and MRR for the given variants across all linkable events.
// variants is the list of variant names to report on; k is the rank cutoff for Hit@K.
func (d *DB) Report(variants []string, k int) (*ReportResult, error) {
	result := &ReportResult{
		K:        k,
		Variants: make(map[string]VariantMetrics),
	}

	if err := d.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&result.SessionCount); err != nil {
		return nil, fmt.Errorf("counting sessions: %w", err)
	}

	if err := d.db.QueryRow(
		`SELECT COUNT(*) FROM events WHERE event_type IN (?, ?, ?)`,
		EventSearch, EventAsk, EventContextPack,
	).Scan(&result.EventCount); err != nil {
		return nil, fmt.Errorf("counting linkable events: %w", err)
	}

	if err := d.db.QueryRow(`SELECT COUNT(*) FROM outcomes`).Scan(&result.OutcomeCount); err != nil {
		return nil, fmt.Errorf("counting outcomes: %w", err)
	}

	if result.EventCount == 0 {
		return result, nil
	}

	for _, variant := range variants {
		metrics, err := d.computeVariantMetrics(variant, k, result.EventCount)
		if err != nil {
			return nil, fmt.Errorf("computing metrics for variant %q: %w", variant, err)
		}
		result.Variants[variant] = metrics
	}

	return result, nil
}

// DistinctVariants returns the sorted list of distinct variant names in the outcomes table.
func (d *DB) DistinctVariants() ([]string, error) {
	rows, err := d.db.Query(`SELECT DISTINCT variant FROM outcomes ORDER BY variant`)
	if err != nil {
		return nil, fmt.Errorf("querying distinct variants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var variants []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scanning variant: %w", err)
		}
		variants = append(variants, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating variant rows: %w", err)
	}
	return variants, nil
}

// computeVariantMetrics computes Hit@K and MRR for a single variant across all
// linkable events. totalEvents is the pre-computed count of linkable events.
func (d *DB) computeVariantMetrics(variant string, k, totalEvents int) (VariantMetrics, error) {
	// Fetch all linkable event IDs.
	rows, err := d.db.Query(
		`SELECT event_id FROM events WHERE event_type IN (?, ?, ?)`,
		EventSearch, EventAsk, EventContextPack,
	)
	if err != nil {
		return VariantMetrics{}, fmt.Errorf("querying linkable event ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var eventIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return VariantMetrics{}, fmt.Errorf("scanning event id: %w", err)
		}
		eventIDs = append(eventIDs, id)
	}
	if err := rows.Err(); err != nil {
		return VariantMetrics{}, fmt.Errorf("iterating event id rows: %w", err)
	}

	hitCount := 0
	reciprocalRankSum := 0.0

	for _, eventID := range eventIDs {
		var bestRank sql.NullInt64
		err := d.db.QueryRow(
			`SELECT MIN(rank) FROM outcomes WHERE event_id = ? AND variant = ?`,
			eventID, variant,
		).Scan(&bestRank)
		if err != nil {
			return VariantMetrics{}, fmt.Errorf("querying best rank for event %s: %w", eventID, err)
		}
		if !bestRank.Valid || bestRank.Int64 <= 0 {
			continue // no outcome for this event+variant
		}
		rank := int(bestRank.Int64)
		if rank <= k {
			hitCount++
		}
		reciprocalRankSum += 1.0 / float64(rank)
	}

	metrics := VariantMetrics{
		HitAtK:     float64(hitCount) / float64(totalEvents),
		MRR:        reciprocalRankSum / float64(totalEvents),
		EventCount: totalEvents,
	}
	return metrics, nil
}
