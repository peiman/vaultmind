package graph_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraverse_StartNodeOnly(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)
	result, err := r.Traverse(graph.TraverseConfig{StartID: "proj-vaultmind", MaxDepth: 0, MinConfidence: "low", MaxNodes: 200})
	require.NoError(t, err)
	require.Len(t, result.Nodes, 1)
	assert.Equal(t, "proj-vaultmind", result.Nodes[0].ID)
	assert.Equal(t, 0, result.Nodes[0].Distance)
	assert.Nil(t, result.Nodes[0].EdgeFrom)
}

func TestTraverse_Depth1(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)
	result, err := r.Traverse(graph.TraverseConfig{StartID: "proj-vaultmind", MaxDepth: 1, MinConfidence: "low", MaxNodes: 200})
	require.NoError(t, err)
	assert.Greater(t, len(result.Nodes), 1)
	assert.Equal(t, 0, result.Nodes[0].Distance)
	for _, n := range result.Nodes[1:] {
		assert.Equal(t, 1, n.Distance)
		assert.NotNil(t, n.EdgeFrom)
	}
}

func TestTraverse_Depth2(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)
	// concept-act-r has fewer direct connections than proj-vaultmind, so depth-2
	// nodes exist (verified: depth-1=11, depth-2=29 with the current vault).
	result, err := r.Traverse(graph.TraverseConfig{StartID: "concept-act-r", MaxDepth: 2, MinConfidence: "low", MaxNodes: 200})
	require.NoError(t, err)
	hasDepth2 := false
	for _, n := range result.Nodes {
		if n.Distance == 2 {
			hasDepth2 = true
			break
		}
	}
	assert.True(t, hasDepth2)
}

func TestTraverse_MaxNodes(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)
	result, err := r.Traverse(graph.TraverseConfig{StartID: "proj-vaultmind", MaxDepth: 3, MinConfidence: "low", MaxNodes: 5})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result.Nodes), 5)
	assert.True(t, result.MaxNodesReached)
}

func TestTraverse_MinConfidenceHigh(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)
	highOnly, _ := r.Traverse(graph.TraverseConfig{StartID: "proj-vaultmind", MaxDepth: 1, MinConfidence: "high", MaxNodes: 200})
	allEdges, _ := r.Traverse(graph.TraverseConfig{StartID: "proj-vaultmind", MaxDepth: 1, MinConfidence: "low", MaxNodes: 200})
	assert.LessOrEqual(t, len(highOnly.Nodes), len(allEdges.Nodes))
}

func TestTraverse_NoCycles(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)
	result, err := r.Traverse(graph.TraverseConfig{StartID: "proj-vaultmind", MaxDepth: 10, MinConfidence: "low", MaxNodes: 200})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result.Nodes), 200)
	seen := make(map[string]bool)
	for _, n := range result.Nodes {
		assert.False(t, seen[n.ID], "duplicate: %s", n.ID)
		seen[n.ID] = true
	}
}

func TestTraverse_UnknownStartID(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)
	result, err := r.Traverse(graph.TraverseConfig{StartID: "nonexistent", MaxDepth: 1, MinConfidence: "low", MaxNodes: 200})
	require.NoError(t, err)
	assert.Len(t, result.Nodes, 1)
}

func TestMeetsConfidence(t *testing.T) {
	assert.True(t, graph.MeetsConfidence("high", "low"))
	assert.True(t, graph.MeetsConfidence("high", "high"))
	assert.True(t, graph.MeetsConfidence("medium", "low"))
	assert.True(t, graph.MeetsConfidence("medium", "medium"))
	assert.False(t, graph.MeetsConfidence("medium", "high"))
	assert.True(t, graph.MeetsConfidence("low", "low"))
	assert.False(t, graph.MeetsConfidence("low", "medium"))
	assert.False(t, graph.MeetsConfidence("low", "high"))
}
