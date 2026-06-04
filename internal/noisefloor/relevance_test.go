package noisefloor_test

import (
	"math"
	"testing"

	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/stretchr/testify/assert"
)

// Measured per-vault calibration from the 2026-05-31 probe (BGE-M3). The tight
// identity vault sits high and narrow (its measured N approaches its own
// real-query cosines); the looser research vault spreads out.
const (
	identN, identSig = 0.5059, 0.0733
	resN, resSig     = 0.4782, 0.0716
	bgeDefaultN      = 0.45 // DefaultNoiseFloor(1024) — the floor clamp ceiling
)

// probeRow is one real `vaultmind ask` measurement: the top hit's raw cosine and
// whether the query was a genuine match or off-topic garbage.
type probeRow struct {
	cosine    float64
	real      bool
	wantLabel string
	query     string
}

// The full 36-query probe, with the label the z-formula must produce. This is
// the golden fixture: it pins the scheme's behavior on real data across a tight
// and a loose vault. The load-bearing assertion is that NO real query is ever
// no_match (the regression the floor clamp fixes); the per-row labels pin the
// provisional bands so a future cutoff change is a visible, deliberate diff.
var identityProbe = []probeRow{
	{0.539, true, noisefloor.ConfidenceWeak, "who am I"},
	{0.479, true, noisefloor.ConfidenceWeak, "what is my purpose"}, // below the garbage ceiling, still not silenced
	{0.565, true, noisefloor.ConfidenceWeak, "spreading activation arc"},
	{0.567, true, noisefloor.ConfidenceWeak, "how do I handle code review"},
	{0.538, true, noisefloor.ConfidenceWeak, "RRF is not cosine similarity"},
	{0.648, true, noisefloor.ConfidenceModerate, "the agent is the user"},
	{0.627, true, noisefloor.ConfidenceModerate, "probe before committing"},
	{0.564, true, noisefloor.ConfidenceWeak, "my relationship with my collaborator"},
	{0.495, true, noisefloor.ConfidenceWeak, "noise floor calibration"},
	{0.538, true, noisefloor.ConfidenceWeak, "spreading activation in memory"},
	{0.508, false, noisefloor.ConfidenceWeak, "flat-pack furniture (lexical 'pack' collision)"},
	{0.426, false, noisefloor.ConfidenceNoMatch, "sourdough hydration"},
	{0.462, false, noisefloor.ConfidenceWeak, "Stratocaster pickup wiring"},
	{0.408, false, noisefloor.ConfidenceNoMatch, "tax filing deadline"},
	{0.480, false, noisefloor.ConfidenceWeak, "how to change a car tire"},
	{0.325, false, noisefloor.ConfidenceNoMatch, "carbonara recipe"},
	{0.358, false, noisefloor.ConfidenceNoMatch, "football scores"},
	{0.458, false, noisefloor.ConfidenceWeak, "knitting a wool sweater"},
}

var researchProbe = []probeRow{
	{0.734, true, noisefloor.ConfidenceStrong, "spreading activation in IR"},
	{0.722, true, noisefloor.ConfidenceStrong, "episodic memory consolidation"},
	{0.661, true, noisefloor.ConfidenceStrong, "reciprocal rank fusion"},
	{0.641, true, noisefloor.ConfidenceModerate, "knowledge graph embeddings"},
	{0.694, true, noisefloor.ConfidenceStrong, "retrieval augmented generation"},
	{0.521, true, noisefloor.ConfidenceWeak, "BGE-M3 embeddings"},
	{0.524, true, noisefloor.ConfidenceWeak, "lost in the middle"},
	{0.771, true, noisefloor.ConfidenceStrong, "hippocampal memory replay"},
	{0.662, true, noisefloor.ConfidenceStrong, "ColBERT late interaction"},
	{0.647, true, noisefloor.ConfidenceModerate, "spaced repetition algorithms"},
	{0.499, false, noisefloor.ConfidenceWeak, "flat-pack furniture (lexical 'pack' collision)"},
	{0.309, false, noisefloor.ConfidenceNoMatch, "sourdough hydration"},
	{0.406, false, noisefloor.ConfidenceNoMatch, "Stratocaster pickup wiring"},
	{0.393, false, noisefloor.ConfidenceNoMatch, "tax filing deadline"},
	{0.302, false, noisefloor.ConfidenceNoMatch, "how to change a car tire"},
	{0.306, false, noisefloor.ConfidenceNoMatch, "carbonara recipe"},
	{0.349, false, noisefloor.ConfidenceNoMatch, "football scores"},
	{0.360, false, noisefloor.ConfidenceNoMatch, "knitting a wool sweater"},
}

func TestRelevance_ProbeFixture(t *testing.T) {
	vaults := []struct {
		name     string
		n, sigma float64
		rows     []probeRow
	}{
		{"identity(tight)", identN, identSig, identityProbe},
		{"research(loose)", resN, resSig, researchProbe},
	}
	for _, v := range vaults {
		t.Run(v.name, func(t *testing.T) {
			for _, r := range v.rows {
				_, label := noisefloor.Relevance(r.cosine, v.n, v.sigma, bgeDefaultN)
				assert.Equalf(t, r.wantLabel, label, "%q (cosine %.3f)", r.query, r.cosine)
				if r.real {
					assert.NotEqualf(t, noisefloor.ConfidenceNoMatch, label,
						"a real query must never be silenced: %q (cosine %.3f)", r.query, r.cosine)
				}
			}
		})
	}
}

// The floor clamp is the structural fix for the tight-vault regression: a
// per-vault measured N must never raise the floor above the embedder ceiling.
// Identity's measured N=0.5059 with N−0.5σ=0.4693 would silence a real hit at
// cosine 0.46; clamped to the 0.45 ceiling, that hit survives as weak.
func TestRelevance_FloorClampedToEmbedderCeiling(t *testing.T) {
	// A hit between the embedder ceiling (0.45) and the unclamped floor (0.4693).
	z, label := noisefloor.Relevance(0.46, identN, identSig, bgeDefaultN)
	assert.Equal(t, noisefloor.ConfidenceWeak, label,
		"clamped floor (0.45) keeps a hit above the ceiling alive; unclamped (0.4693) would silence it")
	assert.Less(t, z, 0.0, "this hit is below measured N, so z is negative — weak, not no_match")

	// At/below the ceiling → no_match.
	_, label = noisefloor.Relevance(0.45, identN, identSig, bgeDefaultN)
	assert.Equal(t, noisefloor.ConfidenceNoMatch, label, "at the clamped floor is no_match")
}

// A degenerate σ (≈0) makes z non-finite. NaN fails every band comparison and
// would fall through to "strong" — labeling garbage as a confident match — so
// the guard must catch it and return no_match (with a serializable z).
func TestRelevance_NonFiniteZIsNoMatchNotStrong(t *testing.T) {
	cases := []struct {
		name                           string
		topCosine, n, sigma, embedderN float64
	}{
		{"sigma zero → +Inf z", 0.9, 0.45, 0.0, 0.45},
		{"sigma zero, below floor → -Inf z", 0.1, 0.45, 0.0, 0.45},
		{"NaN cosine", math.NaN(), 0.45, 0.07, 0.45},
		{"NaN sigma", 0.9, 0.45, math.NaN(), 0.45},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			z, label := noisefloor.Relevance(c.topCosine, c.n, c.sigma, c.embedderN)
			assert.Equal(t, noisefloor.ConfidenceNoMatch, label, "non-finite z must be no_match, never strong")
			assert.False(t, math.IsNaN(z) || math.IsInf(z, 0), "returned z must be finite (JSON-serializable)")
		})
	}
}

// Exact z-band boundaries. embedderN=0.0 holds the floor at 0 so it never
// interferes — these cases isolate the weak/moderate/strong cutoffs.
func TestRelevance_ZBandBoundaries(t *testing.T) {
	const n, sigma = 0.5, 0.1 // z = (c-0.5)/0.1
	cases := []struct {
		cosine    float64
		wantLabel string
		name      string
	}{
		{0.5 + noisefloor.WeakMaxZ*sigma, noisefloor.ConfidenceWeak, "z=WeakMaxZ is inclusive weak"},
		{0.5 + noisefloor.WeakMaxZ*sigma + 0.001, noisefloor.ConfidenceModerate, "just past WeakMaxZ → moderate"},
		{0.5 + noisefloor.ModerateMaxZ*sigma, noisefloor.ConfidenceModerate, "z=ModerateMaxZ is inclusive moderate"},
		{0.5 + noisefloor.ModerateMaxZ*sigma + 0.001, noisefloor.ConfidenceStrong, "just past ModerateMaxZ → strong"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, label := noisefloor.Relevance(c.cosine, n, sigma, 0.0)
			assert.Equal(t, c.wantLabel, label)
		})
	}
}

func TestDefaultNoiseFloor_PerEmbedderDims(t *testing.T) {
	// Cold-start: a brand-new vault with no measured calibration uses the
	// shipped per-embedder default so day-one queries calibrate immediately.
	assert.InDelta(t, 0.45, noisefloor.DefaultNoiseFloor(1024), 1e-9,
		"BGE-M3 (1024 dims) ships the probe-measured ~0.45")
	assert.InDelta(t, 0.0, noisefloor.DefaultNoiseFloor(384), 1e-9,
		"MiniLM (384 dims) ships conservative at 0.0 until measured")
	assert.InDelta(t, 0.0, noisefloor.DefaultNoiseFloor(999), 1e-9,
		"unknown dims fall back to 0.0 (no false no_match)")
}
