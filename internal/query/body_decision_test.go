package query

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The point of BodyDecision is that ONE rule answers both readers. If the
// formatter and the telemetry each decided for themselves, the log would
// eventually claim a body was delivered on a render that withheld it — and a
// delivery metric built on that log would measure the disagreement.
//
// This asserts them against each other: whatever the decision says, the
// rendered bytes must agree.
func TestBodyDecision_MatchesWhatIsActuallyRendered(t *testing.T) {
	body := "the note body that either appears or does not"
	cases := []struct {
		name         string
		confidence   string
		lowContrast  bool
		callerAsked  bool
		wantReason   string
		wantDelivery bool
	}{
		{"moderate hit delivers", ConfidenceModerate, false, false, "", true},
		{"weak hit is withheld", ConfidenceWeak, false, false, experiment.SuppressedBelowFloor, false},
		{"weak hit in a tight vault is withheld, and says which rule", ConfidenceWeak, true, false, experiment.SuppressedLowContrast, false},
		{"no match is withheld", ConfidenceNoMatch, false, false, experiment.SuppressedBelowFloor, false},
		{"caller asking for pointers beats a good hit", ConfidenceModerate, false, true, experiment.SuppressedByCaller, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := &AskResult{
				Query:             "q",
				TopHitConfidence:  tc.confidence,
				LowContrastVault:  tc.lowContrast,
				NoiseFloorApplied: true,
				VaultNoteCount:    500, // well above the calibration gate
				TopHits: []retrieval.ScoredResult{{
					ID: "note-a", Type: "reference", Title: "A Note", Path: "a.md", Score: 0.5,
				}},
				Context: &memory.ContextPackResult{
					TargetID:     "note-a",
					BudgetTokens: 900,
					UsedTokens:   900,
					Target:       &memory.ContextPackTarget{Frontmatter: map[string]any{"type": "reference", "title": "A Note"}, Body: body},
				},
			}

			delivered, reason := result.BodyDecision(tc.callerAsked)
			assert.Equal(t, tc.wantDelivery, delivered)
			assert.Equal(t, tc.wantReason, reason)

			var buf bytes.Buffer
			opts := formatOpts{pointersOnly: tc.callerAsked}
			require.NoError(t, formatAskWithOptions(result, &buf, opts))
			renderedBody := strings.Contains(buf.String(), body)

			assert.Equal(t, delivered, renderedBody,
				"the decision said delivered=%v; the rendered output %s the body",
				delivered, map[bool]string{true: "CONTAINS", false: "omits"}[renderedBody])
		})
	}
}
