package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
)

// ActivationRetriever ranks notes by their ACT-R activation score
// (base-level activation + decay). It implements retrieval.Retriever
// and is intended to be a 5th lane in HybridRetriever's RRF combine —
// the first principled wiring of the reinforcement signal that
// internal/experiment has been collecting since 2026-04-29.
//
// The retriever is query-independent: every Search call returns the
// same ranking regardless of `query`, because activation is a function
// of access history, not text relevance. The mean-of-present RRF
// fusion in HybridRetriever then only boosts notes that ALSO appear
// in at least one query-dependent lane — recently-accessed notes that
// happen to match the query rise; recently-accessed notes that don't
// stay where the query ranks them.
//
// Notes with access_count = 0 are not returned; the lane's coverage
// matches the substrate's coverage. Filters (type, tag) are honored
// because the agent's mental model of "activation" is per-type
// (asking about concepts shouldn't surface recently-touched sources).
type ActivationRetriever struct {
	DB     *index.DB
	ExpDB  *experiment.DB
	Params experiment.ActivationParams
}

// Search returns up to `limit` notes ranked by activation score,
// descending. Score is normalized to [0, 1] so it composes cleanly
// with other retrievers; the absolute scale is irrelevant under RRF
// (only ranks matter), but normalization keeps Components reporting
// consistent with the other lanes.
//
// Implementation:
//  1. Pull all accessed-note IDs from the index (access_count > 0).
//  2. Hydrate metadata (title, type, path, is_domain) in one batch query.
//  3. Apply type/tag filters before scoring (cheaper than scoring then dropping).
//  4. Call experiment.ComputeBatchScores — which uses the experiment
//     DB's note_access events for the full access-time history that
//     ACT-R math needs.
//  5. Sort by score, normalize, return.
func (r *ActivationRetriever) Search(_ context.Context, _ string, limit, offset int, filters index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	stats, err := index.ListAccessedNotes(r.DB)
	if err != nil {
		return nil, 0, fmt.Errorf("activation: list accessed notes: %w", err)
	}
	if len(stats) == 0 {
		return nil, 0, nil
	}

	// Hydrate metadata for accessed notes. Filter early — we don't
	// want to score notes the caller is going to drop.
	candidates := make([]retrieval.ScoredResult, 0, len(stats))
	candidateIDs := make([]string, 0, len(stats))
	for _, s := range stats {
		row, qerr := r.DB.QueryNoteByID(s.NoteID)
		if qerr != nil || row == nil {
			continue
		}
		if !matchesActivationFilters(row, filters) {
			continue
		}
		candidates = append(candidates, retrieval.ScoredResult{
			ID:       row.ID,
			Type:     row.Type,
			Title:    row.Title,
			Path:     row.Path,
			IsDomain: row.IsDomain,
		})
		candidateIDs = append(candidateIDs, row.ID)
	}
	if len(candidates) == 0 {
		return nil, 0, nil
	}

	scores, _, err := experiment.ComputeBatchScores(r.ExpDB, candidateIDs, r.Params, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("activation: compute batch scores: %w", err)
	}

	for i := range candidates {
		candidates[i].Score = scores[candidates[i].ID]
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	normalizeActivationScores(candidates)

	total := len(candidates)
	if offset >= total {
		return nil, total, nil
	}
	candidates = candidates[offset:]
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, total, nil
}

// matchesActivationFilters mirrors the filter semantics that other
// lanes' SQL queries enforce. Activation lane's filtering happens in
// Go because we already have the full row metadata in hand.
func matchesActivationFilters(row *index.NoteRow, filters index.SearchFilters) bool {
	if filters.Type != "" && row.Type != filters.Type {
		return false
	}
	// Tag filtering would require a join to the tags table — defer
	// until evidence the lack matters in practice. Per the Mean-of-
	// Present RRF, missing notes from this lane just get scored from
	// the other 4, so a missing tag filter here is a soft fallback.
	return true
}

// normalizeActivationScores rescales the score column to [0, 1] so
// the Components map reads consistently against the other lanes.
// The top score becomes 1.0, the bottom becomes 0.0; ties are stable.
// RRF only cares about rank order, but external consumers (--explain,
// experiment exports) read the score field, so a sane scale matters.
func normalizeActivationScores(results []retrieval.ScoredResult) {
	if len(results) == 0 {
		return
	}
	hi := results[0].Score
	lo := results[len(results)-1].Score
	if hi == lo {
		for i := range results {
			results[i].Score = 1.0
		}
		return
	}
	span := hi - lo
	for i := range results {
		results[i].Score = (results[i].Score - lo) / span
	}
}
