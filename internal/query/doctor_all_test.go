package query_test

import (
	"encoding/json"
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkVaultResult builds a minimal DoctorResult for rollup tests without touching
// a database — BuildDoctorRollup is pure aggregation over already-diagnosed
// vaults.
func mkVaultResult(path string, total, errs, warns, unresolved int) *query.DoctorResult {
	r := &query.DoctorResult{VaultPath: path, TotalFiles: total}
	r.IssuesSummary = &query.StatusIssuesSummary{Errors: errs, Warnings: warns}
	r.Issues.UnresolvedLinks = unresolved
	return r
}

func TestBuildDoctorRollup_AggregatesCountsAndTotals(t *testing.T) {
	vaults := []*query.DoctorResult{
		mkVaultResult("/a", 10, 0, 0, 0),
		mkVaultResult("/b", 5, 2, 1, 0),
		mkVaultResult("/c", 7, 0, 3, 0),
	}
	rollup := query.BuildDoctorRollup(vaults)

	assert.Equal(t, 3, rollup.VaultCount, "counts every diagnosed vault")
	assert.Equal(t, 22, rollup.TotalNotes, "sums TotalFiles across vaults")
	assert.Equal(t, 2, rollup.TotalErrors, "sums errors across vaults")
	assert.Equal(t, 4, rollup.TotalWarnings, "sums warnings across vaults")
}

func TestBuildDoctorRollup_ListsVaultsWithIssues(t *testing.T) {
	vaults := []*query.DoctorResult{
		mkVaultResult("/clean", 10, 0, 0, 0),
		mkVaultResult("/haserrors", 5, 2, 0, 0),
		mkVaultResult("/haswarnings", 7, 0, 3, 0),
	}
	rollup := query.BuildDoctorRollup(vaults)

	// A vault counts as "having issues" when it has errors OR warnings.
	assert.Equal(t, []string{"/haserrors", "/haswarnings"}, rollup.VaultsWithIssues,
		"only vaults with errors or warnings are listed, in input order")
}

func TestBuildDoctorRollup_CleanWorkspaceHasEmptyIssueList(t *testing.T) {
	vaults := []*query.DoctorResult{
		mkVaultResult("/a", 3, 0, 0, 0),
		mkVaultResult("/b", 4, 0, 0, 0),
	}
	rollup := query.BuildDoctorRollup(vaults)
	assert.Equal(t, 2, rollup.VaultCount)
	assert.Empty(t, rollup.VaultsWithIssues, "a clean workspace lists no problem vaults")
}

// A nil IssuesSummary (a raw, un-validated DoctorResult) must contribute zero
// errors/warnings and never panic — defensive parity with the human renderer.
func TestBuildDoctorRollup_NilIssuesSummaryCountsAsClean(t *testing.T) {
	clean := &query.DoctorResult{VaultPath: "/raw", TotalFiles: 9}
	rollup := query.BuildDoctorRollup([]*query.DoctorResult{clean})
	assert.Equal(t, 1, rollup.VaultCount)
	assert.Equal(t, 9, rollup.TotalNotes)
	assert.Equal(t, 0, rollup.TotalErrors)
	assert.Empty(t, rollup.VaultsWithIssues)
}

func TestBuildDoctorRollup_EmptyInput(t *testing.T) {
	rollup := query.BuildDoctorRollup(nil)
	assert.Equal(t, 0, rollup.VaultCount)
	assert.Equal(t, 0, rollup.TotalNotes)
	assert.Empty(t, rollup.VaultsWithIssues)
}

// The combined envelope shape: result.rollup is an object and result.vaults is
// an array of DoctorResults (each carrying its own vault_path). This is the
// single-envelope contract — NOT one envelope per vault.
func TestDoctorAllResult_JSONShape(t *testing.T) {
	all := query.DoctorAllResult{
		Rollup: query.BuildDoctorRollup([]*query.DoctorResult{
			mkVaultResult("/a", 2, 1, 0, 0),
		}),
		Vaults: []*query.DoctorResult{mkVaultResult("/a", 2, 1, 0, 0)},
	}
	raw, err := json.Marshal(all)
	require.NoError(t, err)

	var decoded struct {
		Rollup struct {
			VaultCount       int      `json:"vault_count"`
			TotalNotes       int      `json:"total_notes"`
			TotalErrors      int      `json:"total_errors"`
			TotalWarnings    int      `json:"total_warnings"`
			VaultsWithIssues []string `json:"vaults_with_issues"`
		} `json:"rollup"`
		Vaults []struct {
			VaultPath string `json:"vault_path"`
		} `json:"vaults"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, 1, decoded.Rollup.VaultCount)
	assert.Equal(t, 2, decoded.Rollup.TotalNotes)
	assert.Equal(t, 1, decoded.Rollup.TotalErrors)
	require.Len(t, decoded.Vaults, 1)
	assert.Equal(t, "/a", decoded.Vaults[0].VaultPath, "each vault entry carries its own vault_path")
}
