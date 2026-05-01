package experiment

import (
	"encoding/json"
	"fmt"
	"io"
)

// VariantStats summarizes one variant's performance across all
// retrieval events in the experiment DB. The fields are exactly what
// the federated paper (reference-paper-federated-constants) needs to
// answer H2.5: which retrieval constants vary meaningfully across
// vaults vs which are stable.
//
// Reciprocal-rank semantics:
//   - For every event whose primary_variant equals this variant AND
//     for which an outcome row was logged (i.e. a note that was retrieved
//     was later actually accessed), we contribute (1 / rank) to the
//     reciprocal-rank sum. MRR = sum / event_count.
//   - Hit@K is the fraction of primary-variant events where the
//     accessed-note's rank is ≤ K.
//
// This is the conservative interpretation: shadow variants do not get
// MRR/Hit@K credit because we don't know what the user "would have"
// accessed under that variant — only what they actually accessed under
// the primary. The federated dataset still recovers shadow signal via
// the side-by-side rank distributions in the variants payload.
type VariantStats struct {
	Name         string  `json:"name"`
	EventCount   int     `json:"event_count"`
	OutcomeCount int     `json:"outcome_count"`
	HitAt5       float64 `json:"hit_at_5"`
	HitAt10      float64 `json:"hit_at_10"`
	MRR          float64 `json:"mrr"`
}

// VariantPerformance computes per-variant stats over the entire
// experiment DB. Returns one entry per distinct primary_variant
// observed in events. The map is keyed by variant name; iteration
// order is not guaranteed — callers that need deterministic output
// must sort by name themselves.
func VariantPerformance(db *DB) (map[string]*VariantStats, error) {
	stats := map[string]*VariantStats{}

	rows, err := db.db.Query(`
		SELECT primary_variant, COUNT(*)
		FROM events
		WHERE primary_variant IS NOT NULL AND primary_variant <> ''
		GROUP BY primary_variant`)
	if err != nil {
		return nil, fmt.Errorf("event-count by variant: %w", err)
	}
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan event-count row: %w", err)
		}
		stats[name] = &VariantStats{Name: name, EventCount: n}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	// Outcomes carry the variant + rank per (event, accessed-note)
	// combination; join with events to filter by primary_variant.
	outcomes, err := db.db.Query(`
		SELECT o.variant, o.rank
		FROM outcomes o
		JOIN events e ON e.event_id = o.event_id
		WHERE e.primary_variant IS NOT NULL AND e.primary_variant <> ''
		  AND o.variant = e.primary_variant`)
	if err != nil {
		return nil, fmt.Errorf("outcome rows: %w", err)
	}
	for outcomes.Next() {
		var name string
		var rank int
		if err := outcomes.Scan(&name, &rank); err != nil {
			_ = outcomes.Close()
			return nil, fmt.Errorf("scan outcome row: %w", err)
		}
		s, ok := stats[name]
		if !ok {
			// Defensive: outcome references a variant with no events row.
			continue
		}
		s.OutcomeCount++
		if rank > 0 {
			s.MRR += 1.0 / float64(rank)
		}
		if rank > 0 && rank <= 5 {
			s.HitAt5++
		}
		if rank > 0 && rank <= 10 {
			s.HitAt10++
		}
	}
	if err := outcomes.Close(); err != nil {
		return nil, err
	}

	// Convert running sums to means. Use OutcomeCount as denominator
	// for MRR/Hit@K — events without outcomes don't contribute (we
	// can't measure success without ground-truth access signal).
	for _, s := range stats {
		if s.OutcomeCount == 0 {
			continue
		}
		s.MRR = s.MRR / float64(s.OutcomeCount)
		s.HitAt5 = s.HitAt5 / float64(s.OutcomeCount)
		s.HitAt10 = s.HitAt10 / float64(s.OutcomeCount)
	}

	return stats, nil
}

// Rollup is the federated-aggregator-shaped payload — one record per
// vault that captures everything the paper (Paper #2 from
// reference-paper-federated-constants) needs without exposing content.
//
// Designed to be small, content-free, and stable across schema
// versions. SchemaVersion tracks payload shape so receivers can
// dispatch on it.
type Rollup struct {
	Kind             string                   `json:"kind"`
	SchemaVersion    int                      `json:"schema_version"`
	Tier             string                   `json:"tier"`
	Fingerprint      string                   `json:"vault_fingerprint"`
	NoteCount        int                      `json:"note_count"`
	TypeDistribution map[string]int           `json:"type_distribution"`
	LinkCount        int                      `json:"link_count"`
	AliasCount       int                      `json:"alias_count"`
	EmbeddingCount   int                      `json:"embedding_count"`
	EmbeddingDims    int                      `json:"embedding_dims"`
	VariantStats     map[string]*VariantStats `json:"variant_stats"`
	ExportedAt       string                   `json:"exported_at"`
	SessionCount     int                      `json:"session_count"`
	EventCount       int                      `json:"event_count"`
	OutcomeCount     int                      `json:"outcome_count"`
}

// MarshalJSON pins the field order via custom marshaling for
// human-readable preview output. The standard library marshaler would
// alphabetize; we want kind/version/tier first, fingerprint next,
// then aggregate features, then variants. Receivers that care about
// order can rely on the JSON itself; receivers that don't, ignore.
func (r *Rollup) MarshalJSON() ([]byte, error) {
	type alias Rollup
	return json.Marshal((*alias)(r))
}

// CountSessions returns the total number of session rows. Used by
// rollup assembly so the receiver can sanity-check the aggregation
// horizon (e.g. is this vault active or stale?).
func (d *DB) CountSessions() (int, error) {
	var n int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n)
	return n, err
}

// CountEvents returns the total number of event rows.
func (d *DB) CountEvents() (int, error) {
	var n int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n)
	return n, err
}

// CountOutcomes returns the total number of outcome rows.
func (d *DB) CountOutcomes() (int, error) {
	var n int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM outcomes`).Scan(&n)
	return n, err
}

// WriteRollup emits a single Rollup record as one indented JSON
// object. Unlike ExportToJSONL (which emits a stream of records),
// the rollup IS the unit of analysis — one record per vault — so
// pretty-printed JSON is the right shape for both machines and
// humans inspecting the payload before transmission.
func WriteRollup(r *Rollup, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return fmt.Errorf("write rollup: %w", err)
	}
	return nil
}
