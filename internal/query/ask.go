package query

import (
	"context"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/rs/zerolog/log"
)

// AskConfig holds parameters for the Ask compound operation.
type AskConfig struct {
	Query            string
	Budget           int
	MaxItems         int
	SearchLimit      int
	ActivationScores map[string]float64
	// Embedder is optional. When non-nil, raw cosine similarities are computed
	// and used for spreading activation scoring via ActivationFunc.
	Embedder embedding.Embedder
	// ActivationFunc optionally recomputes activation scores after similarities
	// are known. When provided and similarities are available, the returned
	// scores replace ActivationScores for context-pack sorting. This enables
	// spreading activation without coupling query to the experiment package.
	ActivationFunc func(similarities map[string]float64) map[string]float64
}

// AskResult is the combined output of a search + context-pack operation.
// RetrievalMode reports which retriever lane actually ran ("hybrid" when
// embeddings exist and were used, "keyword" when ask fell back to FTS-only).
// Set by the caller after Ask returns, since Ask itself does not know which
// retriever it was handed — the caller (cmd/ask.go) has that signal.
// Surfacing it in the JSON envelope lets agent consumers detect keyword-only
// fallback programmatically (the human-readable hint prints on stdout and
// isn't available via --json).
//
// TopHitConfidence is one of "strong" | "moderate" | "weak" | "", derived
// from the normalized gap between the top hit's score and the next-lower
// score. First slice of plasticity-priority-order step 4 (calibrated
// confidence). Empirical thresholds from probes on identity + research
// vaults: canonical-match queries show ~8.6% relative gap; rank-2 / rank-6
// failure-mode queries show ~1.6-2% gap. The 5% / 2% cutoffs separate
// those populations on the small sample we have. See computeTopHitConfidence.
type AskResult struct {
	Query            string                    `json:"query"`
	TopHits          []retrieval.ScoredResult  `json:"top_hits"`
	Context          *memory.ContextPackResult `json:"context,omitempty"`
	RetrievalMode    string                    `json:"retrieval_mode,omitempty"`
	TopHitConfidence string                    `json:"top_hit_confidence,omitempty"`
	Similarities     map[string]float64        `json:"-"` // raw cosine similarities (not serialized)
}

// Confidence tier strings — exported so tests + future calibration tooling
// can reference them by name rather than string literals.
const (
	ConfidenceStrong   = "strong"
	ConfidenceModerate = "moderate"
	ConfidenceWeak     = "weak"
)

// Empirical thresholds on the relative gap between top-1 and top-2 scores,
// derived from probes on identity + research vaults at 2026-04-29:
//
//	canonical-match query "Hebbian learning" → top-1 dominates → gap 8.62%
//	low-confidence query (rank-6 miss)        → many close hits → gap 1.60%
//	low-confidence query (rank-2 case)        → anchor-density  → gap 1.94%
//
// 5% separates the strong-hit case from the borderline cases; 2% separates
// borderline from cases where top-1 might be coincidental and the agent
// should treat top-N as candidates rather than committing to top-1. Both
// cutoffs are tunable as more data accumulates.
const (
	confidenceStrongThreshold   = 0.05 // top1-top2 relative gap >= 5% → strong
	confidenceModerateThreshold = 0.02 // 2-5% → moderate; <2% → weak
)

// computeTopHitConfidence classifies the top hit's confidence based on the
// relative score gap to the next-ranked hit. Returns "" when fewer than 2
// hits exist (no comparison possible) or when top-1's score is non-positive
// (ill-defined denominator).
func computeTopHitConfidence(hits []retrieval.ScoredResult) string {
	if len(hits) < 2 {
		return ""
	}
	top := hits[0].Score
	if top <= 0 {
		return ""
	}
	gap := (top - hits[1].Score) / top
	switch {
	case gap >= confidenceStrongThreshold:
		return ConfidenceStrong
	case gap >= confidenceModerateThreshold:
		return ConfidenceModerate
	default:
		return ConfidenceWeak
	}
}

// Ask searches the vault for the query, computes raw cosine similarities
// (when an embedder is available), recomputes activation scores with
// spreading activation (via ActivationFunc), then packs token-budgeted
// context around the top hit. Context-pack failure is non-fatal.
func Ask(ctx context.Context, retriever retrieval.Retriever, resolver *graph.Resolver, db *index.DB, cfg AskConfig) (*AskResult, error) {
	hits, _, err := retriever.Search(ctx, cfg.Query, cfg.SearchLimit, 0, index.SearchFilters{})
	if err != nil {
		return nil, err
	}

	result := &AskResult{
		Query:            cfg.Query,
		TopHits:          hits,
		TopHitConfidence: computeTopHitConfidence(hits),
	}

	if len(hits) == 0 {
		return result, nil
	}

	// Compute raw cosine similarities for spreading activation.
	if cfg.Embedder != nil {
		sims, simErr := NoteSimilarities(ctx, cfg.Query, cfg.Embedder, db)
		if simErr != nil {
			log.Debug().Err(simErr).Msg("note similarities unavailable; spreading activation disabled for this query")
		} else {
			result.Similarities = sims
		}
	}

	// Recompute activation scores with similarity data when available.
	activationScores := cfg.ActivationScores
	if result.Similarities != nil && cfg.ActivationFunc != nil {
		if updated := cfg.ActivationFunc(result.Similarities); updated != nil {
			activationScores = updated
		}
	}

	packResult, packErr := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input:            hits[0].ID,
		Budget:           cfg.Budget,
		Depth:            1,
		MaxItems:         cfg.MaxItems,
		Slim:             true,
		ActivationScores: activationScores,
	})
	if packErr != nil {
		log.Debug().Err(packErr).Str("note_id", hits[0].ID).Msg("context-pack failed; returning search results only")
		return result, nil
	}

	result.Context = packResult
	return result, nil
}
