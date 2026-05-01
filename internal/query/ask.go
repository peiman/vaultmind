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
	// ConfidenceNoMatch — top results are essentially tied. The agent
	// should treat the result list as "no clear winner" rather than
	// committing to top-1. Added 2026-04-30 after a fresh-session
	// evaluation found that nonsense queries silently landed "weak" with
	// the same shape as borderline real queries — there was no return
	// state for "I don't know."
	ConfidenceNoMatch = "no_match"
)

// Empirical thresholds on the relative gap between top-1 and top-2 scores.
// Re-probed 2026-04-30 against the post-sleep-wave research vault (391+
// notes) after a different agent's evaluation surfaced two failure modes
// in the original 5%/2% calibration:
//
//  1. "Spreading activation" — a clearly canonical query — landed at 1.97%
//     gap and was binned "weak", same label as nonsense. False negative.
//  2. Nonsense queries ("the cake is a lie", "what's the weather") landed
//     at 0–0.7% gap, also "weak". No way to distinguish "borderline real"
//     from "no real signal at all."
//
// Re-probed gap distribution across 19 queries (real, paraphrase, gibberish):
//
//	canonical match  Hebbian learning             5.66%
//	good matches     memory consolidation, REM    3.0–4.1%
//	mid matches      spreading-activation, ACT-R  1.6–2.8%
//	close-but-real   synaptic plasticity, weather 0.5–1.5%
//	tied / nonsense  cake-is-a-lie, plasticity    0–0.5%
//
// New thresholds put each of these in a label that means something:
// - 5%   strong: top-1 clearly ahead — commit to it
// - 1.5% moderate: top-1 ahead but candidates exist — top-2/3 are real options
// - 0.5% weak: top-1 barely ahead — treat top-N as candidates
// - <0.5% no_match: top results essentially tied — no clear winner.
const (
	confidenceStrongThreshold   = 0.05  // gap >= 5%   → strong
	confidenceModerateThreshold = 0.015 // gap >= 1.5% → moderate
	confidenceWeakThreshold     = 0.005 // gap >= 0.5% → weak; below → no_match
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
	case gap >= confidenceWeakThreshold:
		return ConfidenceWeak
	default:
		return ConfidenceNoMatch
	}
}

// AskHits is the search-only equivalent of Ask — runs the retriever
// and computes top-hit confidence, but skips context-pack assembly,
// activation re-scoring, and the RecordNoteAccess fan-out that Ask
// fires on (target + N neighbors). Used by callers that want the menu
// without committing to the top hit, e.g. `vaultmind ask --read 2`
// reads hit #2's body — so packing context around hit #1 would
// mis-attribute access events to a note the agent didn't read. The
// caller is responsible for firing access on whatever it chooses to
// read.
func AskHits(ctx context.Context, retriever retrieval.Retriever, query string, searchLimit int) (*AskResult, error) {
	hits, _, err := retriever.Search(ctx, query, searchLimit, 0, index.SearchFilters{})
	if err != nil {
		return nil, err
	}
	return &AskResult{
		Query:            query,
		TopHits:          hits,
		TopHitConfidence: computeTopHitConfidence(hits),
	}, nil
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

	// Plasticity roadmap step 5 reinforcement signal — record every note
	// that genuinely entered the agent's working context as a result of
	// this Ask. Track A.2 of the 2026-04-29 zoom-out widened this from
	// just the context-pack target to also cover every neighbor included
	// in the pack. Rationale: the neighbor frontmatter (and possibly
	// body) is now in the agent's context window — that's a real
	// retrieval-access event by ACT-R lights, not just framing for the
	// target. Uniform +1 per accessed note; weighting can come later
	// once the ranking layer actually consumes the counts (slice 5b').
	// Best-effort: each per-note tracking miss is logged at debug and
	// never fails the user query.
	if packResult != nil {
		if packResult.TargetID != "" {
			// Target of an Ask is high-intent — agent named the topic
			// and got back its body. CallerAgent.
			if recErr := index.RecordNoteAccessAs(db, packResult.TargetID, index.CallerAgent); recErr != nil {
				log.Debug().Err(recErr).Str("note_id", packResult.TargetID).Msg("recording note access failed (non-fatal)")
			}
		}
		for _, item := range packResult.Context {
			if item.ID == "" {
				continue
			}
			// Context-pack neighbors are medium-intent — they entered
			// the agent's working context as a result of the target
			// query, not by direct naming. CallerAgentNeighbor lets
			// `self` (or future ranking) treat them differently from
			// direct reads if it wants to.
			if recErr := index.RecordNoteAccessAs(db, item.ID, index.CallerAgentNeighbor); recErr != nil {
				log.Debug().Err(recErr).Str("note_id", item.ID).Msg("recording context-pack neighbor access failed (non-fatal)")
			}
		}
	}

	return result, nil
}
