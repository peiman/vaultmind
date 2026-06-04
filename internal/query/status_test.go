package query_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultStatus_ReturnsAllSections(t *testing.T) {
	db := buildIndexedDB(t)
	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)
	reg := schema.NewRegistry(cfg.Types)

	result, err := query.VaultStatus(db, testVaultPath, cfg, reg)
	require.NoError(t, err)

	assert.Equal(t, testVaultPath, result.VaultPath)
	assert.Greater(t, result.TotalFiles, 0)
	assert.Greater(t, result.DomainNotes, 0)
	assert.GreaterOrEqual(t, result.UnstructuredNotes, 0)
	assert.NotEmpty(t, result.IndexStatus)
	assert.NotEmpty(t, result.Types)
	assert.Contains(t, result.Types, "concept")
	assert.Contains(t, result.Types, "project")
}

func TestVaultStatus_TypesIncludeCount(t *testing.T) {
	db := buildIndexedDB(t)
	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)
	reg := schema.NewRegistry(cfg.Types)

	result, err := query.VaultStatus(db, testVaultPath, cfg, reg)
	require.NoError(t, err)

	conceptInfo := result.Types["concept"]
	assert.Greater(t, conceptInfo.Count, 0)
	assert.Contains(t, conceptInfo.Required, "title")
}

func TestVaultStatus_IncludesIssueSummary(t *testing.T) {
	db := buildIndexedDB(t)
	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)
	reg := schema.NewRegistry(cfg.Types)

	result, err := query.VaultStatus(db, testVaultPath, cfg, reg)
	require.NoError(t, err)

	// Clean vault should have 0 errors
	assert.Equal(t, 0, result.IssuesSummary.Errors)
}
