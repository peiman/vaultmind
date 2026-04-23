package memory_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testVaultPath = "../../vaultmind-vault"

func buildTestDB(t *testing.T) *index.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db := testvault.OpenSharedDB(t, testVaultPath, dbPath)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestRecall_Basic(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Recall(resolver, db, memory.RecallConfig{
		Input: "proj-vaultmind", Depth: 1, MinConfidence: "high", MaxNodes: 200,
	})
	require.NoError(t, err)
	assert.Equal(t, "proj-vaultmind", result.TargetID)
	assert.Greater(t, len(result.Nodes), 1)
	assert.Equal(t, 0, result.Nodes[0].Distance)
	assert.NotEmpty(t, result.Nodes[0].Type)
	assert.NotEmpty(t, result.Nodes[0].Title)
}

func TestRecall_HasFrontmatter(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Recall(resolver, db, memory.RecallConfig{
		Input: "proj-vaultmind", Depth: 1, MinConfidence: "high", MaxNodes: 200,
	})
	require.NoError(t, err)
	assert.NotNil(t, result.Nodes[0].Frontmatter)
}

func TestRecall_HasEdges(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Recall(resolver, db, memory.RecallConfig{
		Input: "proj-vaultmind", Depth: 1, MinConfidence: "high", MaxNodes: 200,
	})
	require.NoError(t, err)
	assert.Greater(t, len(result.Edges), 0)
	assert.NotEmpty(t, result.Edges[0].SourceID)
	assert.NotEmpty(t, result.Edges[0].TargetID)
}

func TestRecall_MaxNodes(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Recall(resolver, db, memory.RecallConfig{
		Input: "proj-vaultmind", Depth: 3, MinConfidence: "low", MaxNodes: 5,
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result.Nodes), 5)
	assert.True(t, result.MaxNodesReached)
}

func TestRecall_DepthZero(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Recall(resolver, db, memory.RecallConfig{
		Input: "proj-vaultmind", Depth: 0, MinConfidence: "low", MaxNodes: 200,
	})
	require.NoError(t, err)
	assert.Len(t, result.Nodes, 1)
	assert.Empty(t, result.Edges)
}

// TestRecall_UnresolvableInput verifies an error is returned when the input
// cannot be resolved to a known note.
func TestRecall_UnresolvableInput(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Recall(resolver, db, memory.RecallConfig{
		Input: "does-not-exist-xyz", Depth: 1, MinConfidence: "low", MaxNodes: 200,
	})
	assert.Error(t, err)
	assert.Nil(t, result)
}
