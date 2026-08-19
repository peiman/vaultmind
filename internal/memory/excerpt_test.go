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

// Found by an adversarial probe during review, and it is this whole change-set's
// own failure mode turned inward: at a budget too small for even one word,
// Excerpt returned a bare "…", attachBody stored it, and the item was marked
// BodyIncluded — an item counted as carrying content whose entire content was an
// ellipsis. A pack could then honestly report "3 items, 3 excerpted" and hand
// the agent three ellipses.
//
// Returning "" instead makes that unexpressible: every caller already skips an
// empty excerpt, so a budget that cannot carry a word now yields no item rather
// than a fake one.
func TestExcerpt_DegenerateBudgetYieldsNothingNotAnEllipsis(t *testing.T) {
	body := "Principle\n\nAlways verify the delivery path end to end before trusting a count."

	for _, budget := range []int{1, 2} {
		got := memory.Excerpt(body, budget)
		assert.Empty(t, got,
			"budget %d cannot carry a word; %q is a delivery that delivers nothing", budget, got)
	}

	// A single unbreakable token (a URL) as the whole body — same rule.
	assert.Empty(t, memory.Excerpt("https://example.com/a-very-long-single-token-url-that-never-fits", 2))

	// And the first budget that CAN carry a word still does.
	assert.Contains(t, memory.Excerpt(body, 3), "Always")
}

// THE INVARIANT: a non-empty body must never excerpt to nothing when the budget
// can carry a word. Anything else is content loss dressed as a cap.
//
// Found by an adversarial probe, and it was a regression introduced by making
// the cap unconditional: before that, a bullet-only note whose body fit was
// delivered WHOLE; after, it was routed through Excerpt, which looked for
// "prose" — a block containing .!? or longer than a heading — found none in a
// list of short bullets, and returned "". So a note that used to arrive intact
// silently arrived empty.
//
// CJK is the same bug wearing different clothes: 研究は重要である。 ends in a full
// stop the ASCII check does not recognise, and is too short to clear the length
// fallback.
func TestExcerpt_NeverDropsANonEmptyBody(t *testing.T) {
	cases := map[string]string{
		"bullet list":       "- alpha\n- beta\n- gamma",
		"markdown table":    "| a | b |\n|---|---|\n| 1 | 2 |",
		"heading + bullets": "# Notes\n\n- one\n- two",
		"CJK prose":         "研究は重要である。",
		"single short word": "ship",
		"prose":             "This is a real sentence that should survive.",
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			got := memory.Excerpt(body, 900)
			assert.NotEmpty(t, got,
				"body %q excerpted to nothing at a 900-token budget — the cap dropped content instead of bounding it", body)
			assert.LessOrEqual(t, memory.EstimateTokens(got), 900)
		})
	}
}

// The prose preference still holds where prose exists — the fallback must not
// flatten the Principle-first behaviour into "just return the head".
func TestExcerpt_FallbackDoesNotOverrideProsePreference(t *testing.T) {
	got := memory.Excerpt(arcBody, 120)

	assert.Contains(t, got, "a request is a hypothesis")
	assert.NotContains(t, got, "Peiman had asked")
}

// Only 15 of 81 notes in a real identity vault have a Principle section, so the
// lead-paragraph fallback runs for roughly 96% of notes. It accepted anything
// containing a period — and a file path contains several.
//
// Live consequence, seen in an actual hook injection: the excerpt delivered for
// reference-session-transcript was
// `~/.claude/projects/-Users-peiman.../663a071c-....jsonl`, presented under the
// banner "the excerpt above is the decision rule, not a pointer to one".
//
// A path is one unbroken token with slashes in it. Prose has whitespace. That
// distinction is enough, and it leaves CJK — which also has no spaces but no
// slashes either — correctly classified as prose.
func TestExcerpt_SkipsPathsAndURLs(t *testing.T) {
	cases := map[string]struct{ body, wantNot, want string }{
		"leading file path": {
			body:    "~/.claude/projects/-Users-peiman-dev/663a071c-c343.jsonl\n\nThe session where the arc method was found.",
			wantNot: ".jsonl",
			want:    "arc method",
		},
		"leading URL": {
			body:    "https://example.com/papers/anderson-1983.pdf\n\nBase-level activation decays with time since last retrieval.",
			wantNot: "https://",
			want:    "Base-level activation",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := memory.Excerpt(tc.body, 120)
			assert.NotContains(t, got, tc.wantNot,
				"a bare path or URL is a citation, not a decision rule; got %q", got)
			assert.Contains(t, got, tc.want)
		})
	}
}

// A path is only skipped when there is real prose to prefer. A note that is
// nothing but a path still delivers it — the invariant that a non-empty body
// never excerpts to nothing outranks tidiness.
func TestExcerpt_PathOnlyNoteStillDelivers(t *testing.T) {
	got := memory.Excerpt("~/.claude/projects/only-a-path.jsonl", 120)
	assert.NotEmpty(t, got, "there is nothing better to show; showing nothing is worse")
}

func TestExcerpt_EmptyBody(t *testing.T) {
	assert.Empty(t, memory.Excerpt("", 100))
	assert.Empty(t, memory.Excerpt("   \n\n  ", 100))
}
