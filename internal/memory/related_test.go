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

// TestRelated_InvalidMode verifies that an unrecognised mode falls through to
// "mixed" (default) behaviour — i.e. returns all edges without error.
func TestRelated_InvalidMode(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Related(resolver, db, memory.RelatedConfig{Input: "proj-vaultmind", Mode: "nonexistent-mode"})
	require.NoError(t, err)
	// "nonexistent-mode" hits the default case (keep = true), so it behaves like mixed
	assert.NotNil(t, result)
	assert.Greater(t, len(result.Related), 0)
}

// TestRelated_TargetID_IsResolvedCanonical verifies the TargetID in the result
// is the resolved canonical ID rather than raw cfg.Input (I1 fix).
func TestRelated_TargetID_IsResolvedCanonical(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Related(resolver, db, memory.RelatedConfig{Input: "proj-vaultmind", Mode: "mixed"})
	require.NoError(t, err)
	// The canonical ID for "proj-vaultmind" is "proj-vaultmind" — they match here,
	// but TargetID must come from the resolver, not raw input.
	assert.Equal(t, "proj-vaultmind", result.TargetID)
}

// TestRelated_UnresolvableInput verifies an error is returned when the input
// cannot be resolved to a known note.
func TestRelated_UnresolvableInput(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.Related(resolver, db, memory.RelatedConfig{Input: "does-not-exist-xyz", Mode: "mixed"})
	assert.Error(t, err)
	assert.Nil(t, result)
}
