package noisefloor

import "math"

// Calibration gating + the σ scale used by the z-relevance bands.
//
// z = (top_cosine − N) / σ expresses relevance in units of the vault's own
// embedding-space spread (the note-to-note cosine stddev). σ must stay finite
// and in a sane range, and a σ/N pair measured from too small a vault is too
// noisy to trust — these constants enforce both.
const (
	// SigmaFloor / SigmaCeil bound σ before it divides into z. Below the floor
	// (near-duplicate or tiny vault) z would explode and the no_match band would
	// collapse to a razor-thin cosine sliver. The ceiling only catches a
	// pathological/corrupt σ: clamping a legitimately-diverse vault's σ DOWN
	// inflates z toward over-confident labels (a false "strong" misleads more
	// than a false "weak"), so the ceiling sits well above the measured BGE-M3 σ
	// (~0.072) — typical vaults use their real σ; only a clearly-broken
	// measurement is capped. PROVISIONAL — the validation harness will set both
	// bounds from σ observed across vaults of genuinely different tightness.
	SigmaFloor = 0.03
	SigmaCeil  = 0.12

	// MinCalibNotes / MinCalibPairs gate whether a measured snapshot is trusted
	// for the live label. noteToNoteStats yields (0,0,0) for <2 notes, and a
	// floor/σ estimated from a handful of notes is noise. Below either threshold
	// the ask path falls back to the embedder defaults rather than a poisoned
	// per-vault measurement.
	MinCalibNotes = 30
	MinCalibPairs = 100
)

// embedderDispersion maps an embedding dimensionality to its shipped cold-start
// note-to-note σ, used before a vault measures its own. Keyed by dims (the
// storage-layer model discriminator), matching DefaultNoiseFloor.
//
//   - 1024 (BGE-M3): 0.073, empirically stable across a tight 40-note vault and a
//     loose 407-note vault (2026-05-31 probe).
//   - 384 (MiniLM): 0.0 — unmeasured; ClampSigma lifts it to SigmaFloor so a
//     cold-start MiniLM query never divides z by zero.
var embedderDispersion = map[int]float64{
	1024: 0.073,
	384:  0.0,
}

// DefaultDispersion returns the shipped note-to-note σ for an embedder of the
// given dimensionality, or 0.0 for unknown dims. The 0.0 is intentionally
// out-of-range: ClampSigma lifts it to SigmaFloor, so an unknown embedder gets a
// safe finite σ rather than a divide-by-zero.
func DefaultDispersion(dims int) float64 {
	return embedderDispersion[dims]
}

// ClampSigma constrains σ to [SigmaFloor, SigmaCeil]. A NaN or non-positive σ
// (cold-start, tiny vault, or a corrupt measurement) maps to SigmaFloor — never
// passed through, because a NaN σ would make z NaN and NaN fails every band
// comparison, silently falling through to the strongest label.
func ClampSigma(sigma float64) float64 {
	if math.IsNaN(sigma) || sigma < SigmaFloor {
		return SigmaFloor
	}
	if sigma > SigmaCeil {
		return SigmaCeil
	}
	return sigma
}
