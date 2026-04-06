package query_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNeighbors_Basic(t *testing.T) {
	db := buildIndexedDB(t)
	resolver := graph.NewResolver(db)
	result, err := query.Neighbors(resolver, "proj-vaultmind", 1, "low", 200)
	require.NoError(t, err)
	assert.Equal(t, "proj-vaultmind", result.StartID)
	assert.Greater(t, len(result.Nodes), 1)
}

func TestNeighbors_DepthZero(t *testing.T) {
	db := buildIndexedDB(t)
	resolver := graph.NewResolver(db)
	result, err := query.Neighbors(resolver, "proj-vaultmind", 0, "low", 200)
	require.NoError(t, err)
	assert.Len(t, result.Nodes, 1)
}

func TestNeighbors_MaxNodes(t *testing.T) {
	db := buildIndexedDB(t)
	resolver := graph.NewResolver(db)
	result, err := query.Neighbors(resolver, "proj-vaultmind", 3, "low", 3)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result.Nodes), 3)
	assert.True(t, result.MaxNodesReached)
}

func TestNeighbors_JSONShape(t *testing.T) {
	db := buildIndexedDB(t)
	resolver := graph.NewResolver(db)
	result, err := query.Neighbors(resolver, "proj-vaultmind", 1, "high", 200)
	require.NoError(t, err)
	assert.Nil(t, result.Nodes[0].EdgeFrom)
	if len(result.Nodes) > 1 {
		assert.NotNil(t, result.Nodes[1].EdgeFrom)
	}
}
