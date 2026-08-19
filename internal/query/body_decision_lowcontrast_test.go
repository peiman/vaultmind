package query

import (
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
)

// Low contrast is a property of the VAULT, not evidence about the hit.
//
// A tight vault — one whose notes are all about the same subject, which is what
// an identity vault IS by construction — cannot produce a top hit that rises far
// above its own noise floor. Every correct answer reads "weak". Suppressing
// bodies on that signal therefore withholds content from precisely the vaults
// VaultMind exists to serve, and does so more reliably the better curated the
// vault is.
//
// The formatter already knows this and says so in the output:
//
//	[tight vault: a weak top hit here is often the best available correct match,
//	 not 'nothing relevant' — use --read 1 for the body]
//
// The tool printed that sentence and then withheld the body anyway, telling the
// agent to go fetch by hand what it had already assembled. Measured on the live
// identity vault, all four reach-hook trigger queries land in this band
// (z = +0.17, +1.25, −0.32, +0.32), so this gate alone meant no reach injection
// could ever carry content.
func TestBodyDecision_LowContrastWeakDeliversBody(t *testing.T) {
	r := &AskResult{TopHitConfidence: ConfidenceWeak, LowContrastVault: true}

	delivered, reason := r.BodyDecision(false)

	assert.True(t, delivered, "a tight vault's weak hit is usually the best available correct match — deliver it with the caveat")
	assert.Empty(t, reason)
}

// The caller's own --pointers-only still wins. "The hook asked for ids" and "the
// tool judged the hit too weak" are different facts with opposite remedies, and
// conflating them is how the delivery question became unanswerable.
func TestBodyDecision_CallerPointersOnlyStillWins(t *testing.T) {
	r := &AskResult{TopHitConfidence: ConfidenceWeak, LowContrastVault: true}

	delivered, reason := r.BodyDecision(true)

	assert.False(t, delivered)
	assert.Equal(t, experiment.SuppressedByCaller, reason)
}

// A weak hit in a NORMAL-contrast vault is still withheld: there, "weak" really
// does mean the hit failed to separate from the floor.
func TestBodyDecision_WeakInNormalVaultStillSuppressed(t *testing.T) {
	r := &AskResult{TopHitConfidence: ConfidenceWeak, LowContrastVault: false}

	delivered, reason := r.BodyDecision(false)

	assert.False(t, delivered)
	assert.Equal(t, experiment.SuppressedBelowFloor, reason)
}

// Genuinely irrelevant results are unaffected — no_match is below the floor and
// stays suppressed in every vault, so this change cannot turn off-domain noise
// into injected bodies.
func TestBodyDecision_NoMatchStillSuppressedInTightVault(t *testing.T) {
	r := &AskResult{TopHitConfidence: ConfidenceNoMatch, LowContrastVault: true}

	delivered, reason := r.BodyDecision(false)

	assert.False(t, delivered)
	assert.Equal(t, experiment.SuppressedBelowFloor, reason)
}
