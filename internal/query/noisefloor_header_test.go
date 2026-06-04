package query

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// When noise-floor relevance is applied, the header surfaces the band-normalized
// z and glosses it as "Nσ above/below the off-topic noise floor", reading
// "nothing relevant" on the floor — a distinct, more honest meaning than the RRF
// "tied" case. Pins the agent-facing contract the band-normalization establishes.
func TestFormatAsk_NoiseFloorHeader(t *testing.T) {
	hits := []retrieval.ScoredResult{{ID: "identity-who-i-am", Title: "Who I Am", Score: 0.02}}
	cases := []struct {
		name       string
		confidence string
		z          float64
		wantSubstr string
	}{
		{"floor says nothing relevant", ConfidenceNoMatch, -2.47, "nothing relevant"},
		{"floor shows z", ConfidenceNoMatch, -2.47, "z=-2.47"},
		{"weak above floor", ConfidenceWeak, 0.80, "relevance: weak (z=+0.80, 0.8σ above the off-topic noise floor)"},
		{"weak below floor reads 'below'", ConfidenceWeak, -0.37, "0.4σ below the off-topic noise floor"},
		{"moderate", ConfidenceModerate, 1.94, "relevance: moderate (z=+1.94, 1.9σ above the off-topic noise floor)"},
		{"strong", ConfidenceStrong, 3.58, "relevance: strong (z=+3.58, 3.6σ above the off-topic noise floor)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			result := &AskResult{
				Query:             "q",
				TopHits:           hits,
				TopHitConfidence:  c.confidence,
				RelevanceZ:        c.z,
				NoiseFloorApplied: true,
			}
			require.NoError(t, FormatAsk(result, &buf))
			header := strings.SplitN(buf.String(), "\n", 2)[0]
			assert.Contains(t, header, c.wantSubstr)
		})
	}
}

// --explain adds a relevance-math line reconstructing z = (cosine − N)/σ. It
// surfaces N and σ so a stale or cross-vault noise floor is visible on the CLI
// (e.g. an identity query showing the research vault's N).
func TestFormatAskExplain_RelevanceMath(t *testing.T) {
	var buf bytes.Buffer
	result := &AskResult{
		Query:             "q",
		TopHits:           []retrieval.ScoredResult{{ID: "x", Title: "X", Score: 0.02}},
		TopHitConfidence:  ConfidenceModerate, // moderate isn't auto-degraded to pointers-only
		TopHitCosine:      0.648,
		NoiseFloor:        0.506,
		NoiseFloorSigma:   0.073,
		RelevanceZ:        1.94,
		NoiseFloorApplied: true,
	}
	require.NoError(t, FormatAskExplain(result, &buf))
	out := buf.String()
	assert.Contains(t, out, "relevance math:")
	assert.Contains(t, out, "top_cosine=0.648")
	assert.Contains(t, out, "N=0.506")
	assert.Contains(t, out, "σ=0.073")
	assert.Contains(t, out, "z=")
}

// The low-contrast hint fires only on a weak top hit in a tight vault — so an
// agent doesn't misread a tight vault's persistent "weak" as "nothing relevant".
func TestFormatAsk_LowContrastHint(t *testing.T) {
	hits := []retrieval.ScoredResult{{ID: "identity-who-i-am", Title: "Who I Am", Score: 0.02}}
	cases := []struct {
		name        string
		confidence  string
		lowContrast bool
		wantHint    bool
	}{
		{"weak + tight → hint", ConfidenceWeak, true, true},
		{"weak + not tight → no hint", ConfidenceWeak, false, false},
		{"strong + tight → no hint (only weak triggers)", ConfidenceStrong, true, false},
		{"moderate + tight → no hint", ConfidenceModerate, true, false},
		{"no_match + tight → no hint (off-topic is off-topic, even on a tight vault)", ConfidenceNoMatch, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, FormatAsk(&AskResult{
				Query: "who am I", TopHits: hits, TopHitConfidence: c.confidence,
				RelevanceZ: 0.45, NoiseFloorApplied: true, LowContrastVault: c.lowContrast,
			}, &buf))
			if c.wantHint {
				assert.Contains(t, buf.String(), "tight vault")
			} else {
				assert.NotContains(t, buf.String(), "tight vault")
			}
		})
	}
}

// The plain (non-explain) header must NOT carry the relevance-math line.
func TestFormatAsk_NoRelevanceMathWithoutExplain(t *testing.T) {
	var buf bytes.Buffer
	result := &AskResult{
		Query:             "q",
		TopHits:           []retrieval.ScoredResult{{ID: "x", Title: "X", Score: 0.02}},
		TopHitConfidence:  ConfidenceModerate,
		RelevanceZ:        1.94,
		NoiseFloorApplied: true,
	}
	require.NoError(t, FormatAsk(result, &buf))
	assert.NotContains(t, buf.String(), "relevance math:")
}

// Without noise-floor mode (keyword-only fallback), the header keeps the
// legacy RRF-gap phrasing — guards against the two label vocabularies
// bleeding into each other.
func TestFormatAsk_RRFFallbackHeaderUnchanged(t *testing.T) {
	var buf bytes.Buffer
	result := &AskResult{
		Query:            "q",
		TopHits:          []retrieval.ScoredResult{{ID: "a", Title: "A", Score: 0.5}},
		TopHitConfidence: ConfidenceModerate,
		// NoiseFloorApplied stays false.
	}
	require.NoError(t, FormatAsk(result, &buf))
	header := strings.SplitN(buf.String(), "\n", 2)[0]
	assert.Contains(t, header, "top-hit confidence: moderate")
	assert.NotContains(t, header, "noise floor")
}
