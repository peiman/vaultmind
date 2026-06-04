package noisefloor_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/stretchr/testify/assert"
)

// IsTightVault flags a vault whose notes are highly self-similar (high mean
// note-to-note cosine μ) — low retrieval contrast, where even a correct top hit
// reads "weak" because the embedder can't spread it far above the noise floor.
// The persona/identity vault (μ≈0.62) is tight; the research vault (μ≈0.51) is
// not. The threshold is PROVISIONAL (n=2).
func TestIsTightVault(t *testing.T) {
	assert.True(t, noisefloor.IsTightVault(0.624), "identity-class μ → tight")
	assert.False(t, noisefloor.IsTightVault(0.506), "research-class μ → not tight")
	assert.True(t, noisefloor.IsTightVault(noisefloor.TightVaultMu), "at the threshold counts as tight")
	assert.False(t, noisefloor.IsTightVault(0.0), "uncalibrated (μ=0) is never flagged tight")
}
