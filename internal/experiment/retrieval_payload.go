package experiment

// RetrievalHit is the input shape for building a variant payload: a single
// retrieved note with its rank, aggregated score, and optional per-component
// scores (e.g. fts/dense/sparse/colbert for hybrid retrieval).
//
// Storage/privacy note: payloads built from RetrievalHit are written to the
// local experiment DB only. The anonymous/full telemetry tiers are identical
// in the current (local-only) write path — see cmd/root.go. A future uploader
// MUST filter by tier: anonymous should strip note_id/path; full may send
// everything.
type RetrievalHit struct {
	NoteID   string
	Rank     int
	Score    float64
	NoteType string
	Path     string
	Scores   map[string]float64
}

// BuildRetrievalEventData composes the base event_data payload for a
// retrieval event. resultCount is the total number of hits the retrieval
// reported (may differ from len(variants.*.results) when limits truncate).
// When err is non-nil, an "error" field is added so downstream consumers can
// distinguish genuine zero-hit retrievals (curiosity signal) from crashes
// (error signal). The error field is omitted on success, including the
// zero-hit success case.
func BuildRetrievalEventData(variants map[string]any, resultCount int, err error) map[string]any {
	data := map[string]any{
		"variants":     variants,
		"result_count": resultCount,
	}
	if err != nil {
		data["error"] = err.Error()
	}
	return data
}

// BuildVariantPayload produces the event_data.variants substructure for one
// retrieval variant. The shape matches what LinkOutcomes walks:
//
//	{ variant: { results: [ { note_id, rank, score_final, note_type, path, scores? } ] } }
//
// scores is omitted when the hit has no per-component breakdown.
func BuildVariantPayload(variant string, hits []RetrievalHit) map[string]any {
	results := make([]any, 0, len(hits))
	for _, h := range hits {
		r := map[string]any{
			"note_id":     h.NoteID,
			"rank":        h.Rank,
			"score_final": h.Score,
			"note_type":   h.NoteType,
			"path":        h.Path,
		}
		if len(h.Scores) > 0 {
			r["scores"] = h.Scores
		}
		results = append(results, r)
	}
	return map[string]any{
		variant: map[string]any{"results": results},
	}
}
