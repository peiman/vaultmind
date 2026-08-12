package query

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A brand-new vault cannot clear the noise floor, and saying so is a lie.
//
// `vaultmind init` scaffolds six notes and prints `ask "who am I"` as the next
// step. On that vault the query returns identity-who-am-i at rank 1 — exactly
// right — and the header reads:
//
//	[relevance: weak (z=-0.11, 0.1σ below the off-topic noise floor) — body suppressed]
//
// Retrieval was correct; the LABEL was wrong. z is measured against a floor
// calibrated for a large corpus, and six notes cannot clear it. Worse, the
// existing mitigation (LowContrastVault) is set only from a calibration
// snapshot that requires MinCalibNotes notes and MinCalibPairs pairs — so the
// hint that would have explained a weak label is structurally unavailable to
// precisely the vault that most needs it. A user's first query looks like a
// failure of the tool they just installed.
//
// Below the calibration gate the honest posture is "no basis to judge yet", not
// "weak" — and a vault that small has no context budget to protect, so
// suppressing the body buys nothing and costs the user the answer.

func smallVaultResult(noteCount int, confidence string, z float64) *AskResult {
	return &AskResult{
		Query:             "who am I",
		TopHits:           []retrieval.ScoredResult{{ID: "identity-who-am-i", Title: "Who Am I", Score: 0.02}},
		TopHitConfidence:  confidence,
		RelevanceZ:        z,
		NoiseFloorApplied: true,
		VaultNoteCount:    noteCount,
	}
}

func TestFormatAsk_UncalibratedVaultDoesNotClaimWeak(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, FormatAsk(smallVaultResult(6, ConfidenceWeak, -0.11), &buf))
	header := strings.SplitN(buf.String(), "\n", 2)[0]

	assert.NotContains(t, header, "body suppressed",
		"a vault too small to calibrate has no context budget to protect; withholding the body buys nothing")
	assert.NotContains(t, header, "off-topic noise floor",
		"the floor is calibrated for a large corpus — quoting it against 6 notes states a measurement we don't have")
	assert.Contains(t, header, "6 notes",
		"say the actual size, so the user can see why judgement is withheld")
}

func TestFormatAsk_UncalibratedVaultAlsoCoversNoMatch(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, FormatAsk(smallVaultResult(6, ConfidenceNoMatch, -2.4), &buf))
	header := strings.SplitN(buf.String(), "\n", 2)[0]

	assert.NotContains(t, header, "nothing relevant",
		"'nothing relevant' is a claim about the corpus; with 6 notes we have no standing to make it")
	assert.Contains(t, header, "6 notes")
}

// The gate is the same constant that decides whether a measured calibration
// snapshot can be trusted — one threshold, not two that can drift apart.
func TestFormatAsk_AtCalibrationGateTheNormalLabelReturns(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, FormatAsk(smallVaultResult(noisefloor.MinCalibNotes, ConfidenceWeak, -0.11), &buf))
	header := strings.SplitN(buf.String(), "\n", 2)[0]

	assert.Contains(t, header, "body suppressed",
		"at/above the calibration gate the existing weak-label behaviour is unchanged")
}

// An unknown note count (0 — the field was never populated) must behave exactly
// as before, so every caller that doesn't set it keeps today's semantics.
func TestFormatAsk_UnknownNoteCountKeepsExistingBehaviour(t *testing.T) {
	var buf bytes.Buffer
	result := smallVaultResult(0, ConfidenceWeak, -0.11)
	require.NoError(t, FormatAsk(result, &buf))
	header := strings.SplitN(buf.String(), "\n", 2)[0]

	assert.Contains(t, header, "body suppressed",
		"note count 0 means 'not measured', which must not be read as 'tiny vault'")
	assert.Contains(t, header, "off-topic noise floor")
}

// A confident hit in a small vault is still reported normally — the gate
// changes only the labels that withhold an answer.
func TestFormatAsk_SmallVaultStrongHitUnaffected(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, FormatAsk(smallVaultResult(6, ConfidenceStrong, 3.1), &buf))
	header := strings.SplitN(buf.String(), "\n", 2)[0]

	assert.Contains(t, header, "relevance: strong")
}
