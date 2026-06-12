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
// vaults. The errs/warns params drive the SURFACED axis (the rollup headline
// now sums ResultSurfacedIssueCounts, matching each per-vault report), so errs
// maps to a surfaced error bucket (DuplicateIDs) and warns to a surfaced
// warning bucket (BrokenReferences). unresolved adds further surfaced warnings
// via UnresolvedLinks. ValidationSummary (the RAW axis) is left nil here; tests
// that exercise the raw-vs-surfaced distinction set it explicitly.
func mkVaultResult(path string, total, errs, warns, unresolved int) *query.DoctorResult {
	r := &query.DoctorResult{VaultPath: path, TotalFiles: total}
	r.Issues.DuplicateIDs = errs
	r.Issues.BrokenReferences = warns
	r.Issues.UnresolvedLinks = unresolved
	return r
}

func TestBuildDoctorRollup_AggregatesCountsAndTotals(t *testing.T) {
	vaults := []*query.DoctorResult{
		mkVaultResult("/a", 10, 0, 0, 0),
		mkVaultResult("/b", 5, 2, 1, 0),
		mkVaultResult("/c", 7, 0, 3, 0),
	}
	rollup := query.BuildDoctorRollup(vaults, nil)

	assert.Equal(t, 3, rollup.VaultCount, "counts every diagnosed vault")
	assert.Equal(t, 3, rollup.Discovered, "no failures: discovered == diagnosed")
	assert.Equal(t, 3, rollup.Diagnosed, "all three diagnosed")
	assert.Equal(t, 0, rollup.Failed, "no vaults failed to open")
	assert.Equal(t, 22, rollup.TotalNotes, "sums TotalFiles across vaults")
	assert.Equal(t, 2, rollup.TotalErrors, "sums errors across vaults")
	assert.Equal(t, 4, rollup.TotalWarnings, "sums warnings across vaults")
}

// When a vault fails to open, the rollup's counts must stay honest: Discovered
// reflects the total found (diagnosed + failed), never hiding a broken vault.
func TestBuildDoctorRollup_CountsFailedVaults(t *testing.T) {
	vaults := []*query.DoctorResult{
		mkVaultResult("/a", 10, 0, 0, 0),
		mkVaultResult("/b", 5, 2, 0, 0),
	}
	failed := []query.FailedVault{{VaultPath: "/broken", Error: "bad config"}}
	rollup := query.BuildDoctorRollup(vaults, failed)

	assert.Equal(t, 2, rollup.Diagnosed, "two vaults diagnosed")
	assert.Equal(t, 1, rollup.Failed, "one vault failed to open")
	assert.Equal(t, 3, rollup.Discovered, "discovered = diagnosed + failed; never hides the broken one")
}

func TestBuildDoctorRollup_ListsVaultsWithIssues(t *testing.T) {
	vaults := []*query.DoctorResult{
		mkVaultResult("/clean", 10, 0, 0, 0),
		mkVaultResult("/haserrors", 5, 2, 0, 0),
		mkVaultResult("/haswarnings", 7, 0, 3, 0),
	}
	rollup := query.BuildDoctorRollup(vaults, nil)

	// A vault counts as "having issues" when it has errors OR warnings.
	assert.Equal(t, []string{"/haserrors", "/haswarnings"}, rollup.VaultsWithIssues,
		"only vaults with errors or warnings are listed, in input order")
}

func TestBuildDoctorRollup_CleanWorkspaceHasEmptyIssueList(t *testing.T) {
	vaults := []*query.DoctorResult{
		mkVaultResult("/a", 3, 0, 0, 0),
		mkVaultResult("/b", 4, 0, 0, 0),
	}
	rollup := query.BuildDoctorRollup(vaults, nil)
	assert.Equal(t, 2, rollup.VaultCount)
	assert.Empty(t, rollup.VaultsWithIssues, "a clean workspace lists no problem vaults")
}

// A nil ValidationSummary (a raw, un-validated DoctorResult) must contribute
// zero errors/warnings and never panic — defensive parity with the human
// renderer.
func TestBuildDoctorRollup_NilValidationSummaryCountsAsClean(t *testing.T) {
	clean := &query.DoctorResult{VaultPath: "/raw", TotalFiles: 9}
	rollup := query.BuildDoctorRollup([]*query.DoctorResult{clean}, nil)
	assert.Equal(t, 1, rollup.VaultCount)
	assert.Equal(t, 9, rollup.TotalNotes)
	assert.Equal(t, 0, rollup.TotalErrors)
	assert.Empty(t, rollup.VaultsWithIssues)
}

func TestBuildDoctorRollup_EmptyInput(t *testing.T) {
	rollup := query.BuildDoctorRollup(nil, nil)
	assert.Equal(t, 0, rollup.VaultCount)
	assert.Equal(t, 0, rollup.TotalNotes)
	assert.Empty(t, rollup.VaultsWithIssues)
}

// The headline TotalErrors/TotalWarnings must report the SURFACED set
// (query.ResultSurfacedIssueCounts) — the same axis each per-vault report
// prints — so `doctor --all` totals reconcile with summing the per-vault
// reports. The RAW schema-validation aggregate (ValidationSummary) is a
// DIFFERENT axis: it counts findings (unknown_type / invalid_status) the text
// report never surfaces as per-item lines. Collapsing both under the same
// label is the two-unlabeled-axes bug PR #31 fixed for the single-vault case;
// this guards the remaining rollup instance.
func TestBuildDoctorRollup_TotalsAreSurfacedNotRawValidation(t *testing.T) {
	// Vault A: surfaced advisories only (Obsidian-incompatible + unresolved
	// links), zero raw validation findings. Surfaced warnings = 2 + 1 = 3.
	a := &query.DoctorResult{VaultPath: "/a", TotalFiles: 12}
	a.Issues.ObsidianIncompatibleLinks = 2
	a.Issues.UnresolvedLinks = 1

	// Vault B: a surfaced broken reference (warning) PLUS several raw-only
	// unknown_type findings that live in ValidationSummary but are NEVER
	// surfaced as text lines. Surfaced warnings = 1 (BrokenReferences). The raw
	// aggregate is far larger (1 surfaced broken_reference + 57 unknown_type).
	b := &query.DoctorResult{VaultPath: "/b", TotalFiles: 30}
	b.Issues.BrokenReferences = 1
	b.ValidationSummary = &query.StatusIssuesSummary{Errors: 0, Warnings: 58}

	vaults := []*query.DoctorResult{a, b}
	rollup := query.BuildDoctorRollup(vaults, nil)

	// Headline totals == sum of each vault's SURFACED counts (NOT the raw sum).
	var wantErrs, wantWarns int
	for _, v := range vaults {
		e, w := query.ResultSurfacedIssueCounts(v)
		wantErrs += e
		wantWarns += w
	}
	assert.Equal(t, wantErrs, rollup.TotalErrors,
		"TotalErrors sums the surfaced set, matching per-vault reports")
	assert.Equal(t, wantWarns, rollup.TotalWarnings,
		"TotalWarnings sums the surfaced set, matching per-vault reports")
	// Concretely: surfaced warnings are 3 (vault A) + 1 (vault B) = 4, NOT the
	// raw 3 + 58 = 61 the old ValidationSummary-based code reported.
	assert.Equal(t, 4, rollup.TotalWarnings, "surfaced, not raw, warning total")
	assert.Equal(t, 0, rollup.TotalErrors)

	// The raw validation aggregate is preserved under its own distinctly-labeled
	// field — sum of each ValidationSummary.Errors+Warnings (nil ⇒ 0).
	assert.Equal(t, 58, rollup.TotalRawValidationFindings,
		"raw schema-validation aggregate is kept, not discarded, under its own field")

	// "Has issues" keys on the surfaced count too: both vaults surface warnings.
	assert.Equal(t, []string{"/a", "/b"}, rollup.VaultsWithIssues)
}

// RawValidationGap reports the raw-only findings (those NOT surfaced as text
// lines) across the workspace: TotalRawValidationFindings minus the surfaced
// validation findings (MissingRequiredFields + BrokenReferences). It must NOT
// subtract non-validation surfaced items, so it never under-reports the gap.
func TestBuildDoctorRollup_RawValidationGap(t *testing.T) {
	// Vault has: 1 surfaced broken_reference + 1 surfaced missing_required_field
	// (both ARE validation findings) PLUS surfaced obsidian links (NON-validation)
	// PLUS a raw aggregate of 60 (= 2 surfaced validation + 58 raw-only).
	v := &query.DoctorResult{VaultPath: "/v", TotalFiles: 40}
	v.Issues.BrokenReferences = 1
	v.Issues.MissingRequiredFields = 1
	v.Issues.ObsidianIncompatibleLinks = 5 // non-validation surfaced noise
	v.ValidationSummary = &query.StatusIssuesSummary{Errors: 1, Warnings: 59}

	rollup := query.BuildDoctorRollup([]*query.DoctorResult{v}, nil)

	assert.Equal(t, 60, rollup.TotalRawValidationFindings)
	// Gap subtracts ONLY the surfaced validation findings (1+1=2), not the
	// obsidian links — so 60 - 2 = 58 raw-only findings.
	assert.Equal(t, 58, rollup.RawValidationGap(),
		"gap subtracts surfaced validation findings only, never the full headline")
}

// A vault whose ONLY findings are raw-only validation findings (unknown_type)
// surfaces nothing actionable, so it must NOT count as "has issues" and must
// NOT inflate the headline warnings — but its raw aggregate is still carried in
// TotalRawValidationFindings so the information is not lost.
func TestBuildDoctorRollup_RawOnlyVaultIsNotASurfacedIssue(t *testing.T) {
	rawOnly := &query.DoctorResult{VaultPath: "/raw-only", TotalFiles: 8}
	rawOnly.ValidationSummary = &query.StatusIssuesSummary{Errors: 0, Warnings: 57}

	rollup := query.BuildDoctorRollup([]*query.DoctorResult{rawOnly}, nil)

	assert.Equal(t, 0, rollup.TotalWarnings, "raw-only findings are not surfaced warnings")
	assert.Equal(t, 0, rollup.TotalErrors)
	assert.Equal(t, 57, rollup.TotalRawValidationFindings, "raw aggregate still carried")
	assert.Empty(t, rollup.VaultsWithIssues,
		"a vault with no surfaced issues is not listed even if it has raw findings")
}

// The combined envelope shape: result.rollup is an object and result.vaults is
// an array of DoctorResults (each carrying its own vault_path). This is the
// single-envelope contract — NOT one envelope per vault.
func TestDoctorAllResult_JSONShape(t *testing.T) {
	all := query.DoctorAllResult{
		Rollup: query.BuildDoctorRollup([]*query.DoctorResult{
			mkVaultResult("/a", 2, 1, 0, 0),
		}, nil),
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

// Failed vaults are carried in the single envelope under result.failed[], each
// naming its path and the reason it could not be opened — surfaced, not dropped.
// A workspace with no failures omits the field entirely (omitempty).
func TestDoctorAllResult_FailedVaultsJSONShape(t *testing.T) {
	withFailures := query.DoctorAllResult{
		Rollup: query.BuildDoctorRollup(nil, []query.FailedVault{
			{VaultPath: "/broken", Error: "loading config: invalid yaml"},
		}),
		Vaults: nil,
		Failed: []query.FailedVault{{VaultPath: "/broken", Error: "loading config: invalid yaml"}},
	}
	raw, err := json.Marshal(withFailures)
	require.NoError(t, err)

	var decoded struct {
		Rollup struct {
			Discovered int `json:"discovered"`
			Diagnosed  int `json:"diagnosed"`
			Failed     int `json:"failed"`
		} `json:"rollup"`
		Failed []struct {
			VaultPath string `json:"vault_path"`
			Error     string `json:"error"`
		} `json:"failed"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Len(t, decoded.Failed, 1, "the failed vault is surfaced under result.failed[]")
	assert.Equal(t, "/broken", decoded.Failed[0].VaultPath)
	assert.Contains(t, decoded.Failed[0].Error, "invalid yaml", "the failure reason is carried")
	assert.Equal(t, 1, decoded.Rollup.Discovered)
	assert.Equal(t, 0, decoded.Rollup.Diagnosed)
	assert.Equal(t, 1, decoded.Rollup.Failed)

	// A clean workspace omits the failed field entirely.
	clean := query.DoctorAllResult{
		Rollup: query.BuildDoctorRollup([]*query.DoctorResult{mkVaultResult("/a", 1, 0, 0, 0)}, nil),
		Vaults: []*query.DoctorResult{mkVaultResult("/a", 1, 0, 0, 0)},
	}
	cleanRaw, err := json.Marshal(clean)
	require.NoError(t, err)
	assert.NotContains(t, string(cleanRaw), `"failed":[`, "no failed array when nothing failed (omitempty)")
}
