package memory_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Body inclusion is all-or-nothing: `if bodyTokens <= *remaining`. A note whose
// body does not fit the LEFTOVER budget contributes nothing, and the pack
// reports "N items, 0 with bodies" — items present, content absent.
//
// That is the common case, not the edge case. Measured on a real identity vault
// (63 notes, median ~1,084 tokens) against the reach hook's 900-token budget:
// budget 1500 -> 0 bodies, 3000 -> 1, 6000 -> 2. The agent is handed a list of
// titles and told to go read them itself.
//
// ExcerptTokens turns "no body" into "the part that matters". It is opt-in
// (0 = unchanged), matching how MaxItems == 0 preserves legacy packing.
func TestContextPack_ExcerptFillsWhereBodyWouldNotFit(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)

	// Chosen by sweeping the fixture vault: at 200 the frontmatter fits but no
	// whole body does, which is the shape a 900-token hook budget has against a
	// vault whose median note is larger than the budget. At >=250 this fixture's
	// bodies fit outright and the excerpt path correctly does nothing.
	const tightBudget = 200

	// Baseline: today's behaviour at a budget too small for whole bodies.
	before, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input: "proj-vaultmind", Budget: tightBudget, MaxItems: 3,
	})
	require.NoError(t, err)

	after, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input: "proj-vaultmind", Budget: tightBudget, MaxItems: 3, ExcerptTokens: 60,
	})
	require.NoError(t, err)

	assert.Greater(t, withText(after), withText(before),
		"at a budget where whole bodies do not fit, excerpts must deliver text where today delivers none")
	assert.LessOrEqual(t, after.UsedTokens, tightBudget,
		"an excerpt that overruns the budget would be dropped by the packer — the very failure this fixes")
}

// The excerpt path must not degrade the case that already works: when a body
// fits, the full body is still delivered.
func TestContextPack_ExcerptDoesNotReplaceFullBodies(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)

	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input: "proj-vaultmind", Budget: 8192, MaxItems: 3, ExcerptTokens: 60,
	})
	require.NoError(t, err)

	// BodyIncluded keeps its released meaning — "this item carries note text" —
	// so every existing formatter renders excerpts without modification.
	// BodyExcerpted is the narrower fact: that text is partial.
	for _, item := range result.Context {
		if item.BodyExcerpted {
			assert.True(t, item.BodyIncluded,
				"item %s: an excerpt is still included text; formatters gate on BodyIncluded", item.ID)
		}
	}
	assert.Positive(t, withText(result), "a large budget must still deliver whole bodies")
}

// ExcerptTokens == 0 is the released behaviour and must stay byte-identical, so
// upgrading cannot silently change what an existing caller is handed.
func TestContextPack_ExcerptOptInIsDefaultOff(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	cfg := memory.ContextPackConfig{Input: "proj-vaultmind", Budget: 400, MaxItems: 3}

	a, err := memory.ContextPack(resolver, db, cfg)
	require.NoError(t, err)
	b, err := memory.ContextPack(resolver, db, cfg)
	require.NoError(t, err)

	assert.Equal(t, a.UsedTokens, b.UsedTokens)
	for i := range a.Context {
		assert.False(t, a.Context[i].BodyExcerpted, "no excerpting without opting in")
	}
}

// withText counts items carrying any note prose — full body or excerpt. It is
// the number the header should be reporting: "0 with bodies" is only honest if
// the agent truly received nothing.
func withText(r *memory.ContextPackResult) int {
	n := 0
	for _, item := range r.Context {
		if item.Body != "" {
			n++
		}
	}
	return n
}
