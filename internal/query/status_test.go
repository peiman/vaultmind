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

// CollectTypeBreakdown is the SSOT helper now shared between vault status and
// the doctor health hub: it must return per-type counts plus the required
// fields and valid statuses straight from the registry, with the same shape
// VaultStatus.Types carries.
func TestCollectTypeBreakdown_SharedHelper(t *testing.T) {
	db := buildIndexedDB(t)
	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	types, err := query.CollectTypeBreakdown(db, cfg)
	require.NoError(t, err)
	require.Contains(t, types, "concept")
	require.Contains(t, types, "project")
	assert.Greater(t, types["concept"].Count, 0)
	assert.Contains(t, types["concept"].Required, "title")

	// The doctor path and the status path must agree byte-for-byte: same DB,
	// same cfg, identical breakdown. This is the SSOT contract.
	reg := schema.NewRegistry(cfg.Types)
	status, err := query.VaultStatus(db, testVaultPath, cfg, reg)
	require.NoError(t, err)
	assert.Equal(t, status.Types, types,
		"the shared breakdown helper must produce exactly what VaultStatus reports")
}

// SummarizeValidationIssues is the SSOT helper for the errors/warnings rollup.
// It must match the count VaultStatus reports for the same DB + registry.
func TestSummarizeValidationIssues_SharedHelper(t *testing.T) {
	db := buildIndexedDB(t)
	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)
	reg := schema.NewRegistry(cfg.Types)

	summary, err := query.SummarizeValidationIssues(db, reg)
	require.NoError(t, err)

	status, err := query.VaultStatus(db, testVaultPath, cfg, reg)
	require.NoError(t, err)
	assert.Equal(t, status.IssuesSummary, summary,
		"the shared rollup helper must produce exactly what VaultStatus reports")
}
