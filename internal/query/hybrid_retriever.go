package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
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
//
// Fusion uses mean-of-present RRF: a note's score is the mean of its
// 1/(K+rank) contributions across the lanes where it appeared. Absence
// from a lane is treated as missing data, not a zero score. This is the
// fix for the 2026-04-24 partial-coverage compression bug where newly-
// added notes missing sparse/colbert embeddings lost to ubiquitous-
// mediocre competitors even when they were rank 1 where they scored.
//
// In small vaults (fetchLimit ≥ note count) this is unambiguously
// correct: absence from a lane's results means the modality wasn't
// computed for that note. In very large vaults (where fetchLimit
// truncates a lane's tail) absence could also mean "ranked below
// cutoff" — a different failure mode worth separate attention, not
// solved here.
type HybridRetriever struct {
	Retrievers []retrieval.NamedRetriever
	K          int // RRF smoothing constant; zero-value falls back to DefaultRRFK
}

type rrfEntry struct {
	result     retrieval.ScoredResult
	score      float64
	components map[string]float64
	// sectionRank is the best rank at which a lane matched one of this long
	// note's sections; the fused hit names that section. -1 while none has.
	sectionRank int
}

// Search runs all retrievers concurrently, then fuses their ranked lists via RRF.
func (h *HybridRetriever) Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
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
		results []retrieval.ScoredResult
	}

	perRetriever := make([]retrieverResult, len(h.Retrievers))

	// Two phases. Lanes that must see every note run first, in parallel; a lane
	// that can score a given set (ColBERT) then scores only what they found.
	// ColBERT's per-token MaxSim over every note was most of a search; the
	// other lanes' candidates are a fraction of the vault (163 of 416). On 32
	// labelled real queries (`vaultmind eval`) it was no worse than scoring every
	// note — Hit@1 27 -> 28, Hit@5 30 -> 30, MRR 0.888 -> 0.903 — and 1.3s faster.
	// It does move results: under mean-of-present fusion a low-ranked candidate's
	// extra ColBERT vote lowers that note's score, so top-5 sets differ, among
	// notes the labels mark not relevant.
	run := func(indices []int, candidates []string) error {
		g, gCtx := errgroup.WithContext(ctx)
		for _, i := range indices {
			nr := h.Retrievers[i]
			g.Go(func() error {
				var (
					results []retrieval.ScoredResult
					err     error
				)
				if cs, ok := nr.Retriever.(CandidateSearcher); ok && len(candidates) > 0 {
					results, _, err = cs.SearchAmong(gCtx, query, fetchLimit, filters, candidates)
				} else {
					results, _, err = nr.Retriever.Search(gCtx, query, fetchLimit, 0, filters)
				}
				if err != nil {
					return fmt.Errorf("retriever %s: %w", nr.Name, err)
				}
				perRetriever[i] = retrieverResult{results: results}
				return nil
			})
		}
		return g.Wait()
	}
	var direct, narrowed []int
	for i, nr := range h.Retrievers {
		if _, ok := nr.Retriever.(CandidateSearcher); ok {
			narrowed = append(narrowed, i)
		} else {
			direct = append(direct, i)
		}
	}
	if err := run(direct, nil); err != nil {
		return nil, 0, err
	}
	if len(narrowed) > 0 {
		// Empty when the other lanes found nothing: the lane then searches
		// everything rather than returning nothing.
		lane := func(i int) []retrieval.ScoredResult { return perRetriever[i].results }
		if err := run(narrowed, candidateIDs(lane, direct)); err != nil {
			return nil, 0, err
		}
	}

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
				// A long note matched by section: name the section from the
				// lane that ranked it best, with that section's snippet. The
				// keyword lane matches the whole note and names none.
				if result.Section != "" && (entry.sectionRank < 0 || rank < entry.sectionRank) {
					entry.result.Section, entry.result.Snippet = result.Section, result.Snippet
					entry.sectionRank = rank
				}
			} else {
				sectionRank := -1
				if result.Section != "" {
					sectionRank = rank
				}
				rrfScores[result.ID] = &rrfEntry{
					result:      result,
					score:       rrfScore,
					components:  map[string]float64{name: rrfScore},
					sectionRank: sectionRank,
				}
			}
		}
	}

	// Mean-of-present normalization: divide each note's raw RRF sum by
	// the number of lanes it actually appeared in. See type doc above.
	for _, e := range rrfScores {
		if n := len(e.components); n > 0 {
			e.score /= float64(n)
		}
	}

	// Sort by RRF score descending. Notes at the same rank in different lanes
	// score exactly alike, and entries come from a map, so ties break by id —
	// otherwise the same query ranked them differently on every run (#208).
	entries := make([]rrfEntry, 0, len(rrfScores))
	for _, e := range rrfScores {
		entries = append(entries, *e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].score != entries[j].score {
			return entries[i].score > entries[j].score
		}
		return entries[i].result.ID < entries[j].result.ID
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

	results := make([]retrieval.ScoredResult, len(entries))
	for i, e := range entries {
		results[i] = e.result
		results[i].Score = e.score
		results[i].Components = e.components
	}

	return results, total, nil
}

// CandidateSearcher is a lane that can score a given set of notes instead of
// the whole vault. The hybrid retriever hands it the other lanes' results.
type CandidateSearcher interface {
	SearchAmong(ctx context.Context, query string, limit int, filters index.SearchFilters, ids []string) ([]retrieval.ScoredResult, int, error)
}

// candidateIDs is the union of the ids the given lanes returned, in first-seen
// order.
func candidateIDs(lane func(int) []retrieval.ScoredResult, indices []int) []string {
	seen := map[string]bool{}
	var ids []string
	for _, i := range indices {
		for _, r := range lane(i) {
			if !seen[r.ID] {
				seen[r.ID] = true
				ids = append(ids, r.ID)
			}
		}
	}
	return ids
}
