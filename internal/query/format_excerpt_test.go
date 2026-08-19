package query

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The header's job is to describe what the agent actually received. An excerpt
// is not a body, and reporting it as one recreates the exact defect this session
// spent its length chasing: a surface that says the content arrived when it did
// not.
//
// "3 items, 2 with bodies" when two of those are 60-token excerpts of
// 1,000-token notes is a lie of the same family as "0 items, 900/900 tokens".
func TestContextHeader_ReportsExcerptsSeparately(t *testing.T) {
	ctx := &memory.ContextPackResult{
		TargetID:     "arc-example",
		BudgetTokens: 900,
		UsedTokens:   400,
		Context: []memory.ContextItem{
			{ID: "a", BodyIncluded: true, Body: "whole body"},
			{ID: "b", BodyIncluded: true, BodyExcerpted: true, Body: "the principle"},
			{ID: "c"},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, writeContextHeader(&buf, ctx, countItemsWithBodies(ctx.Context, formatOpts{}), formatOpts{}))
	got := buf.String()

	assert.Contains(t, got, "1 with bodies", "only the whole body counts as a body")
	assert.Contains(t, got, "1 excerpted", "an excerpt is a distinct, weaker delivery and must say so")
}

// When nothing was excerpted the line must stay exactly as it reads today —
// this change may not add noise to the common case.
func TestContextHeader_NoExcerptNoticeWhenNoneExcerpted(t *testing.T) {
	ctx := &memory.ContextPackResult{
		TargetID:     "arc-example",
		BudgetTokens: 900,
		UsedTokens:   400,
		Context: []memory.ContextItem{
			{ID: "a", BodyIncluded: true, Body: "whole"},
			{ID: "b"},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, writeContextHeader(&buf, ctx, countItemsWithBodies(ctx.Context, formatOpts{}), formatOpts{}))

	assert.NotContains(t, buf.String(), "excerpt")
}

// The relevance hint hardcoded "body suppressed" for every weak hit. Once a
// tight vault's weak hit started DELIVERING, that line became false — the output
// told the agent to run --read for a body already printed below it.
//
// BodyDecision exists precisely so the formatter and the telemetry read one
// value instead of each deriving its own. The hint was a third derivation, and
// it drifted the moment the rule changed. It must consult the decision.
func TestAskHeader_DoesNotClaimSuppressionWhenDelivering(t *testing.T) {
	result := &AskResult{
		Query:             "q",
		TopHitConfidence:  ConfidenceWeak,
		LowContrastVault:  true,
		NoiseFloorApplied: true,
		VaultNoteCount:    500,
		RelevanceZ:        0.83,
	}
	require.True(t, mustDeliver(t, result), "precondition: a tight vault's weak hit delivers")

	var buf bytes.Buffer
	require.NoError(t, writeAskHeader(&buf, result, false, false))

	assert.NotContains(t, buf.String(), "body suppressed",
		"the body is delivered; saying otherwise is the same defect class as reporting 0 items for a full pack")
	assert.Contains(t, buf.String(), "weak", "the weak label itself is still honest and still useful")
}

// A weak hit in a normal-contrast vault IS suppressed, so the hint stays.
func TestAskHeader_KeepsSuppressionNoticeWhenSuppressing(t *testing.T) {
	result := &AskResult{
		Query:             "q",
		TopHitConfidence:  ConfidenceWeak,
		LowContrastVault:  false,
		NoiseFloorApplied: true,
		VaultNoteCount:    500,
		RelevanceZ:        0.4,
	}

	var buf bytes.Buffer
	require.NoError(t, writeAskHeader(&buf, result, false, false))

	assert.Contains(t, buf.String(), "body suppressed")
}

// An excerpt is already bounded by ExcerptTokens — that is what makes it an
// excerpt. Running it through the 120-rune display truncation as well cuts an
// 80-token passage down to about a third and lands mid-word, which is the
// "compute the right thing, then don't show it" failure one more time.
func TestFormatAsk_ExcerptRendersWholeNotReTruncated(t *testing.T) {
	full := "When work is complex, multi-path, or speculative — meaning I have hypotheses about cost or architecture but not evidence — probe first. Do not commit to an estimate until cheap experiments have shifted the question."
	result := &AskResult{
		TopHitConfidence: ConfidenceStrong,
		Context: &memory.ContextPackResult{
			TargetID: "t", BudgetTokens: 900, UsedTokens: 100,
			Context: []memory.ContextItem{{
				ID: "x", BodyIncluded: true, BodyExcerpted: true, Body: full,
				Frontmatter: map[string]interface{}{"type": "reference", "title": "Probe Before Committing"},
			}},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, FormatAsk(result, &buf))

	assert.Contains(t, buf.String(), "shifted the question.",
		"the excerpt's own bound is the budget; re-truncating it for display discards what it was chosen to carry")
}

// The same re-truncation bug, one code path over. Fixing it for context items
// and not the target left the MOST prominent slot cutting its own excerpt at
// 120 runes — "...you hold the authority to probe i..." — while the smaller
// items below it rendered whole. Adjacent paths need the same fix, or the fix
// reads as done while the visible case stays broken.
func TestFormatAsk_TargetExcerptRendersWhole(t *testing.T) {
	full := "When you are the consumer of what you build, a request is a hypothesis, not an order — you hold the authority to probe it and to revert it when your own use of the tool says it is wrong."
	result := &AskResult{
		TopHitConfidence: ConfidenceStrong,
		Context: &memory.ContextPackResult{
			TargetID: "arc-decides", BudgetTokens: 900, UsedTokens: 100,
			Target: &memory.ContextPackTarget{
				ID:            "arc-decides",
				BodyExcerpted: true,
				Body:          full,
				Frontmatter:   map[string]interface{}{"type": "arc", "title": "The One Who Uses It Decides"},
			},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, FormatAsk(result, &buf))

	assert.Contains(t, buf.String(), "says it is wrong.",
		"the target's excerpt is already budget-bounded; truncating it again discards the half that carries the rule")
}

// "863/6000 tokens" with five sixths of the budget unspent reads as "the vault
// had nothing more to give". It actually means "a cap bound every item and 5,137
// tokens went unused" — a completely different fact, and the one that tells a
// reader the cap is the knob, not the budget.
//
// Same defect shape as "0 items, 900/900 tokens": a true number that describes
// the pack rather than what happened.
func TestContextHeader_SaysWhenACapBoundThePackNotTheBudget(t *testing.T) {
	ctx := &memory.ContextPackResult{
		TargetID:     "identity-who-i-am",
		BudgetTokens: 6000,
		UsedTokens:   863,
		Context: []memory.ContextItem{
			{ID: "a", BodyIncluded: true, BodyExcerpted: true, Body: "one"},
			{ID: "b", BodyIncluded: true, BodyExcerpted: true, Body: "two"},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, writeContextHeader(&buf, ctx, countItemsWithBodies(ctx.Context, formatOpts{}), formatOpts{}))

	assert.Contains(t, buf.String(), "unspent",
		"a pack bound by its cap with most of the budget left must say so; got %q", buf.String())
}

// A pack that genuinely used its budget must NOT claim spare room — that would
// be the mirror-image lie.
func TestContextHeader_NoUnspentNoticeWhenBudgetIsSpent(t *testing.T) {
	ctx := &memory.ContextPackResult{
		TargetID:     "t",
		BudgetTokens: 900,
		UsedTokens:   879,
		Context: []memory.ContextItem{
			{ID: "a", BodyIncluded: true, BodyExcerpted: true, Body: "one"},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, writeContextHeader(&buf, ctx, countItemsWithBodies(ctx.Context, formatOpts{}), formatOpts{}))

	assert.NotContains(t, buf.String(), "unspent")
}

func mustDeliver(t *testing.T, r *AskResult) bool {
	t.Helper()
	delivered, _ := r.BodyDecision(false)
	return delivered
}

// An excerpted item must render its text — otherwise the pack pays for the
// excerpt and the agent still sees only a title, which is the all-or-nothing
// failure wearing a new name.
func TestFormatAsk_RendersExcerptedBodies(t *testing.T) {
	result := &AskResult{
		TopHitConfidence: ConfidenceStrong,
		Context: &memory.ContextPackResult{
			TargetID:     "arc-example",
			BudgetTokens: 900,
			UsedTokens:   100,
			Context: []memory.ContextItem{
				{
					ID:            "arc-decides",
					BodyIncluded:  true,
					BodyExcerpted: true,
					Body:          "a request is a hypothesis, not an order",
					Frontmatter:   map[string]interface{}{"type": "arc", "title": "The One Who Uses It Decides"},
				},
			},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, FormatAsk(result, &buf))

	assert.True(t, strings.Contains(buf.String(), "a request is a hypothesis"),
		"an excerpt that is packed but not printed delivers nothing; got:\n%s", buf.String())
}
