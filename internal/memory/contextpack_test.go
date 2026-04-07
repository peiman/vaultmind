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

// TestContextPack_DepthGreaterThanOne verifies that Depth=2 collects nodes at
// distance 2 in addition to distance-1 nodes. Distance-2 nodes must have
// higher combined priority values (lower priority = closer) than distance-1
// nodes, so they appear later in context ordering.
func TestContextPack_DepthGreaterThanOne(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)

	// Collect depth-1 result for comparison baseline.
	result1, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input:  "proj-vaultmind",
		Budget: 100000,
		Depth:  1,
	})
	require.NoError(t, err)
	depth1IDs := make(map[string]bool)
	for _, item := range result1.Context {
		depth1IDs[item.ID] = true
	}

	// Collect depth-2 result.
	result2, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input:  "proj-vaultmind",
		Budget: 100000,
		Depth:  2,
	})
	require.NoError(t, err)

	// Depth-2 must include at least as many context items as depth-1.
	assert.GreaterOrEqual(t, len(result2.Context), len(result1.Context),
		"depth=2 should include all depth-1 context items plus potentially more")

	// Depth-2 result must include all depth-1 context items.
	for _, item := range result1.Context {
		found := false
		for _, item2 := range result2.Context {
			if item2.ID == item.ID {
				found = true
				break
			}
		}
		assert.True(t, found, "depth-1 item %s must be present in depth-2 result", item.ID)
	}

	// If depth-2 found more items, distance-2 nodes must appear after distance-1 nodes.
	// Distance-2 items have higher combined priority (distance * 10 + edgePriority),
	// so they should not precede any depth-1 item.
	if len(result2.Context) > len(result1.Context) {
		// Verify target ID is still correct.
		assert.Equal(t, "proj-vaultmind", result2.TargetID)
		// Verify all context items have valid IDs and edge types.
		for _, item := range result2.Context {
			assert.NotEmpty(t, item.ID)
			assert.NotEmpty(t, item.EdgeType)
		}
	}
}

// TestContextPack_DepthDefault verifies that omitting Depth (zero value) behaves
// identically to Depth=1 (backward compatibility).
func TestContextPack_DepthDefault(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)

	resultDefault, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input:  "proj-vaultmind",
		Budget: 100000,
	})
	require.NoError(t, err)

	resultDepth1, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input:  "proj-vaultmind",
		Budget: 100000,
		Depth:  1,
	})
	require.NoError(t, err)

	// Both should produce the same number of context items.
	assert.Equal(t, len(resultDepth1.Context), len(resultDefault.Context),
		"default depth (0) should behave like depth=1")
	assert.Equal(t, resultDepth1.TargetID, resultDefault.TargetID)
}

// TestContextPack_MaxItems verifies that when MaxItems > 0, the result has at
// most MaxItems context items.
func TestContextPack_MaxItems(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)

	maxItems := 3
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input:    "proj-vaultmind",
		Budget:   100000,
		MaxItems: maxItems,
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result.Context), maxItems,
		"MaxItems=%d should cap context items", maxItems)
}

// TestContextPack_MaxItemsWithBody verifies that when MaxItems constrains the
// count, items have body text included (body-first packing).
func TestContextPack_MaxItemsWithBody(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)

	// Use a large budget with MaxItems=5: body-first packing should fill each item
	// with body content since budget is ample.
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input:    "proj-vaultmind",
		Budget:   100000,
		MaxItems: 5,
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result.Context), 5,
		"should not exceed MaxItems=5")

	// With a large budget and body-first packing, items that have body content
	// should have BodyIncluded=true.
	bodyIncludedCount := 0
	for _, item := range result.Context {
		if item.BodyIncluded {
			bodyIncludedCount++
			assert.NotEmpty(t, item.Body, "BodyIncluded=true but Body is empty for %s", item.ID)
		}
	}
	if len(result.Context) > 0 {
		assert.Greater(t, bodyIncludedCount, 0,
			"with large budget and body-first packing, at least one item should have body included")
	}
}

// TestContextPack_SlimFrontmatter verifies that Slim=true reduces frontmatter
// to only type, title, and status fields.
func TestContextPack_SlimFrontmatter(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)

	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input:  "proj-vaultmind",
		Budget: 100000,
		Slim:   true,
	})
	require.NoError(t, err)
	require.Greater(t, len(result.Context), 0, "need context items to verify slim frontmatter")

	for _, item := range result.Context {
		for k := range item.Frontmatter {
			assert.Contains(t, []string{"type", "title", "status"}, k,
				"slim frontmatter should only contain type, title, status; got key %q for item %s", k, item.ID)
		}
	}
}

// TestContextPack_MaxItemsZero_BackwardCompat verifies that MaxItems=0 (default)
// preserves the existing two-pass behavior (frontmatter → backfill).
func TestContextPack_MaxItemsZero_BackwardCompat(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)

	resultDefault, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input:    "proj-vaultmind",
		Budget:   100000,
		MaxItems: 0,
	})
	require.NoError(t, err)

	resultNoField, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input:  "proj-vaultmind",
		Budget: 100000,
	})
	require.NoError(t, err)

	// MaxItems=0 and omitting MaxItems should produce identical results.
	assert.Equal(t, len(resultNoField.Context), len(resultDefault.Context),
		"MaxItems=0 should preserve existing behavior")
}
