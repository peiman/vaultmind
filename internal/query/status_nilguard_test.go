package query_test

// status_nilguard_test.go — behavior tests for the nil-cfg/nil-reg guard
// branches in CollectTypeBreakdown and SummarizeValidationIssues.
//
// These guards are the single-source-of-truth shared helpers surfaced by both
// `vault status` (cold-start) and `doctor` (health hub). The nil-guard
// branches were uncovered at 64.5–75% (patch-coverage gap run 2026-06-07).
// Covering them also verifies that callers can safely range over the result
// without nil-dereference panics.

import (
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CollectTypeBreakdown with a nil vault.Config must return an empty non-nil map.
// Callers that range over the result must not panic even when no config is
// available (e.g., vault with no .vaultmind/config.yaml).
func TestCollectTypeBreakdown_NilConfigReturnsEmptyMap(t *testing.T) {
	db := buildIndexedDB(t)

	types, err := query.CollectTypeBreakdown(db, nil)
	require.NoError(t, err, "nil cfg must not cause an error")
	assert.NotNil(t, types, "result must be a non-nil map so callers can range safely")
	assert.Empty(t, types, "no types are defined when cfg is nil")
}

// CollectTypeBreakdown with a valid config populates per-type counts and their
// schema metadata (required fields + valid statuses). This is the happy path
// exercised transitively by TestCollectTypeBreakdown_SharedHelper; we add an
// explicit nil-statuses check here because the helper normalises nil → empty
// slice to prevent callers from receiving a nil slice for an optional field.
func TestCollectTypeBreakdown_NilStatusesNormalisedToEmpty(t *testing.T) {
	db := buildIndexedDB(t)

	// The test vault has a "concept" type with no statuses defined.
	// CollectTypeBreakdown must return []string{} not nil for the Statuses field.
	// (A nil slice serialises as JSON null; callers expect [].)
	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	types, err := query.CollectTypeBreakdown(db, cfg)
	require.NoError(t, err)

	for typeName, info := range types {
		assert.NotNil(t, info.Statuses,
			"Statuses for type %q must be a non-nil slice, never JSON null", typeName)
	}
}

// SummarizeValidationIssues with a nil registry must return a zero-value
// summary without error. The nil guard exists so both vault status and doctor
// can call this helper before a registry has been loaded.
func TestSummarizeValidationIssues_NilRegistryReturnsZeroSummary(t *testing.T) {
	db := buildIndexedDB(t)

	summary, err := query.SummarizeValidationIssues(db, nil)
	require.NoError(t, err, "nil registry must not cause an error")
	assert.Equal(t, 0, summary.Errors, "nil registry must yield 0 errors")
	assert.Equal(t, 0, summary.Warnings, "nil registry must yield 0 warnings")
}

// SummarizeValidationIssues counts errors and warnings correctly on a clean
// vault. A vault with no schema violations must report 0 errors and 0 warnings.
// This is the SSOT contract: both the status path and the doctor path call the
// same function and must agree.
func TestSummarizeValidationIssues_CleanVaultHasZeroIssues(t *testing.T) {
	db := buildIndexedDB(t)
	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)
	reg := schema.NewRegistry(cfg.Types)

	summary, err := query.SummarizeValidationIssues(db, reg)
	require.NoError(t, err)
	assert.Equal(t, 0, summary.Errors,
		"clean fixture vault must have no schema errors")
}

// VaultStatus with nil cfg still returns a valid result (no panic, no error):
// the types map will be empty because there is no type registry to draw from.
func TestVaultStatus_NilCfgReturnsValidResult(t *testing.T) {
	db := buildIndexedDB(t)

	result, err := query.VaultStatus(db, testVaultPath, nil, nil)
	require.NoError(t, err, "nil cfg/reg must not cause VaultStatus to fail")
	assert.Equal(t, testVaultPath, result.VaultPath)
	assert.Greater(t, result.TotalFiles, 0,
		"VaultStatus must still count notes even without cfg")
	assert.NotNil(t, result.Types, "Types must be a non-nil map even when cfg is nil")
	assert.Empty(t, result.Types, "no types can be listed when cfg is nil")
	assert.Equal(t, 0, result.IssuesSummary.Errors,
		"nil reg yields zero issues — not an error")
}
