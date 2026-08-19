package query

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// formatAskWithOptions OVERWRITES opts.pointersOnly when confidence auto-degrades:
//
//	delivered, _ := result.BodyDecision(opts.pointersOnly)
//	if !delivered { opts.pointersOnly = true }
//
// After that line the field means "no bodies will render", not "the caller asked
// for ids". writeAskHeader was already given the answer separately for exactly
// this reason. The FOOTER was not, so it named a flag the caller never passed and
// offered a remedy that does not exist:
//
//	$ vaultmind ask "kanye west discography" --vault vaultmind-identity --budget 900
//	(pointers only: 900 tokens assembled and withheld by --pointers-only — drop the flag …)
//
// There is no flag to drop. That is the same defect this whole change exists to
// close — a surface asserting a cause that is not the cause — reintroduced by the
// wording that replaced it.
func TestContextFooter_NamesTheRealCauseOfWithholding(t *testing.T) {
	body := strings.Repeat("assembled text the caller never received. ", 40)
	weakResult := func() *AskResult {
		return &AskResult{
			Query:             "kanye west discography",
			TopHitConfidence:  ConfidenceNoMatch,
			NoiseFloorApplied: true,
			VaultNoteCount:    500,
			RelevanceZ:        -2.76,
			TopHits:           []retrieval.ScoredResult{{ID: "a", Title: "A", Score: 0.1}},
			Context: &memory.ContextPackResult{
				TargetID: "a", BudgetTokens: 900, UsedTokens: 900,
				Target: &memory.ContextPackTarget{
					Frontmatter: map[string]any{"type": "reference", "title": "A"},
					Body:        body,
				},
			},
		}
	}

	t.Run("auto-degraded: the caller passed no flag", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, formatAskWithOptions(weakResult(), &buf, formatOpts{}))
		out := buf.String()

		require.Contains(t, out, "0 delivered", "precondition: this path withholds")
		assert.NotContains(t, out, "drop the flag",
			"no flag was passed; there is nothing to drop")
		assert.NotContains(t, out, "by --pointers-only",
			"blaming a flag the caller never used is a false cause")
		assert.Contains(t, out, "confidence",
			"say what actually withheld it — the top hit scored below the delivery threshold")
		assert.Contains(t, out, "--read",
			"and the override that exists on this path")
	})

	t.Run("caller asked: naming the flag is correct", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, formatAskWithOptions(weakResult(), &buf, formatOpts{pointersOnly: true}))
		out := buf.String()

		assert.Contains(t, out, "--pointers-only",
			"here the flag IS the cause and dropping it IS the remedy")
		assert.Contains(t, out, "drop the flag")
	})

	// A hit strong enough to deliver, withheld only because the caller asked:
	// the flag is unambiguously the cause, with no confidence story involved.
	t.Run("strong hit under the flag blames only the flag", func(t *testing.T) {
		r := weakResult()
		r.TopHitConfidence = ConfidenceStrong
		r.NoiseFloorApplied = false
		r.RelevanceZ = 3.0

		var buf bytes.Buffer
		require.NoError(t, formatAskWithOptions(r, &buf, formatOpts{pointersOnly: true}))
		out := buf.String()

		assert.Contains(t, out, "drop the flag")
		assert.NotContains(t, out, "confidence is below",
			"confidence was fine; only the flag withheld this")
	})
}
