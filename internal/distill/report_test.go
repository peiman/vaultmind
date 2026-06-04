package distill_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/distill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatReport_ProposeOnlyAndGrouped(t *testing.T) {
	r := distill.Report{
		EpisodesScanned: 11,
		EpisodesKept:    9,
		Candidates: []distill.Candidate{
			{Rule: distill.RuleManifestoLens, EpisodeID: "ep-b", TurnIndex: 5, Trigger: "manifesto lens", Verbatim: "do it, manifesto lens on"},
			{Rule: distill.RuleAuthorityGrant, EpisodeID: "ep-a", TurnIndex: 9, Trigger: "full autonomy", Verbatim: "you have full autonomy"},
			{Rule: distill.RuleAuthorityGrant, EpisodeID: "ep-a", TurnIndex: 2, Trigger: "you decide", Verbatim: "you decide"},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, distill.FormatReport(r, &buf))
	out := buf.String()

	// The propose-only contract is stated up front and at the close.
	assert.Contains(t, out, "propose-only")
	assert.Contains(t, out, "MOMENTS, not arcs")
	assert.Contains(t, out, "Never auto-write identity")
	assert.Contains(t, out, "Scanned 11 episodes (9 after signal filter) → 3 candidate moments")

	// Episodes are grouped in id order (ep-a before ep-b) and candidates in turn
	// order (turn 2 before turn 9 within ep-a).
	assert.Less(t, strings.Index(out, "## ep-a"), strings.Index(out, "## ep-b"))
	assert.Less(t, strings.Index(out, "turn 2"), strings.Index(out, "turn 9"))
	assert.Contains(t, out, "full autonomy")
}

func TestFormatReport_Empty(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, distill.FormatReport(distill.Report{EpisodesScanned: 2, EpisodesKept: 0}, &buf))
	assert.Contains(t, buf.String(), "No candidate moments found")
}
