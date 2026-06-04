package noisefloor

// TightVaultMu is the note-to-note cosine mean (μ) above which a vault is treated
// as "tight" — its notes are so self-similar that the embedder can't spread a
// correct top hit far above the noise floor, so genuine matches read "weak". On
// a tight vault, a weak label often means "best available correct match", not
// "nothing relevant"; the ask formatter surfaces that as a one-line hint so an
// agent doesn't misread persistent weak labels as a broken vault.
//
// PROVISIONAL (n=2, 2026-05-31): the identity persona vault measured μ≈0.62
// (tight), the research vault μ≈0.51 (not). The threshold splits those two; a
// third vault should confirm or move it. Kept deliberately above 0.51 and below
// 0.62 with margin on both sides.
const TightVaultMu = 0.58

// IsTightVault reports whether a vault's measured note-to-note μ marks it as
// low-contrast. μ ≤ 0 (uncalibrated) is never tight.
func IsTightVault(mu float64) bool {
	return mu >= TightVaultMu
}
