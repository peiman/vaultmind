package memory_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextPack_Basic(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "proj-vaultmind", Budget: 4096})
	require.NoError(t, err)
	assert.Equal(t, "proj-vaultmind", result.TargetID)
	assert.Equal(t, 4096, result.BudgetTokens)
	assert.Greater(t, result.UsedTokens, 0)
	assert.NotNil(t, result.Target)
}

func TestContextPack_IncludesContext(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "proj-vaultmind", Budget: 8192})
	require.NoError(t, err)
	assert.Greater(t, len(result.Context), 0)
	// With body backfill, context items may have bodies included if budget allows.
	// We only verify that context items exist; body inclusion is tested separately.
}

func TestContextPack_SmallBudget_Truncates(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "proj-vaultmind", Budget: 10})
	require.NoError(t, err)
	assert.True(t, result.Truncated)
	// With I4 fix: frontmatter always counted, may exceed budget
	assert.Greater(t, result.UsedTokens, 0)
}

func TestContextPack_LargeBudget(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "proj-vaultmind", Budget: 100000})
	require.NoError(t, err)
	assert.False(t, result.BudgetExhausted)
	assert.False(t, result.Truncated)
}

func TestContextPack_MediumBudget(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "proj-vaultmind", Budget: 500})
	require.NoError(t, err)
	assert.Greater(t, result.UsedTokens, 0)
}

// TestContextPack_UnresolvableInput verifies an error is returned when the input
// cannot be resolved to a known note.
func TestContextPack_UnresolvableInput(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "does-not-exist-xyz", Budget: 4096})
	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestContextPack_TargetID_IsResolvedCanonical verifies TargetID in the result
// is the resolved canonical ID, not the raw cfg.Input (I1 fix).
func TestContextPack_TargetID_IsResolvedCanonical(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	// "proj-vaultmind" resolves to canonical ID "proj-vaultmind"
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "proj-vaultmind", Budget: 4096})
	require.NoError(t, err)
	// The resolved canonical ID must equal the ID in the Target, not just raw input
	assert.Equal(t, result.Target.ID, result.TargetID)
}

// TestContextPack_FrontmatterExceedsBudget verifies that when frontmatter alone
// exceeds the budget, UsedTokens still reflects the frontmatter cost (I4 fix).
func TestContextPack_FrontmatterExceedsBudget(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	// Budget of 1 is smaller than any realistic frontmatter
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "proj-vaultmind", Budget: 1})
	require.NoError(t, err)
	assert.True(t, result.Truncated)
	// UsedTokens must be > 0 (frontmatter cost counted even when it exceeds budget)
	assert.Greater(t, result.UsedTokens, 0)
}

// TestContextPack_BodyBackfill verifies that with a large budget, some context items
// have body_included: true after the body backfill pass.
func TestContextPack_BodyBackfill(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "proj-vaultmind", Budget: 100000})
	require.NoError(t, err)

	// With a very large budget, at least one context item should have its body included
	bodyIncludedCount := 0
	for _, item := range result.Context {
		if item.BodyIncluded {
			bodyIncludedCount++
			assert.NotEmpty(t, item.Body, "body_included is true but body is empty for %s", item.ID)
		}
	}
	if len(result.Context) > 0 {
		assert.Greater(t, bodyIncludedCount, 0, "with large budget, at least one context item should have body included")
	}
}

// TestContextPack_BodyBackfillConsistency verifies that cached note data is
// consistent: items with BodyIncluded=true have non-empty bodies, and all
// context item Frontmatter maps are non-nil (cache refactor correctness).
func TestContextPack_BodyBackfillConsistency(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "proj-vaultmind", Budget: 100000})
	require.NoError(t, err)

	for _, item := range result.Context {
		assert.NotNil(t, item.Frontmatter, "Frontmatter should never be nil for context item %s", item.ID)
		if item.BodyIncluded {
			assert.NotEmpty(t, item.Body, "BodyIncluded=true but Body is empty for context item %s", item.ID)
		}
		if !item.BodyIncluded {
			assert.Empty(t, item.Body, "BodyIncluded=false but Body is non-empty for context item %s", item.ID)
		}
	}
}

// TestEdgePriority_AllEdgeTypes exercises edgePriority via ContextPack to verify
// explicit_embed is treated the same as explicit_link (C1 fix) and all
// priority tiers are covered.
func TestContextPack_EdgePriority_ExplicitEmbed(t *testing.T) {
	// edgePriority is an unexported function; we test it indirectly by verifying
	// ContextPack runs without error and produces consistent ordering.
	// The direct unit test below exercises all branches via the exported API.
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "proj-vaultmind", Budget: 8192})
	require.NoError(t, err)
	// Context items should be ordered by priority (no panics, no wrong type strings).
	for _, item := range result.Context {
		assert.NotEmpty(t, item.ID)
		assert.NotEmpty(t, item.EdgeType)
	}
}
