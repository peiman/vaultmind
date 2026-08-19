package query

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// THE INVARIANT
//
// Every number in the context header must be verifiable by counting the blocks
// printed underneath it. Nothing else keeps the header honest: three separate
// defects this session were a header describing internal state that no longer
// matched the render.
//
//   - "0 items, 900/900 tokens" — a full budget and nothing in it, because
//     UsedTokens counted a body that pointers-only then discarded.
//   - "2 items, 2 excerpted" while three blocks rendered — the target is
//     rendered but was not in len(ctx.Context).
//   - "8 items, 0 with bodies, 9 excerpted" — nine bodies delivered, reported
//     as zero, because countItemsWithBodies required !BodyExcerpted.
//
// Each was individually plausible and individually fixed. This test is the
// check that makes the class impossible, which is the only thing that has
// actually worked: a correct number is one the reader could recount.
//
// The counts are parsed back OUT of the rendered header and compared against
// the rendered body. A wording change that keeps the grammar keeps this test;
// a counting change that drifts from the render fails it.
var headerCounts = regexp.MustCompile(`(\d+) notes?, (\d+) delivered`)

// countRenderedBlocks returns (notes, delivered) as a reader would tally them:
// a note is a "  [type] Title" line, and it was delivered if indented text
// follows it. Bodies are indented on EVERY line precisely so this is decidable
// — a multi-line excerpt used to emit ragged continuation lines that could
// impersonate a note line.
func countRenderedBlocks(out string) (notes, delivered int) {
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "  [") {
			continue
		}
		notes++
		if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "    ") {
			delivered++
		}
	}
	return notes, delivered
}

func TestContextHeader_EveryNumberIsRecountable(t *testing.T) {
	target := func(body string, excerpted bool) *memory.ContextPackTarget {
		return &memory.ContextPackTarget{
			Frontmatter:   map[string]any{"type": "arc", "title": "The Target"},
			Body:          body,
			BodyExcerpted: excerpted,
		}
	}
	whole := memory.ContextItem{
		ID: "whole", BodyIncluded: true, Body: "a whole body",
		Frontmatter: map[string]any{"type": "reference", "title": "Whole"},
	}
	excerpt := memory.ContextItem{
		ID: "excerpt", BodyIncluded: true, BodyExcerpted: true, Body: "the principle",
		Frontmatter: map[string]any{"type": "arc", "title": "Excerpt"},
	}
	// A multi-line excerpt: the shape that made the render ambiguous.
	multi := memory.ContextItem{
		ID: "multi", BodyIncluded: true, BodyExcerpted: true,
		Body:        "first line of the principle\n  [not] a note line\nthird line",
		Frontmatter: map[string]any{"type": "arc", "title": "Multiline"},
	}
	titleOnly := memory.ContextItem{
		ID: "bare", Frontmatter: map[string]any{"type": "concept", "title": "Bare"},
	}

	cases := []struct {
		name  string
		ctx   *memory.ContextPackResult
		opts  formatOpts
		shape string // the phrase the header must use for this mix
	}{
		{
			name:  "all excerpts — the live SessionStart case that reported zero",
			ctx:   &memory.ContextPackResult{TargetID: "t", BudgetTokens: 6000, UsedTokens: 982, Target: target("the rule", true), Context: []memory.ContextItem{excerpt, multi}},
			shape: "3 notes, 3 delivered as excerpts",
		},
		{
			name:  "all whole",
			ctx:   &memory.ContextPackResult{TargetID: "t", BudgetTokens: 900, UsedTokens: 120, Target: target("whole target", false), Context: []memory.ContextItem{whole}},
			shape: "2 notes, 2 delivered in full",
		},
		{
			name:  "mixed whole and excerpt",
			ctx:   &memory.ContextPackResult{TargetID: "t", BudgetTokens: 900, UsedTokens: 400, Target: target("whole target", false), Context: []memory.ContextItem{whole, excerpt}},
			shape: "3 notes, 3 delivered, 1 as excerpt",
		},
		{
			name:  "budget ran out — some notes are titles only",
			ctx:   &memory.ContextPackResult{TargetID: "t", BudgetTokens: 900, UsedTokens: 880, Target: target("the rule", true), Context: []memory.ContextItem{excerpt, titleOnly, titleOnly}},
			shape: "4 notes, 2 delivered as excerpts, 2 titles only",
		},
		{
			name:  "nothing delivered at all",
			ctx:   &memory.ContextPackResult{TargetID: "t", BudgetTokens: 900, UsedTokens: 20, Target: target("", false), Context: []memory.ContextItem{titleOnly}},
			shape: "2 notes, 0 delivered",
		},
		{
			name:  "pointers-only withholds text the pack assembled",
			ctx:   &memory.ContextPackResult{TargetID: "t", BudgetTokens: 900, UsedTokens: 900, Target: target("the rule", true), Context: []memory.ContextItem{excerpt}},
			opts:  formatOpts{pointersOnly: true},
			shape: "2 notes, 0 delivered",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, writeContextHeader(&buf, tc.ctx, tc.opts))
			require.NoError(t, writeContextTarget(&buf, tc.ctx.Target, tc.opts))
			require.NoError(t, writeContextItems(&buf, tc.ctx.Context, tc.opts))
			out := buf.String()

			m := headerCounts.FindStringSubmatch(out)
			require.NotNil(t, m,
				"the header must state %q as a recountable pair; got:\n%s", "N notes, M delivered", out)

			notes, delivered := countRenderedBlocks(out)
			assert.Equal(t, atoi(t, m[1]), notes,
				"header claims %s notes; %d blocks rendered:\n%s", m[1], notes, out)
			assert.Equal(t, atoi(t, m[2]), delivered,
				"header claims %s delivered; %d blocks carry text:\n%s", m[2], delivered, out)

			assert.Contains(t, out, tc.shape)

			// Non-vacuity: the fixtures must actually render something. If
			// countRenderedBlocks returned 0,0 the equality assertions above
			// would pass against a header of all zeroes.
			require.Positive(t, notes, "no note line rendered — the case asserts nothing")
		})
	}
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, r := range s {
		require.True(t, r >= '0' && r <= '9', "not a number: %q", s)
		n = n*10 + int(r-'0')
	}
	return n
}

// The budget denominator appears when, and only when, the budget bound the
// result. "982/6000" on a pack that got everything it asked for reads as "the
// vault had nothing more to give"; it means "a per-note cap bound this, and
// 5,018 tokens went unspent" — a different fact, and the one that earned the
// old, wrong "bound by --excerpt" note. That note fired on a budget RATIO
// (unspent > budget/2), which --max-items or a small vault produce just as
// easily. Naming the budget only when notes were actually dropped for it is
// the same information with none of the guessing.
func TestContextHeader_NamesTheBudgetOnlyWhenItBound(t *testing.T) {
	full := &memory.ContextPackResult{
		TargetID: "t", BudgetTokens: 6000, UsedTokens: 982,
		Target: &memory.ContextPackTarget{
			Frontmatter: map[string]any{"type": "arc", "title": "T"}, Body: "rule", BodyExcerpted: true,
		},
	}
	var buf bytes.Buffer
	require.NoError(t, writeContextHeader(&buf, full, formatOpts{}))
	require.NoError(t, writeContextFooter(&buf, full, formatOpts{}))
	assert.NotContains(t, buf.String(), "6000",
		"nothing was dropped, so the budget is not what limited this")

	bound := &memory.ContextPackResult{
		TargetID: "t", BudgetTokens: 900, UsedTokens: 890,
		Target: &memory.ContextPackTarget{
			Frontmatter: map[string]any{"type": "arc", "title": "T"}, Body: "rule", BodyExcerpted: true,
		},
		Context: []memory.ContextItem{{ID: "bare", Frontmatter: map[string]any{"type": "concept", "title": "B"}}},
	}
	buf.Reset()
	require.NoError(t, writeContextHeader(&buf, bound, formatOpts{}))
	require.NoError(t, writeContextFooter(&buf, bound, formatOpts{}))
	out := buf.String()
	assert.Contains(t, out, "900", "a note was dropped for budget; name the budget that dropped it")
	assert.Contains(t, out, "--budget", "and the flag that changes it")
}
