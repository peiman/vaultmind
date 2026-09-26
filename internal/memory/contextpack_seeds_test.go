package memory_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// outsider returns a fixture note that is neither the target nor in its
// unseeded pack, so a test can prove a seed arrived by being seeded rather
// than by already being a neighbour.
func outsider(t *testing.T, base *memory.ContextPackResult) string {
	t.Helper()
	inPack := map[string]bool{base.TargetID: true}
	for _, item := range base.Context {
		inPack[item.ID] = true
	}
	for _, id := range []string{"concept-forgetting-curve", "concept-episodic-memory", "source-anderson-1983", "decision-example", "concept-act-r"} {
		if !inPack[id] {
			return id
		}
	}
	t.Fatal("fixture has no note outside the unseeded pack")
	return ""
}

func TestContextPack_SeedsLeadThePackInRankOrder(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	cfg := memory.ContextPackConfig{Input: "concept-spreading-activation", Budget: 8192}
	base, err := memory.ContextPack(resolver, db, cfg)
	require.NoError(t, err)
	seed := outsider(t, base)

	cfg.Seeds = []memory.Seed{{ID: seed, Confidence: "moderate"}}
	got, err := memory.ContextPack(resolver, db, cfg)
	require.NoError(t, err)

	require.NotEmpty(t, got.Context)
	assert.Equal(t, seed, got.Context[0].ID, "a seed goes ahead of every graph neighbour")
	assert.Equal(t, memory.EdgeTypeSearchHit, got.Context[0].EdgeType)
	assert.Equal(t, "moderate", got.Context[0].Confidence, "the seed carries its own relevance band")
	assert.Len(t, got.Context, len(base.Context)+1, "seeding adds the note without dropping neighbours at this budget")
}

func TestContextPack_SeedsSkipTargetDuplicatesAndUnknownNotes(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	cfg := memory.ContextPackConfig{Input: "concept-spreading-activation", Budget: 8192}
	base, err := memory.ContextPack(resolver, db, cfg)
	require.NoError(t, err)
	require.NotEmpty(t, base.Context, "fixture target must have a neighbour to seed")
	neighbour := base.Context[0].ID

	cfg.Seeds = []memory.Seed{
		{ID: base.TargetID, Confidence: "strong"},
		{ID: "no-such-note", Confidence: "strong"},
		{ID: neighbour, Confidence: "moderate"},
		{ID: neighbour, Confidence: "moderate"},
	}
	got, err := memory.ContextPack(resolver, db, cfg)
	require.NoError(t, err)

	counts := map[string]int{}
	for _, item := range got.Context {
		counts[item.ID]++
	}
	assert.Zero(t, counts[base.TargetID], "the target is never repeated as context")
	assert.Zero(t, counts["no-such-note"], "a seed that names no note is skipped, not an error")
	assert.Equal(t, 1, counts[neighbour], "a seed that is also a neighbour appears once")
	require.NotEmpty(t, got.Context)
	assert.Equal(t, neighbour, got.Context[0].ID)
	assert.Equal(t, memory.EdgeTypeSearchHit, got.Context[0].EdgeType, "the search-hit reading wins over the edge")
	assert.Len(t, got.Context, len(base.Context), "no note was added or lost")
}

func TestContextPack_SeedsCountTowardMaxItems(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	cfg := memory.ContextPackConfig{Input: "concept-spreading-activation", Budget: 8192}
	base, err := memory.ContextPack(resolver, db, cfg)
	require.NoError(t, err)
	seed := outsider(t, base)

	cfg.MaxItems = 1
	cfg.Seeds = []memory.Seed{{ID: seed, Confidence: "strong"}}
	got, err := memory.ContextPack(resolver, db, cfg)
	require.NoError(t, err)

	require.Len(t, got.Context, 1)
	assert.Equal(t, seed, got.Context[0].ID, "with one slot, the seed takes it")
}
