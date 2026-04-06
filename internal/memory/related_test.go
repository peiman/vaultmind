package memory_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelated_Mixed(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Related(resolver, db, memory.RelatedConfig{Input: "proj-vaultmind", Mode: "mixed"})
	require.NoError(t, err)
	assert.Equal(t, "proj-vaultmind", result.TargetID)
	assert.Equal(t, "mixed", result.Mode)
	assert.Greater(t, len(result.Related), 0)
}

func TestRelated_ExplicitOnly(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Related(resolver, db, memory.RelatedConfig{Input: "proj-vaultmind", Mode: "explicit"})
	require.NoError(t, err)
	for _, r := range result.Related {
		assert.Equal(t, "high", r.Confidence)
	}
}

func TestRelated_InferredOnly(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Related(resolver, db, memory.RelatedConfig{Input: "proj-vaultmind", Mode: "inferred"})
	require.NoError(t, err)
	for _, r := range result.Related {
		assert.NotEqual(t, "high", r.Confidence)
	}
}

func TestRelated_HasMetadata(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Related(resolver, db, memory.RelatedConfig{Input: "proj-vaultmind", Mode: "mixed"})
	require.NoError(t, err)
	if len(result.Related) > 0 {
		assert.NotEmpty(t, result.Related[0].ID)
		assert.NotEmpty(t, result.Related[0].EdgeType)
	}
}
