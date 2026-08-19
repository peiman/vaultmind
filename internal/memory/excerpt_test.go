package memory_test

import (
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
)

// An arc's payload is not its opening. The arc discipline
// (vaultmind-identity/principles/how-to-write-arcs.md) structures every arc as
// trigger / push / deeper sight / principle / what I did differently — so the
// FIRST paragraph is the story setup and the DECISION RULE is several sections
// down. A delivery path that injects "the lead paragraph" hands an agent the
// anecdote and withholds the lesson.
//
// Measured on the live identity vault: the lead of arc-the-one-who-uses-it-decides
// is "Peiman had asked for a stronger noise-floor estimator…" (setup), while its
// Principle is "When you are the consumer of what you build, a request is a
// hypothesis, not an order…" (the rule that changes what an agent does).
const arcBody = `Trigger

Peiman had asked for a stronger noise-floor estimator — one of three follow-ups
he picked off a menu. I built it exactly as asked.

Push

He said: decide what you want to do.

Deeper sight

I had been treating a request as an order.

Principle

When you are the consumer of what you build, a request is a hypothesis, not an
order — you hold the authority to probe it and to revert it when your own use of
the tool says it is wrong.

What I did differently

I reverted P90 to the conservative max estimator.

Source

session transcript`

func TestExcerpt_PrefersPrincipleOverLead(t *testing.T) {
	got := memory.Excerpt(arcBody, 120)

	assert.Contains(t, got, "a request is a hypothesis",
		"the Principle section is the decision rule — it is what makes an injection actionable")
	assert.NotContains(t, got, "Peiman had asked",
		"the Trigger is story setup; delivering it instead of the Principle is why pointer blocks read as noise")
}

// Without a Principle section (references, concepts, sources), the opening
// paragraph IS the substance, so fall back to it.
func TestExcerpt_FallsBackToLeadWhenNoPrinciple(t *testing.T) {
	body := "Probe before committing.\n\nWhen work is complex or speculative, run cheap experiments first.\n\nThis shipped real architecture in one session."

	got := memory.Excerpt(body, 120)

	assert.Contains(t, got, "Probe before committing")
}

// The excerpt must fit the budget it is given — that is the whole point. The
// shipped packer is all-or-nothing (contextpack.go: `if bodyTokens <= *remaining`),
// so a note larger than the leftover budget contributes NOTHING and the pack
// reports "3 items, 0 with bodies". An excerpt that overruns would reproduce
// exactly that failure.
func TestExcerpt_RespectsTokenBudget(t *testing.T) {
	got := memory.Excerpt(arcBody, 20)

	assert.LessOrEqual(t, memory.EstimateTokens(got), 20,
		"an excerpt that exceeds its budget gets dropped by the packer and delivers nothing")
	assert.NotEmpty(t, got, "a tight budget must still deliver something, not silence")
}

// Truncation mid-word is what the current teaser does ("…one of three follow-ups
// he…"). An excerpt should end on a sentence where it can.
func TestExcerpt_TruncatesOnSentenceBoundary(t *testing.T) {
	got := memory.Excerpt(arcBody, 30)

	trimmed := strings.TrimSpace(got)
	assert.True(t, strings.HasSuffix(trimmed, ".") || strings.HasSuffix(trimmed, "…"),
		"got %q — truncation should land on a sentence or an explicit ellipsis, never mid-word", trimmed)
}

// Caught by running Excerpt against the LIVE vault rather than this file's
// fixtures: a real note begins with a YAML frontmatter block, and the first
// "paragraph" of a raw note file is therefore `--- id: … type: … tags: …`.
// Without this, a reference note's excerpt is metadata — the one thing an agent
// at a decision point cannot use.
//
// The fixtures above have no frontmatter, so they could never have found this.
// See feedback_checker_validates_assumption_not_reality: a test whose fixture
// encodes the same assumption as the code is not evidence the code is right.
func TestExcerpt_StripsFrontmatter(t *testing.T) {
	body := `---
id: reference-probe-before-commit
type: reference
tags:
  - method
---

Probe before committing. When work is complex or speculative, run cheap
experiments before committing to an estimate.`

	got := memory.Excerpt(body, 120)

	assert.NotContains(t, got, "reference-probe-before-commit", "frontmatter is metadata, not memory")
	assert.NotContains(t, got, "type:")
	assert.Contains(t, got, "Probe before committing")
}

// Also caught on the live vault, immediately after the frontmatter fix: with the
// YAML gone, the first line of a real note is its Markdown H1 — so the excerpt
// became "# Probe Before Committing", a title the agent already has from the
// pointer line. Any heading is navigation, not memory.
func TestExcerpt_SkipsMarkdownHeadings(t *testing.T) {
	body := `# Probe Before Committing

When work is complex or speculative, probe first — run cheap experiments before
committing to an estimate or an architecture.`

	got := memory.Excerpt(body, 120)

	assert.NotContains(t, got, "#", "a Markdown heading is navigation, not content")
	assert.Contains(t, got, "probe first")
}

func TestExcerpt_EmptyBody(t *testing.T) {
	assert.Empty(t, memory.Excerpt("", 100))
	assert.Empty(t, memory.Excerpt("   \n\n  ", 100))
}
