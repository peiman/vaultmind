package memory_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
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

// Found by inspecting real --json output after shipping: the target was capped
// at ExcerptTokens while a context item whose body happened to FIT was included
// whole — 2,685 characters (~670 tokens) against a requested cap of 80. The
// flag's own description says it "applies to the target note and every context
// item", so the documentation described behaviour the code did not have.
//
// A cap that only engages on overflow is not a cap. It also makes the budget
// unpredictable in exactly the setting this flag exists for: a hook that must
// fit several notes into a small, fixed allowance.
func TestContextPack_ExcerptCapsItemsThatWouldOtherwiseFit(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)

	const cap = 20

	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input: "proj-vaultmind", Budget: 8192, MaxItems: 3, ExcerptTokens: cap,
	})
	require.NoError(t, err)

	for _, item := range result.Context {
		if item.Body == "" {
			continue
		}
		assert.LessOrEqual(t, memory.EstimateTokens(item.Body), cap,
			"item %s carries %d tokens against a %d-token cap — a cap that only engages on overflow is not a cap",
			item.ID, memory.EstimateTokens(item.Body), cap)
		assert.True(t, item.BodyExcerpted,
			"item %s carries capped text, so it must be reported as an excerpt", item.ID)
	}
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

// #331: with ExcerptTokens 0, a body larger than the room left used to deliver
// nothing, and that room went to smaller, lower-ranked notes. OverflowExcerpt
// fills that room with the caller's chosen excerpt instead. Both packers do it.
func TestContextPack_OverflowExcerptFillsTheRoomLeft(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	const tightBudget = 200
	for _, maxItems := range []int{3, 0} {
		cfg := memory.ContextPackConfig{Input: "proj-vaultmind", Budget: tightBudget, MaxItems: maxItems}
		before, err := memory.ContextPack(resolver, db, cfg)
		require.NoError(t, err)
		cfg.OverflowExcerpt = func(n *index.FullNote, maxTokens int) (string, string) {
			return memory.Excerpt(n.Body, maxTokens), "stub"
		}
		after, err := memory.ContextPack(resolver, db, cfg)
		require.NoError(t, err)

		assert.Zero(t, withText(before), "max_items=%d: without the option an over-budget body delivers nothing", maxItems)
		assert.Positive(t, withText(after), "max_items=%d: the over-budget body must deliver an excerpt", maxItems)
		assert.LessOrEqual(t, after.UsedTokens, tightBudget)
		for _, item := range after.Context {
			if item.Body != "" {
				assert.True(t, item.BodyIncluded && item.BodyExcerpted, "item %s", item.ID)
				assert.False(t, item.BodyOmittedBudget, "item %s carries text, so it was not omitted", item.ID)
				assert.Equal(t, "stub", item.PassageSelector, "item %s names what chose its text", item.ID)
			}
		}
	}
}

// An OverflowExcerpt that returns more than the room it was given cannot
// break the budget: the packer's own excerpt replaces it.
func TestContextPack_OverflowExcerptOverRoomFallsBack(t *testing.T) {
	db := buildTestDB(t)
	const tightBudget = 200
	result, err := memory.ContextPack(graph.NewResolver(db), db, memory.ContextPackConfig{
		Input: "proj-vaultmind", Budget: tightBudget, MaxItems: 3,
		OverflowExcerpt: func(n *index.FullNote, _ int) (string, string) { return n.Body + n.Body, "greedy" },
	})
	require.NoError(t, err)

	assert.Positive(t, withText(result))
	assert.LessOrEqual(t, result.UsedTokens, tightBudget)
	for _, item := range result.Context {
		if item.Body != "" {
			assert.Equal(t, "none", item.PassageSelector, "item %s", item.ID)
		}
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
