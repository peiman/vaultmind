package memory_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// EVERY note of the type, not the top-K.
//
// This is the whole point of #47. `ask "who am I"` delivered 3 of 27 arcs on a
// real vault — the same 3 every run, chosen by similarity to a literal string
// that has nothing to do with the session. An identity layer is not a ranking
// problem.
func TestRecite_ReturnsEveryNoteOfTheType(t *testing.T) {
	db := buildTestDB(t)

	// Counted INDEPENDENTLY of the code under test. The first version of this
	// test asserted len(Items) == Total, and both are derived from the same
	// list — so a mutation that truncated the list to 3 shrank both and the
	// test stayed green. Verified by mutation; it was decorative.
	var want int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM notes WHERE type = ?`, "concept").Scan(&want))
	require.Greater(t, want, 3, "fixture needs enough concepts that a top-K truncation would be visible")

	got, err := memory.Recite(db, memory.ReciteConfig{Type: "concept"})
	require.NoError(t, err)

	assert.Equal(t, want, got.Total, "Total must reflect the whole layer")
	assert.Len(t, got.Items, want,
		"unbounded recite must return the whole layer, not a selection")
	assert.False(t, got.Truncated())
}

// Deterministic order. An identity layer that arrives differently each session
// is a different identity each session, and nothing can be compared run to run.
func TestRecite_OrderIsDeterministicAndSorted(t *testing.T) {
	db := buildTestDB(t)

	first, err := memory.Recite(db, memory.ReciteConfig{Type: "concept"})
	require.NoError(t, err)
	second, err := memory.Recite(db, memory.ReciteConfig{Type: "concept"})
	require.NoError(t, err)

	require.NotEmpty(t, first.Items)
	ids := func(r *memory.ReciteResult) []string {
		out := make([]string, 0, len(r.Items))
		for _, it := range r.Items {
			out = append(out, it.ID)
		}
		return out
	}
	assert.Equal(t, ids(first), ids(second), "two runs must agree")
	assert.IsIncreasing(t, ids(first), "order must be by id, not by insertion or rank")
}

// THE ONE THAT MATTERS: truncation is reported, never silent.
//
// A bulk loader that drops the tail to fit a budget is the same defect #47
// describes, in new clothes — the caller cannot tell "here are your 27 arcs"
// from "here are 9 of them". The count AND the ids of what was dropped have to
// come back.
func TestRecite_BudgetTruncationIsReportedWithTheOmittedIDs(t *testing.T) {
	db := buildTestDB(t)

	full, err := memory.Recite(db, memory.ReciteConfig{Type: "concept"})
	require.NoError(t, err)
	require.Greater(t, full.Total, 1, "need at least two notes to truncate meaningfully")

	// A budget that admits roughly the first note only.
	tight := full.Items[0].Tokens
	got, err := memory.Recite(db, memory.ReciteConfig{Type: "concept", Budget: tight})
	require.NoError(t, err)

	assert.True(t, got.Truncated(), "a budget below the layer's size must report truncation")
	assert.Equal(t, got.Total, len(got.Items)+got.Omitted,
		"every note must be accounted for as either delivered or omitted")
	assert.Len(t, got.OmittedIDs, got.Omitted,
		"the omitted COUNT without the omitted IDS still leaves the caller unable to see what is missing")
	assert.LessOrEqual(t, got.Tokens, tight, "the budget must actually bound the output")
}

func TestRecite_MaxBytesReportsOverflowIDsInAscendingOrder(t *testing.T) {
	db := buildTestDB(t)

	full, err := memory.Recite(db, memory.ReciteConfig{Type: "concept"})
	require.NoError(t, err)
	cap := len(memory.RenderRecitation(full)) / 2
	got, err := memory.Recite(db, memory.ReciteConfig{Type: "concept", MaxBytes: cap})
	require.NoError(t, err)
	require.Greater(t, got.Overflow, 1, "need multiple overflow notes to verify report ordering")
	assert.LessOrEqual(t, len(memory.RenderRecitation(got)), cap)
	assert.IsIncreasing(t, got.OverflowIDs, "overflow IDs must retain ascending shard/id order")
	accounted := make(map[string]int)
	for _, item := range got.Items {
		accounted[item.ID]++
	}
	for _, id := range got.OmittedIDs {
		accounted[id]++
	}
	for _, id := range got.OverflowIDs {
		accounted[id]++
	}
	assert.Len(t, accounted, full.Total)
	for id, count := range accounted {
		assert.Equal(t, 1, count, id)
	}
}

func TestRenderRecitation_HardCapsLongOverflowReportsAndJSONKeepsAllIDs(t *testing.T) {
	ids := make([]string, 30)
	for i := range ids {
		ids[i] = "arc-" + strings.Repeat("x", 20) + string(rune('a'+i))
	}
	for _, cap := range []int{40, 200} {
		r := &memory.ReciteResult{Type: "arc", Total: len(ids), MaxBytes: cap, Overflow: len(ids), OverflowIDs: ids}
		text := memory.RenderRecitation(r)
		assert.LessOrEqual(t, len(text), cap)
		assert.Contains(t, text, "+", "a bounded overflow list must report its remaining count")
		if cap == 40 {
			assert.Contains(t, text, "+30 more arcs")
		}
	}
	raw, err := json.Marshal(&memory.ReciteResult{Type: "arc", Total: len(ids), MaxBytes: 40, Overflow: len(ids), OverflowIDs: ids})
	require.NoError(t, err)
	for _, id := range ids {
		assert.Contains(t, string(raw), id)
	}
}

// Excerpting must not change WHICH notes arrive — only how much of each.
// Coverage of the layer and depth per note are separate decisions.
func TestRecite_ExcerptingShrinksContentNotCoverage(t *testing.T) {
	db := buildTestDB(t)

	full, err := memory.Recite(db, memory.ReciteConfig{Type: "concept"})
	require.NoError(t, err)
	excerpted, err := memory.Recite(db, memory.ReciteConfig{Type: "concept", ExcerptTokens: 20})
	require.NoError(t, err)

	require.NotEmpty(t, full.Items)
	assert.Equal(t, len(full.Items), len(excerpted.Items),
		"an excerpt cap must not drop notes")
	assert.Less(t, excerpted.Tokens, full.Tokens,
		"excerpting must actually reduce the token cost or it is doing nothing")
	for _, it := range excerpted.Items {
		assert.LessOrEqual(t, it.Tokens, 20+8,
			"each excerpt must respect its cap (small slack for boundary rounding)")
	}
}

// An unknown type is an empty layer, not an error — but Total must say 0 so a
// caller can tell "no arcs yet" from "arcs failed to load".
func TestRecite_UnknownTypeIsAnEmptyLayerNotAFailure(t *testing.T) {
	db := buildTestDB(t)

	got, err := memory.Recite(db, memory.ReciteConfig{Type: "no-such-type-exists"})
	require.NoError(t, err)
	assert.Zero(t, got.Total)
	assert.Empty(t, got.Items)
	assert.False(t, got.Truncated())
}

func TestRecite_RequiresAType(t *testing.T) {
	db := buildTestDB(t)
	_, err := memory.Recite(db, memory.ReciteConfig{})
	require.Error(t, err, "reciting with no type would silently enumerate nothing")
}

// The budget boundary is EXACT: a layer that fits precisely must arrive whole.
//
// Found by gremlins, not by me. I hand-picked two mutations for recite.go and
// both were caught; automated mutation then killed `>` -> `>=` and the negation
// at the same line and nothing failed. An off-by-one here means the last arc
// silently becomes "omitted" on a budget that exactly fits it — the identity
// layer quietly losing its tail, which is the defect this file exists to
// prevent.
func TestRecite_ABudgetThatExactlyFitsDeliversEverything(t *testing.T) {
	db := buildTestDB(t)

	full, err := memory.Recite(db, memory.ReciteConfig{Type: "concept", ExcerptTokens: 20})
	require.NoError(t, err)
	require.NotEmpty(t, full.Items)
	require.False(t, full.Truncated(), "precondition: unbounded must deliver the whole layer")

	// Exactly the tokens the layer needs — not one more.
	exact, err := memory.Recite(db, memory.ReciteConfig{
		Type: "concept", ExcerptTokens: 20, Budget: full.Tokens,
	})
	require.NoError(t, err)

	assert.False(t, exact.Truncated(),
		"a budget equal to the layer's cost must fit it — `>` must not be `>=`")
	assert.Len(t, exact.Items, len(full.Items))

	// And one token short must drop exactly the tail, still reported.
	short, err := memory.Recite(db, memory.ReciteConfig{
		Type: "concept", ExcerptTokens: 20, Budget: full.Tokens - 1,
	})
	require.NoError(t, err)
	assert.True(t, short.Truncated(), "one token short must not silently fit")
	assert.Equal(t, 1, short.Omitted, "exactly the last note should fall out, not more")
}
