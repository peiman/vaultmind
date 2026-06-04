package query

import (
	"context"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/noisefloor"
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
	// NoiseFloor is the vault's noise-floor N (the cosine an off-topic query gets
	// to any note). When HasNoiseFloor is true and cosine similarities are
	// available, the top-hit confidence is derived honestly from N and the
	// dispersion below (see internal/noisefloor) instead of the legacy RRF gap.
	NoiseFloor float64
	// NoiseFloorSigma is the vault's note-to-note cosine dispersion σ — the scale
	// for z = (top_cosine − N)/σ. The caller passes a measured, clamped σ; when
	// zero/unset, Ask falls back to the embedder's DefaultDispersion.
	NoiseFloorSigma float64
	HasNoiseFloor   bool
	// VaultLowContrast marks a tight vault (high note-to-note μ) where even
	// correct top hits read "weak". Surfaced on the result so the formatter can
	// explain a weak label rather than let an agent misread it as "irrelevant".
	VaultLowContrast bool
	// SuppressOnNoMatch is the recall floor: when the top hit lands at/below the
	// noise floor (no_match), Ask returns before the context pack and the access
	// fan-out — so an ambient recall on an off-domain prompt neither injects
	// noise nor reinforces the irrelevant note it happened to surface. Opt-in
	// (the recall hook sets it); interactive ask leaves it false.
	SuppressOnNoMatch bool
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
// TopHitConfidence is one of "strong" | "moderate" | "weak" | "no_match" | "".
// Its PRIMARY derivation is the band-normalized relevance RelevanceZ =
// (TopHitCosine − NoiseFloor)/NoiseFloorSigma (see internal/noisefloor and
// NoiseFloorApplied below): z measures how many vault-σ the top hit clears the
// off-topic noise floor by — which is the question the agent is actually asking,
// expressed on a scale that transfers across vault tightness. When no noise
// floor is available (HasNoiseFloor false), it FALLS BACK to the RRF-gap
// heuristic (computeTopHitConfidenceRRFGap), which measures candidate
// separation, not relevance, and so never yields "no_match". Read
// NoiseFloorApplied to tell the two apart.
type AskResult struct {
	Query            string                    `json:"query"`
	TopHits          []retrieval.ScoredResult  `json:"top_hits"`
	Context          *memory.ContextPackResult `json:"context,omitempty"`
	RetrievalMode    string                    `json:"retrieval_mode,omitempty"`
	TopHitConfidence string                    `json:"top_hit_confidence,omitempty"`
	Similarities     map[string]float64        `json:"-"` // raw cosine similarities (not serialized)
	// Noise-floor relevance (populated when HasNoiseFloor + similarities exist).
	// TopHitCosine is the raw cosine of the query against the top hit; NoiseFloor
	// is N; NoiseFloorSigma is σ; RelevanceZ = (TopHitCosine − N)/σ is the label's
	// derivation; RelevanceR = TopHitCosine − N is the raw cosine margin, kept for
	// agents that want the un-normalized number. All surfaced so an agent sees the
	// real values, not just the tier word.
	// NoiseFloorApplied is true when TopHitConfidence was derived from the
	// noise-floor relevance (not the RRF-gap fallback). The formatter uses it to
	// print honest "nothing relevant" / "z above noise floor" labels, and JSON
	// consumers use it to know the floats below are meaningful (rather than
	// zero-because-unset). Because it disambiguates, the floats are NOT omitempty
	// — a MiniLM floor of 0.0 or an exact-boundary z of 0.0 must still serialize
	// rather than silently vanish.
	NoiseFloorApplied bool    `json:"noise_floor_applied,omitempty"`
	TopHitCosine      float64 `json:"top_hit_cosine,omitempty"`
	NoiseFloor        float64 `json:"noise_floor"`
	NoiseFloorSigma   float64 `json:"noise_floor_sigma"`
	RelevanceR        float64 `json:"relevance_r"`
	RelevanceZ        float64 `json:"relevance_z"`
	// LowContrastVault is true when the vault is tight (high note-to-note μ), so
	// a "weak" top hit often means the best available correct match, not
	// "irrelevant". The formatter renders a one-line hint when it's set.
	LowContrastVault bool `json:"low_contrast_vault,omitempty"`
}

// Confidence tier strings. The noisefloor package is the single source of
// truth (Principle 7); these are aliases so the rest of the query package can
// keep referring to query.ConfidenceX without a second literal set that could
// silently drift from the labels Ask now writes via noisefloor.Relevance.
const (
	ConfidenceStrong   = noisefloor.ConfidenceStrong
	ConfidenceModerate = noisefloor.ConfidenceModerate
	ConfidenceWeak     = noisefloor.ConfidenceWeak
	// ConfidenceNoMatch — in noise-floor mode, the top hit is at/below the
	// embedder's noise floor (nothing relevant). In the RRF-gap fallback it
	// means the top results are essentially tied (no clear winner). Both read
	// as "don't commit to top-1".
	ConfidenceNoMatch = noisefloor.ConfidenceNoMatch
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
func computeTopHitConfidenceRRFGap(hits []retrieval.ScoredResult) string {
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
// fires on (target + N neighbors).
//
// KNOWN DIVERGENCE (follow-up): AskHits does not compute cosine similarities,
// so it cannot derive noise-floor confidence — it uses the RRF-gap fallback
// label. The same query therefore reads as "relevance: <tier> (R=…)" under
// `ask` but "top-hit confidence: <tier>" under `ask --read N`, with different
// semantics behind the same tier word. The header phrasing differs so the
// derivation is at least visible. Unifying them means computing the top hit's
// cosine here too (an extra embed+score per --read); deferred until that cost
// is justified. Used by callers that want the menu
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
		TopHitConfidence: computeTopHitConfidenceRRFGap(hits),
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
		TopHitConfidence: computeTopHitConfidenceRRFGap(hits),
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

	// Honest confidence: with a noise floor and the top hit's cosine in hand,
	// derive the label as z = (top_cosine − N)/σ (see internal/noisefloor),
	// overriding the RRF-gap fallback set when result was built. The RRF gap
	// measures candidate separation; the noise-floor relevance measures how far
	// the top hit clears the off-topic floor, which is the question the agent is
	// actually asking. A top cosine at/below the floor → no_match, which the
	// recall hook treats as "inject nothing".
	if cfg.HasNoiseFloor && result.Similarities != nil {
		if topCosine, ok := result.Similarities[hits[0].ID]; ok {
			var dims int
			if cfg.Embedder != nil {
				dims = cfg.Embedder.Dims()
			}
			// σ: the caller's measured value when set, else the embedder default.
			// Clamped so a degenerate σ can't divide z to a false "strong".
			sigma := cfg.NoiseFloorSigma
			if sigma <= 0 {
				sigma = noisefloor.DefaultDispersion(dims)
			}
			sigma = noisefloor.ClampSigma(sigma)
			embedderN := noisefloor.DefaultNoiseFloor(dims)
			z, label := noisefloor.Relevance(topCosine, cfg.NoiseFloor, sigma, embedderN)
			result.TopHitCosine = topCosine
			result.NoiseFloor = cfg.NoiseFloor
			result.NoiseFloorSigma = sigma
			result.RelevanceR = topCosine - cfg.NoiseFloor
			result.RelevanceZ = z
			result.TopHitConfidence = label
			result.NoiseFloorApplied = true
			result.LowContrastVault = cfg.VaultLowContrast
		} else {
			// Caller wanted honest noise-floor labeling but the top hit has no
			// cosine in the similarities map (keyword-only hit, or similarity
			// computation skipped above). We silently keep the RRF-gap fallback
			// label — leave a breadcrumb so this degradation is diagnosable
			// rather than invisible.
			log.Debug().Str("top_hit_id", hits[0].ID).
				Msg("noise floor requested but top hit has no cosine; keeping RRF-gap confidence")
		}
	}

	// Recall floor: nothing is relevant (top hit at/below the noise floor) and
	// the caller opted into suppression — stop here. Skipping the context pack
	// and its access fan-out means an ambient recall on an off-domain prompt
	// injects nothing AND doesn't reinforce the irrelevant note it surfaced.
	// Gated on NoiseFloorApplied so this fires only on the honest noise-floor
	// no_match, never the RRF-gap fallback's "results essentially tied" no_match
	// (that means "no clear winner", not "nothing relevant").
	if cfg.SuppressOnNoMatch && result.NoiseFloorApplied && result.TopHitConfidence == noisefloor.ConfidenceNoMatch {
		return result, nil
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
