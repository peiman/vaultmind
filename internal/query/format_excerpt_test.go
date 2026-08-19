package query

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/retrieval"
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
	require.NoError(t, writeContextHeader(&buf, ctx, formatOpts{}))
	got := buf.String()

	// The original form of this test asserted "1 with bodies" — and that phrasing
	// is what produced the live SessionStart line "8 items, 0 with bodies, 9
	// excerpted". Excluding excerpts from the body count made the two categories
	// disjoint, so a pack of pure excerpts reported ZERO delivered while nine
	// bodies rendered underneath it.
	//
	// The intent above is still right; the expression was wrong. An excerpt IS a
	// delivery — the agent received words it can act on — and it is ALSO not the
	// whole note. Both facts are stated, and neither stands in for the other.
	assert.Contains(t, got, "2 delivered",
		"the whole body and the excerpt both reached the agent")
	assert.Contains(t, got, "1 as excerpt",
		"an excerpt is a weaker delivery and must still say so")
	assert.NotContains(t, got, "0 delivered",
		"the defect this test now guards: text rendered, reported as nothing")
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
	require.NoError(t, writeContextHeader(&buf, ctx, formatOpts{}))

	assert.NotContains(t, buf.String(), "excerpt")
	assert.Contains(t, buf.String(), "1 delivered in full",
		"and it says so positively — 'in full' is the fact that no note get is needed")
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
	delivered := mustDeliver(t, result)
	require.True(t, delivered, "precondition: a tight vault's weak hit delivers")

	var buf bytes.Buffer
	// The 4th arg is the ANSWER — derived here rather than hardcoded, so this
	// test cannot drift away from what BodyDecision actually says.
	require.NoError(t, writeAskHeader(&buf, result, false, delivered))

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
	delivered := mustDeliver(t, result)
	require.False(t, delivered, "precondition: a normal-contrast weak hit is withheld")

	var buf bytes.Buffer
	require.NoError(t, writeAskHeader(&buf, result, false, delivered))

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
	require.NoError(t, writeContextHeader(&buf, ctx, formatOpts{}))
	got := buf.String()

	// SUPERSEDED, deliberately. This used to assert the header said "bound by
	// --excerpt, 5137 tokens unspent". That notice fired on `unspent >
	// budget/2` — a budget RATIO, which --max-items or a vault with few matches
	// produce just as easily. It described a guess about the cause.
	//
	// The denominator is gone instead, which removes the misreading at its
	// source: with no "863/6000" there is nothing to misread as "the vault had
	// nothing more to give". The budget is named by the footer, and only when a
	// note was actually dropped for it — the same information, none of the
	// guessing. See TestContextHeader_NamesTheBudgetOnlyWhenItBound.
	assert.NotContains(t, got, "6000",
		"every note fit; the budget is not what limited this pack")
	assert.Contains(t, got, "2 delivered as excerpts",
		"what the caller can act on is that these are excerpts, not that the budget was roomy")
}

// --read is the one path that ALWAYS renders a body — it exists to fetch one.
// FormatAskReadWithOptions passed callerAsked=false and let writeAskHeader
// re-derive suppression from the confidence label, so a weak hit printed
// "body suppressed; use --read N to override" directly above the body that
// --read had just delivered.
//
// `callerAsked bool` cannot express "a body is definitely coming" — it answers
// what the caller REQUESTED, not what is about to happen. The header needs the
// answer, so it takes one.
func TestFormatAskRead_NeverClaimsSuppressionAboveTheBodyItPrints(t *testing.T) {
	result := &AskResult{
		Query:             "who am i",
		TopHitConfidence:  ConfidenceWeak,
		NoiseFloorApplied: true,
		VaultNoteCount:    500,
		RelevanceZ:        0.30,
		TopHits:           []retrieval.ScoredResult{{ID: "arc-x", Title: "Arc", Score: 0.5}},
	}
	note := &index.FullNote{ID: "arc-x", Type: "arc", Title: "Arc", Body: "THE BODY IS RIGHT HERE."}

	var buf bytes.Buffer
	require.NoError(t, FormatAskRead(result, note, &buf))
	got := buf.String()

	require.Contains(t, got, "THE BODY IS RIGHT HERE.", "precondition: --read rendered the body")
	assert.NotContains(t, got, "body suppressed",
		"the body is printed a few lines below this claim; got:\n%s", got)
	assert.NotContains(t, got, "--read N to override",
		"offering --read as an override on the --read path is advice to do what was just done")
}

// countItemsExcerpted walks only ctx.Context, so an excerpted TARGET was never
// counted — the header said "2 items, 2 excerpted" while THREE blocks rendered
// and all three were excerpts. The target is in the list but not the count.
//
// ContextItem's own doc comment argues that "the agent got the note" and "the
// agent got the gist" must stay distinguishable. That applies hardest to the
// target, which is the block an agent reads first.
func TestContextHeader_CountsAnExcerptedTarget(t *testing.T) {
	ctx := &memory.ContextPackResult{
		TargetID:     "arc-decides",
		BudgetTokens: 900,
		UsedTokens:   300,
		Target: &memory.ContextPackTarget{
			ID: "arc-decides", BodyExcerpted: true, Body: "the principle",
			Frontmatter: map[string]interface{}{"type": "arc", "title": "Decides"},
		},
		Context: []memory.ContextItem{
			{ID: "a", BodyIncluded: true, BodyExcerpted: true, Body: "one"},
			{ID: "b"},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, writeContextHeader(&buf, ctx, formatOpts{}))
	got := buf.String()

	assert.Contains(t, got, "3 notes",
		"the target renders as a block and must be counted as one; got %q", got)
	assert.Contains(t, got, "2 delivered as excerpts",
		"one context item plus the target are excerpted; counting only the item under-reports "+
			"the block the agent reads first. got %q", got)
}

// A whole-bodied target must not be counted as an excerpt — the mirror error.
func TestContextHeader_DoesNotCountAWholeTargetAsExcerpted(t *testing.T) {
	ctx := &memory.ContextPackResult{
		TargetID:     "arc-decides",
		BudgetTokens: 900,
		UsedTokens:   300,
		Target:       &memory.ContextPackTarget{ID: "arc-decides", Body: "the whole note"},
		Context: []memory.ContextItem{
			{ID: "a", BodyIncluded: true, BodyExcerpted: true, Body: "one"},
			{ID: "b"},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, writeContextHeader(&buf, ctx, formatOpts{}))
	got := buf.String()

	assert.Contains(t, got, "1 as excerpt", "only the context item was capped")
	assert.NotContains(t, got, "2 as excerpt",
		"the target arrived whole; calling it an excerpt sends the agent to fetch what it already has")
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
