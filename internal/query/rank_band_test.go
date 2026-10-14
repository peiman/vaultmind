package query

import (
	"testing"

	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
)

func TestBandFor_NoiseFloorLabelsEachHit(t *testing.T) {
	// N 0.45, σ 0.1, embedder floor 0.45. The no-match cosine floor is 0.40.
	// z = (cosine − 0.45) / 0.1: 0.90 strong, 0.60 moderate, 0.50 weak.
	// The values sit inside the bands, clear of the edges, so float error
	// cannot move one across a cutoff.
	r := &AskResult{
		NoiseFloorApplied: true,
		NoiseFloor:        0.45,
		NoiseFloorSigma:   0.1,
		VaultNoteCount:    100,
		TopHitConfidence:  noisefloor.ConfidenceStrong,
		TopHits: []retrieval.ScoredResult{
			{ID: "strong-note"},
			{ID: "moderate-note"},
			{ID: "weak-note"},
			{ID: "noise-note"},
			{ID: "unscored-note"},
		},
		Similarities: map[string]float64{
			"strong-note":   0.90,
			"moderate-note": 0.60,
			"weak-note":     0.50,
			"noise-note":    0.30,
		},
	}
	const floor = 0.45
	assert.Equal(t, noisefloor.ConfidenceStrong, BandFor(r, "strong-note", floor))
	assert.Equal(t, RelevanceBand(r), BandFor(r, "strong-note", floor))
	assert.Equal(t, noisefloor.ConfidenceModerate, BandFor(r, "moderate-note", floor))
	assert.Equal(t, noisefloor.ConfidenceWeak, BandFor(r, "weak-note", floor))
	assert.Equal(t, noisefloor.ConfidenceNoMatch, BandFor(r, "noise-note", floor))
	assert.Empty(t, BandFor(r, "unscored-note", floor))
}

func TestBandFor_KeywordLabelsOnlyTheTopHit(t *testing.T) {
	r := &AskResult{
		TopHitConfidence: noisefloor.ConfidenceModerate,
		TopHits: []retrieval.ScoredResult{
			{ID: "first"},
			{ID: "second"},
		},
	}
	assert.Equal(t, noisefloor.ConfidenceModerate, BandFor(r, "first", 0))
	assert.Equal(t, RelevanceBand(r), BandFor(r, "first", 0))
	assert.Empty(t, BandFor(r, "second", 0))
}

func TestBandFor_WithheldJudgementIsEmpty(t *testing.T) {
	unmeasured := &AskResult{
		NoiseFloorApplied: true,
		floorUnmeasured:   true,
		VaultNoteCount:    6,
		TopHitConfidence:  noisefloor.ConfidenceStrong,
		NoiseFloor:        0,
		NoiseFloorSigma:   0.1,
		TopHits:           []retrieval.ScoredResult{{ID: "a"}},
		Similarities:      map[string]float64{"a": 0.9},
	}
	assert.Empty(t, RelevanceBand(unmeasured))
	assert.Empty(t, BandFor(unmeasured, "a", 0))

	weakSmall := &AskResult{
		NoiseFloorApplied: true,
		VaultNoteCount:    6,
		TopHitConfidence:  noisefloor.ConfidenceWeak,
		NoiseFloor:        0.45,
		NoiseFloorSigma:   0.1,
		TopHits:           []retrieval.ScoredResult{{ID: "a"}, {ID: "b"}},
		Similarities:      map[string]float64{"a": 0.55, "b": 0.90},
	}
	assert.Empty(t, BandFor(weakSmall, "a", 0.45))
	assert.Empty(t, BandFor(weakSmall, "b", 0.45))
	assert.Empty(t, BandFor(nil, "a", 0))
	assert.Empty(t, BandFor(unmeasured, "", 0))
}
