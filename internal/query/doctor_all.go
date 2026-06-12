package query

// DoctorAllResult is the combined, single-envelope payload for `doctor --all`.
// It carries a workspace-level Rollup plus the per-vault DoctorResults. Each
// entry in Vaults is a full DoctorResult (which already carries its own
// vault_path), so consumers get one machine-readable value for the whole
// workspace — NOT one envelope per vault.
//
// Failed carries every discovered vault that could not be opened or diagnosed:
// rather than silently dropping a corrupt vault (which would hide it from the
// operator), we surface it here by path and reason. The field is omitempty so a
// clean workspace emits no `failed` key. Reporting failures in this combined
// envelope — instead of a per-vault error envelope — preserves the
// single-envelope contract.
type DoctorAllResult struct {
	Rollup DoctorRollup    `json:"rollup"`
	Vaults []*DoctorResult `json:"vaults"`
	Failed []FailedVault   `json:"failed,omitempty"`
}

// FailedVault names a discovered vault that could not be opened or diagnosed,
// together with the reason. It is the surfaced form of what `doctor --all` used
// to silently skip: an operator (human or JSON consumer) sees the path and the
// error instead of the vault vanishing.
type FailedVault struct {
	VaultPath string `json:"vault_path"`
	Error     string `json:"error"`
}

// DoctorRollup summarizes the health of every vault discovered under the root:
// how many vaults, the total note count, the combined error/warning counts, and
// the paths of vaults that have at least one error or warning. It is the
// top-of-output signal an operator scans before reading per-vault detail.
//
// The count fields are kept honest so a vault that failed to open can never hide
// behind a lower number: Discovered is the total found (Diagnosed + Failed),
// Diagnosed is how many produced a full report, and Failed is how many could not
// be opened. VaultCount is retained as an alias of Discovered for backward
// compatibility with existing JSON consumers.
type DoctorRollup struct {
	VaultCount    int `json:"vault_count"`
	Discovered    int `json:"discovered"`
	Diagnosed     int `json:"diagnosed"`
	Failed        int `json:"failed"`
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
// A vault counts as "having issues" when its ValidationSummary reports any
// errors or warnings; a nil ValidationSummary (a raw, un-validated result)
// contributes zero and is treated as clean. The failed slice (vaults that could
// not be opened) is folded into the honest count breakdown —
// Discovered = diagnosed + failed — so the rollup never under-reports the
// number of vaults found. Pure aggregation: no I/O, no mutation of inputs.
func BuildDoctorRollup(vaults []*DoctorResult, failed []FailedVault) DoctorRollup {
	diagnosed := len(vaults)
	discovered := diagnosed + len(failed)
	rollup := DoctorRollup{
		VaultCount:       discovered,
		Discovered:       discovered,
		Diagnosed:        diagnosed,
		Failed:           len(failed),
		VaultsWithIssues: []string{},
	}
	for _, v := range vaults {
		if v == nil {
			continue
		}
		rollup.TotalNotes += v.TotalFiles
		errs, warns := 0, 0
		if v.ValidationSummary != nil {
			errs = v.ValidationSummary.Errors
			warns = v.ValidationSummary.Warnings
		}
		rollup.TotalErrors += errs
		rollup.TotalWarnings += warns
		if errs > 0 || warns > 0 {
			rollup.VaultsWithIssues = append(rollup.VaultsWithIssues, v.VaultPath)
		}
	}
	return rollup
}
