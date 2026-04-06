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
	assert.False(t, result.Context[0].BodyIncluded)
}

func TestContextPack_SmallBudget_Truncates(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{Input: "proj-vaultmind", Budget: 10})
	require.NoError(t, err)
	assert.True(t, result.Truncated)
	assert.LessOrEqual(t, result.UsedTokens, 10)
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
	assert.LessOrEqual(t, result.UsedTokens, 500)
}
