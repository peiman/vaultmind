package memory

import (
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestEnrichAndSortCandidates_WithActivation(t *testing.T) {
	candidates := []contextCandidate{
		{noteID: "low-act", edgeType: "explicit_link", confidence: "high", priority: 1},
		{noteID: "high-act", edgeType: "explicit_link", confidence: "high", priority: 1},
	}
	activation := map[string]float64{"low-act": 0.5, "high-act": 2.0}
	loader := func(id string) (*index.FullNote, error) {
		return &index.FullNote{ID: id, Frontmatter: map[string]any{}}, nil
	}

	err := enrichAndSortCandidates(loader, candidates, activation)
	require.NoError(t, err)
	assert.Equal(t, "high-act", candidates[0].noteID)
	assert.Equal(t, "low-act", candidates[1].noteID)
}

func TestEnrichAndSortCandidates_PriorityBeatsActivation(t *testing.T) {
	candidates := []contextCandidate{
		{noteID: "high-act-low-pri", edgeType: "tag_overlap", confidence: "low", priority: 3},
		{noteID: "low-act-high-pri", edgeType: "explicit_relation", confidence: "high", priority: 0},
	}
	activation := map[string]float64{"high-act-low-pri": 5.0, "low-act-high-pri": 0.1}
	loader := func(id string) (*index.FullNote, error) {
		return &index.FullNote{ID: id, Frontmatter: map[string]any{}}, nil
	}

	err := enrichAndSortCandidates(loader, candidates, activation)
	require.NoError(t, err)
	assert.Equal(t, "low-act-high-pri", candidates[0].noteID)
}

func TestEnrichAndSortCandidates_NilActivation(t *testing.T) {
	candidates := []contextCandidate{
		{noteID: "b", edgeType: "explicit_link", confidence: "high", priority: 1},
		{noteID: "a", edgeType: "explicit_link", confidence: "high", priority: 1},
	}
	loader := func(id string) (*index.FullNote, error) {
		fm := map[string]any{}
		if id == "a" {
			fm["updated"] = "2026-04-09"
		} else {
			fm["updated"] = "2026-04-08"
		}
		return &index.FullNote{ID: id, Frontmatter: fm}, nil
	}

	err := enrichAndSortCandidates(loader, candidates, nil)
	require.NoError(t, err)
	// Falls back to updated date sort: a (newer) before b
	assert.Equal(t, "a", candidates[0].noteID)
}
