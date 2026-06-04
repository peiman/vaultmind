// Package noisefloor turns a raw top-hit cosine similarity into an honest
// relevance signal, relative to the vault's noise floor N — the cosine an
// off-topic ("garbage") query gets to any note in the vault.
//
// Why this exists: the prior confidence label was derived from the gap between
// the top two RRF scores, which measures candidate separation, not relevance.
// A 2026-05-30 probe across two real vaults (BGE-M3) proved it mislabels —
// garbage queries got "moderate", real ones got "weak". Absolute cosine against
// a noise floor separated real from garbage; a 2026-05-31 probe then showed an
// ABSOLUTE band doesn't transfer across vault tightness (a tight persona vault
// compresses everything near a high floor), so the bands are normalized by the
// vault's own dispersion.
//
//	z = (top_cosine − N) / σ        — how many vault-σ the top hit clears N by
//	floor_cos = min(N − 0.5σ, EmbedderDefaultN) — the no_match cutoff (cosine)
//
// The FLOOR is the robust, probe-validated signal: a top cosine at/below it is
// indistinguishable from off-topic, and the recall hook reads no_match as
// "inject nothing" (silence beats noise). The min(…, EmbedderDefaultN) clamp
// lets a per-vault measurement only RELAX the floor, never raise it above the
// embedder ceiling — which is what keeps a tight vault's high measured N from
// silencing its own weak-but-real queries. The finer z-bands above the floor
// are PROVISIONAL (fit on n=2 vaults with near-equal σ) until the validation
// harness earns them on vaults of genuinely different tightness.
package noisefloor

import "math"

// Confidence tier strings. Kept identical to internal/query's tiers so the
// agent-facing label vocabulary is unchanged; only the derivation is honest now.
const (
	ConfidenceStrong   = "strong"
	ConfidenceModerate = "moderate"
	ConfidenceWeak     = "weak"
	// ConfidenceNoMatch — the top hit is at or below the noise floor, i.e.
	// indistinguishable from what an off-topic query scores. Callers treat
	// this as "nothing relevant" and inject silence.
	ConfidenceNoMatch = "no_match"
)

// Relevance-band parameters.
//
// PROVISIONAL (2026-05-31): the z-cutoffs are grounded in the probe (identity +
// research vaults, BGE-M3) but NOT yet validated on a large/diverse query set.
// Observed z: real queries 0.45–4.09, garbage −2.5–0.3 (the one overlap is a
// lexical "flat-pack"≈"context-pack" collision). The floor margin is the robust
// part; these inner cutoffs are what the validation harness must confirm.
const (
	// FloorSigmaMargin sets the no_match floor FloorSigmaMargin·σ below N, so a
	// genuinely weak match just under N degrades to "weak" rather than being
	// silenced (moving the floor from N to N−0.5σ recovered all 20 real probe
	// queries across both vaults).
	FloorSigmaMargin = 0.5

	// Above-floor z-bands on z = (top_cosine − N)/σ.
	WeakMaxZ     = 1.0 // (floor, 1.0] → weak
	ModerateMaxZ = 2.5 // (1.0, 2.5]   → moderate; above → strong
)

// Relevance turns a raw top-hit cosine into an honest relevance score z and a
// tier label, given the vault's noise floor N, its note-to-note dispersion σ
// (pre-clamped — see ClampSigma), and the embedder's shipped default floor.
//
// no_match is decided by a COSINE floor, not by z: floor_cos =
// min(N − FloorSigmaMargin·σ, embedderDefaultN). The min keeps a per-vault
// measurement from ever raising the floor above the probe-validated embedder
// ceiling. Above the floor, z = (top_cosine − N)/σ is banded at WeakMaxZ /
// ModerateMaxZ.
//
// The NaN/Inf guard runs first: a degenerate σ (≈0) would make z non-finite,
// and a NaN z fails every band comparison below and would fall through to
// "strong" — labeling garbage as a confident match. ClampSigma upstream should
// prevent this; the guard is defence in depth.
func Relevance(topCosine, noiseFloor, sigmaEff, embedderDefaultN float64) (z float64, label string) {
	z = (topCosine - noiseFloor) / sigmaEff
	if math.IsNaN(z) || math.IsInf(z, 0) {
		// Relevance is uncomputable from these inputs. Report no_match with a
		// serializable z=0 — returning NaN/Inf would break --json encoding, and
		// (worse) NaN silently fails the band switch below.
		return 0, ConfidenceNoMatch
	}
	floorCos := math.Min(noiseFloor-FloorSigmaMargin*sigmaEff, embedderDefaultN)
	if topCosine <= floorCos {
		return z, ConfidenceNoMatch
	}
	switch {
	case z <= WeakMaxZ:
		return z, ConfidenceWeak
	case z <= ModerateMaxZ:
		return z, ConfidenceModerate
	default:
		return z, ConfidenceStrong
	}
}

// embedderNoiseFloor maps an embedding dimensionality to its shipped default
// noise floor, used at cold start before a vault has measured its own. Keyed
// by dims because that's what distinguishes the models at the storage layer
// (BGE-M3 = 1024, MiniLM = 384).
//
//   - 1024 (BGE-M3): 0.45, the probe-measured garbage ceiling.
//   - 384  (MiniLM): 0.0 conservative — its floor hasn't been probe-measured,
//     and 0.0 never produces a false no_match (it just can't flag garbage yet).
var embedderNoiseFloor = map[int]float64{
	1024: 0.45,
	384:  0.0,
}

// DefaultNoiseFloor returns the shipped noise floor for an embedder of the
// given dimensionality, or 0.0 for unknown dims (the safe choice: no false
// "nothing relevant").
func DefaultNoiseFloor(dims int) float64 {
	if n, ok := embedderNoiseFloor[dims]; ok {
		return n
	}
	return 0.0
}
