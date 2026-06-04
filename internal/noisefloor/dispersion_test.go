package noisefloor_test

import (
	"math"
	"testing"

	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/stretchr/testify/assert"
)

// DefaultDispersion ships a per-embedder note-to-note σ for cold start, before a
// vault measures its own. BGE-M3 σ was empirically stable (~0.072 across a tight
// 40-note vault and a loose 407-note vault, 2026-05-31). MiniLM is unmeasured →
// 0.0, which ClampSigma lifts to SigmaFloor so z never divides by zero.
func TestDefaultDispersion_PerEmbedderDims(t *testing.T) {
	assert.InDelta(t, 0.073, noisefloor.DefaultDispersion(1024), 1e-9,
		"BGE-M3 (1024) ships the probe-measured σ")
	assert.InDelta(t, 0.0, noisefloor.DefaultDispersion(384), 1e-9,
		"MiniLM (384) unmeasured → 0.0 (clamp lifts it)")
	assert.InDelta(t, 0.0, noisefloor.DefaultDispersion(999), 1e-9,
		"unknown dims → 0.0")
}

// ClampSigma keeps σ inside [SigmaFloor, SigmaCeil] so z stays finite and
// meaningful: a tiny/near-duplicate vault (σ→0) would explode z and razor-thin
// the floor; a sprawling vault (σ large) would flatten every hit to weak.
func TestClampSigma(t *testing.T) {
	cases := []struct {
		in, want float64
		name     string
	}{
		{0.073, 0.073, "in-range passes through"},
		{noisefloor.SigmaFloor, noisefloor.SigmaFloor, "floor boundary"},
		{noisefloor.SigmaCeil, noisefloor.SigmaCeil, "ceil boundary"},
		{0.0, noisefloor.SigmaFloor, "zero (cold-start MiniLM / tiny vault) → floor"},
		{0.001, noisefloor.SigmaFloor, "below floor → floor"},
		{0.5, noisefloor.SigmaCeil, "above ceil → ceil"},
		{-1.0, noisefloor.SigmaFloor, "negative (nonsense) → floor"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.InDelta(t, c.want, noisefloor.ClampSigma(c.in), 1e-9)
		})
	}
}

// NaN σ must not propagate (a NaN σ would make z NaN, and NaN fails every band
// comparison → would fall through to "strong"). Clamp it to the floor.
func TestClampSigma_NaN(t *testing.T) {
	assert.InDelta(t, noisefloor.SigmaFloor, noisefloor.ClampSigma(math.NaN()), 1e-9)
}

// The provisional-calibration gate: a snapshot measured from too few notes/pairs
// is a noisy estimate that would poison the live label. Constants exist so the
// ask path can refuse such a snapshot and fall back to defaults.
func TestProvisionalGateConstants(t *testing.T) {
	assert.Equal(t, 30, noisefloor.MinCalibNotes)
	assert.Equal(t, 100, noisefloor.MinCalibPairs)
	assert.Less(t, noisefloor.SigmaFloor, noisefloor.SigmaCeil, "floor must be below ceil")
}
