package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateTokens_Basic(t *testing.T) {
	assert.Equal(t, 1, EstimateTokens("hi"))
	assert.Equal(t, 3, EstimateTokens("hello world"))
	assert.Equal(t, 0, EstimateTokens(""))
}

func TestEstimateTokens_CeilingDivision(t *testing.T) {
	assert.Equal(t, 1, EstimateTokens("abc"))
	assert.Equal(t, 1, EstimateTokens("abcd"))
	assert.Equal(t, 2, EstimateTokens("abcde"))
}

// TestEdgePriority_AllBranches exercises all priority tiers of edgePriority,
// including the explicit_embed fix from C1.
func TestEdgePriority_AllBranches(t *testing.T) {
	// Tier 0: explicit_relation (highest priority)
	assert.Equal(t, 0, edgePriority("explicit_relation", "high"))
	assert.Equal(t, 0, edgePriority("explicit_relation", "low"))

	// Tier 1: explicit_link or explicit_embed
	assert.Equal(t, 1, edgePriority("explicit_link", "high"))
	assert.Equal(t, 1, edgePriority("explicit_embed", "high")) // C1 fix

	// Tier 2: medium confidence (other edge types)
	assert.Equal(t, 2, edgePriority("tag_overlap", "medium"))
	assert.Equal(t, 2, edgePriority("alias_mention", "medium"))

	// Tier 3: anything else (low/unknown confidence, unknown edge type)
	assert.Equal(t, 3, edgePriority("tag_overlap", "low"))
	assert.Equal(t, 3, edgePriority("unknown_type", ""))

	// Verify the old "embed" (non-explicit) is NOT tier 1
	assert.NotEqual(t, 1, edgePriority("embed", "high"))
}
