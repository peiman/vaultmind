package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/index"
	"golang.org/x/sync/errgroup"
)

// HybridRetriever fuses results from N retrievers using Reciprocal Rank Fusion.
type HybridRetriever struct {
	Retrievers []Retriever
	K          int // RRF constant, default 60
}

type rrfEntry struct {
	result ScoredResult
	score  float64
}

// Search runs all retrievers concurrently, then fuses their ranked lists via RRF.
func (h *HybridRetriever) Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]ScoredResult, int, error) {
	k := h.K
	if k <= 0 {
		k = 60
	}

	if len(h.Retrievers) == 0 {
		return nil, 0, nil
	}

	// Fetch a generous number of results from each retriever for good fusion.
	fetchLimit := limit + offset
	if fetchLimit < 100 {
		fetchLimit = 100
	}

	type retrieverResult struct {
		results []ScoredResult
	}

	perRetriever := make([]retrieverResult, len(h.Retrievers))

	g, gCtx := errgroup.WithContext(ctx)
	for i, ret := range h.Retrievers {
		g.Go(func() error {
			results, _, err := ret.Search(gCtx, query, fetchLimit, 0, filters)
			if err != nil {
				return fmt.Errorf("retriever %d: %w", i, err)
			}
			perRetriever[i] = retrieverResult{results: results}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, 0, err
	}

	// Compute RRF scores: for each note, sum 1/(K+rank) across all retrievers
	rrfScores := make(map[string]*rrfEntry)
	for _, rr := range perRetriever {
		for rank, result := range rr.results {
			rrfScore := 1.0 / float64(k+rank+1) // rank is 0-based, RRF uses 1-based
			if entry, ok := rrfScores[result.ID]; ok {
				entry.score += rrfScore
			} else {
				rrfScores[result.ID] = &rrfEntry{
					result: result,
					score:  rrfScore,
				}
			}
		}
	}

	// Sort by RRF score descending
	entries := make([]rrfEntry, 0, len(rrfScores))
	for _, e := range rrfScores {
		entries = append(entries, *e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].score > entries[j].score
	})

	total := len(entries)

	// Apply offset/limit
	if offset >= len(entries) {
		return nil, total, nil
	}
	entries = entries[offset:]
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	results := make([]ScoredResult, len(entries))
	for i, e := range entries {
		results[i] = e.result
		results[i].Score = e.score
	}

	return results, total, nil
}
