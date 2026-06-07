package query

// DoctorAllResult is the combined, single-envelope payload for `doctor --all`.
// It carries a workspace-level Rollup plus the per-vault DoctorResults. Each
// entry in Vaults is a full DoctorResult (which already carries its own
// vault_path), so consumers get one machine-readable value for the whole
// workspace — NOT one envelope per vault.
type DoctorAllResult struct {
	Rollup DoctorRollup    `json:"rollup"`
	Vaults []*DoctorResult `json:"vaults"`
}

// DoctorRollup summarizes the health of every vault discovered under the root:
// how many vaults, the total note count, the combined error/warning counts, and
// the paths of vaults that have at least one error or warning. It is the
// top-of-output signal an operator scans before reading per-vault detail.
type DoctorRollup struct {
	VaultCount    int `json:"vault_count"`
	TotalNotes    int `json:"total_notes"`
	TotalErrors   int `json:"total_errors"`
	TotalWarnings int `json:"total_warnings"`
	// VaultsWithIssues lists the paths of vaults with errors or warnings, in the
	// order the vaults were diagnosed (already deterministic — discovery sorts
	// them). Always non-nil so JSON consumers see [] for a clean workspace
	// rather than null.
	VaultsWithIssues []string `json:"vaults_with_issues"`
}

// BuildDoctorRollup aggregates per-vault DoctorResults into a workspace rollup.
// A vault counts as "having issues" when its IssuesSummary reports any errors or
// warnings; a nil IssuesSummary (a raw, un-validated result) contributes zero
// and is treated as clean. Pure aggregation: no I/O, no mutation of inputs.
func BuildDoctorRollup(vaults []*DoctorResult) DoctorRollup {
	rollup := DoctorRollup{
		VaultCount:       len(vaults),
		VaultsWithIssues: []string{},
	}
	for _, v := range vaults {
		if v == nil {
			continue
		}
		rollup.TotalNotes += v.TotalFiles
		errs, warns := 0, 0
		if v.IssuesSummary != nil {
			errs = v.IssuesSummary.Errors
			warns = v.IssuesSummary.Warnings
		}
		rollup.TotalErrors += errs
		rollup.TotalWarnings += warns
		if errs > 0 || warns > 0 {
			rollup.VaultsWithIssues = append(rollup.VaultsWithIssues, v.VaultPath)
		}
	}
	return rollup
}
