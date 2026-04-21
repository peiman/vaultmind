package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/index"
	"golang.org/x/sync/errgroup"
)

// DefaultRRFK is the Reciprocal Rank Fusion smoothing constant from the
// original RRF paper. Applied when HybridRetriever.K is zero-value.
// Tuning this value shifts every hybrid retrieval's ranking; it lives
// here so there's one home to reason about.
const DefaultRRFK = 60

// HybridRetriever fuses results from N retrievers using Reciprocal Rank Fusion.
// Sub-retrievers are named so each note's Components map reports which
// sub-retriever contributed what — useful for studying the 4-way RRF
// contribution ("what did FTS add here vs dense?") during research.
type HybridRetriever struct {
	Retrievers []NamedRetriever
	K          int // RRF smoothing constant; zero-value falls back to DefaultRRFK
}

type rrfEntry struct {
	result     ScoredResult
	score      float64
	components map[string]float64
}

// Search runs all retrievers concurrently, then fuses their ranked lists via RRF.
func (h *HybridRetriever) Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]ScoredResult, int, error) {
	k := h.K
	if k <= 0 {
		k = DefaultRRFK
	}

	if len(h.Retrievers) == 0 {
		return nil, 0, nil
	}

	// Fetch a generous number of results from each retriever for good fusion.
	// minFusionCandidates ensures enough overlap between retriever result sets
	// for RRF to produce meaningful combined rankings.
	const minFusionCandidates = 100
	fetchLimit := limit + offset
	if fetchLimit < minFusionCandidates {
		fetchLimit = minFusionCandidates
	}

	type retrieverResult struct {
		results []ScoredResult
	}

	perRetriever := make([]retrieverResult, len(h.Retrievers))

	g, gCtx := errgroup.WithContext(ctx)
	for i, nr := range h.Retrievers {
		g.Go(func() error {
			results, _, err := nr.Retriever.Search(gCtx, query, fetchLimit, 0, filters)
			if err != nil {
				return fmt.Errorf("retriever %s: %w", nr.Name, err)
			}
			perRetriever[i] = retrieverResult{results: results}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, 0, err
	}

	// Compute RRF scores: for each note, sum 1/(K+rank) across all retrievers.
	// Also track the per-retriever contribution keyed by retriever name.
	rrfScores := make(map[string]*rrfEntry)
	for i, rr := range perRetriever {
		name := h.Retrievers[i].Name
		for rank, result := range rr.results {
			rrfScore := 1.0 / float64(k+rank+1) // rank is 0-based, RRF uses 1-based
			if entry, ok := rrfScores[result.ID]; ok {
				entry.score += rrfScore
				entry.components[name] = rrfScore
				// Prefer result with non-empty snippet for display
				if entry.result.Snippet == "" && result.Snippet != "" {
					entry.result.Snippet = result.Snippet
				}
			} else {
				rrfScores[result.ID] = &rrfEntry{
					result:     result,
					score:      rrfScore,
					components: map[string]float64{name: rrfScore},
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
		results[i].Components = e.components
	}

	return results, total, nil
}
