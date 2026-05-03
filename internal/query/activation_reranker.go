package query

import (
	"context"
	"sort"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
)

// ActivationScoreFn returns the activation score for each requested note id.
// Notes with no access events return 0 (or are absent from the map — the
// reranker treats both the same). Decoupling the scoring function from
// experiment.ComputeBatchScores at the type boundary keeps the reranker
// testable without spinning up an experiment DB and lets future scorers
// (e.g. activation-with-similarity, decay-only, count-only) compose
// without changing the reranker.
type ActivationScoreFn func(ids []string) (map[string]float64, error)

// ActivationReranker is slice 5b” — the post-RRF rerank that replaces
// slice 5b's 5th-lane approach. It wraps a base retriever (typically the
// 4-way HybridRetriever), takes the top-N candidates, and reorders them
// by blending RRF score with activation score.
//
// The structural property: activation operates only on candidates that
// already cleared the base retriever. It cannot introduce notes from
// outside the candidate set, so the activation drown-out that broke
// slice 5b' (mean-of-present treating activation-only single-lane as
// equivalent to multi-query-lane) is impossible by construction.
//
// See reference-activation-rerank-decision in the identity vault for
// the full probe sequence and the candidate-fix analysis that produced
// this design.
//
// Algorithm:
//
//  1. base.Search(query, FetchN) → N candidates with RRF scores
//  2. For each candidate c:
//     rrf_norm[c]        = c.Score / max_rrf
//     activation_raw[c]  = Score(c.ID)
//     activation_norm[c] = activation_raw[c] / max_activation
//     final[c]           = Alpha * rrf_norm[c] + Beta * activation_norm[c]
//  3. Sort candidates by final desc.
//  4. Return top-K (= the limit passed to Search).
//
// Zero-value Alpha/Beta/FetchN fall back to defaults that preserve the
// base retriever's behavior — a caller constructing the reranker with no
// explicit knobs gets safe behavior (rrf-only effective when β=0).
type ActivationReranker struct {
	Base   retrieval.Retriever
	Score  ActivationScoreFn
	Alpha  float64 // weight on RRF score, normalized to [0, 1]
	Beta   float64 // weight on activation score, normalized to [0, 1]
	FetchN int     // candidates to fetch from base; rerank operates on these
}

// Default constants. Pinned 2026-05-03 from the α/β probe documented in
// reference-activation-rerank-decision. The probe ran four pairs across
// the identity (n=19) and research (n=40) vaults:
//
//	α/β        identity ΔHit@5  identity ΔMRR  research ΔHit@5  research ΔMRR
//	0.5/0.5    -0.263           -0.304          0.000             -0.067
//	0.7/0.3     0.000           -0.193          0.000             -0.037
//	0.9/0.1     0.000           -0.053          0.000              0.000  ← winner
//	0.95/0.05   0.000           -0.027          0.000              0.000  ← safer, but β almost no-op
//
// 0.9/0.1 is the documented default: research stays at parity with 4-way
// while identity loses 0.053 MRR. Higher α (0.95) reduces identity loss
// but β becomes effectively a no-op. Lower α (0.7) damages identity too
// much. The trade-off is real and the data says broad-anchor vaults like
// identity (where reference-current-context, identity-who-i-am dominate
// access counts) are structurally harder for activation-in-retrieval.
const (
	defaultRerankAlpha  = 0.9
	defaultRerankBeta   = 0.1
	defaultRerankFetchN = 10
)

// Search fetches FetchN candidates from the base retriever, blends RRF
// score with activation score using Alpha/Beta weights, sorts by the
// combined score, and returns the top `limit` (with `offset` applied
// after sorting).
//
// Implements retrieval.Retriever so the reranker can slot into any
// pipeline that expects a Retriever — including being further wrapped
// by future cross-vault federation rerankers.
func (r *ActivationReranker) Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	alpha := r.Alpha
	beta := r.Beta
	fetchN := r.FetchN
	if alpha == 0 && beta == 0 {
		alpha = defaultRerankAlpha
		beta = defaultRerankBeta
	}
	if fetchN <= 0 {
		fetchN = defaultRerankFetchN
	}
	if limit > fetchN {
		fetchN = limit
	}

	candidates, _, err := r.Base.Search(ctx, query, fetchN, 0, filters)
	if err != nil {
		return nil, 0, err
	}
	if len(candidates) == 0 {
		return candidates, 0, nil
	}

	// Collect the IDs we need activation scores for.
	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = c.ID
	}
	activationScores, err := r.Score(ids)
	if err != nil {
		return nil, 0, err
	}

	// Rank-based RRF blending. Both lanes are scored on the same
	// reciprocal-rank scale (1/(K+rank+1) with K=60, mirroring RRF's
	// smoothing), so neither lane can dominate via scale asymmetry.
	//
	// Why rank-based, not score-normalized: score normalization to [0,1]
	// stretches activation's range to be equivalent to RRF's, which gave
	// activation crushing weight on broad anchor notes (probe data,
	// 2026-05-03 deep-dive). Rank-based blending bounds each lane's
	// contribution and dampens the broad-anchor drown-out.
	//
	// Activation rank: candidates are ranked by activation score within
	// the candidate set (1 = highest activation; N+1 = no activation).
	// Candidates with no activation (untouched) get the smallest possible
	// activation contribution (=1/(K+N+2)) — a tiny floor, not zero, so
	// they're still ordered consistently.
	const rrfK = 60
	type withAct struct {
		c   retrieval.ScoredResult
		act float64
	}
	withActs := make([]withAct, len(candidates))
	for i, c := range candidates {
		withActs[i] = withAct{c: c, act: activationScores[c.ID]}
	}
	// Sort by activation desc to assign activation ranks.
	sortByActivation := make([]int, len(withActs))
	for i := range sortByActivation {
		sortByActivation[i] = i
	}
	sort.SliceStable(sortByActivation, func(i, j int) bool {
		return withActs[sortByActivation[i]].act > withActs[sortByActivation[j]].act
	})
	activationRank := make(map[int]int, len(withActs))
	for rank, idx := range sortByActivation {
		activationRank[idx] = rank + 1 // 1-based
	}

	type scored struct {
		result retrieval.ScoredResult
		final  float64
	}
	scoredCandidates := make([]scored, len(candidates))
	for i, c := range candidates {
		rrfRank := i + 1 // 1-based; candidates already arrive in 4-way rank order
		rrfScore := 1.0 / float64(rrfK+rrfRank)
		var actScore float64
		if withActs[i].act > 0 {
			actScore = 1.0 / float64(rrfK+activationRank[i])
		} else {
			// Untouched note — minimal floor so blending stays continuous.
			actScore = 1.0 / float64(rrfK+len(candidates)+2)
		}
		final := alpha*rrfScore + beta*actScore
		c.Score = final
		scoredCandidates[i] = scored{result: c, final: final}
	}

	// Stable sort so 4-way order is preserved on ties. Critical when
	// activation is sparse (most candidates have activation_norm = 0)
	// and the rerank should fall back to base order rather than
	// reshuffle arbitrarily.
	sort.SliceStable(scoredCandidates, func(i, j int) bool {
		return scoredCandidates[i].final > scoredCandidates[j].final
	})

	total := len(scoredCandidates)
	if offset >= total {
		return nil, total, nil
	}
	out := make([]retrieval.ScoredResult, 0, len(scoredCandidates)-offset)
	for _, s := range scoredCandidates[offset:] {
		out = append(out, s.result)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, total, nil
}
