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
	VaultCount int `json:"vault_count"`
	Discovered int `json:"discovered"`
	Diagnosed  int `json:"diagnosed"`
	Failed     int `json:"failed"`
	TotalNotes int `json:"total_notes"`
	// TotalErrors/TotalWarnings are the SURFACED set — the sum of each vault's
	// query.ResultSurfacedIssueCounts, the same axis each per-vault report
	// prints — so the rollup totals reconcile with summing the per-vault
	// reports. They deliberately do NOT count the raw schema-validation
	// aggregate (see TotalRawValidationFindings), which is a different axis.
	TotalErrors   int `json:"total_errors"`
	TotalWarnings int `json:"total_warnings"`
	// TotalRawValidationFindings is the RAW schema-validation aggregate summed
	// across vaults: each vault's ValidationSummary.Errors+Warnings (nil ⇒ 0).
	// It is a DIFFERENT axis from TotalErrors/TotalWarnings — it includes
	// unknown_type / invalid_status findings the per-vault text report never
	// surfaces as per-item lines. Kept under its own distinctly-labeled key so
	// the two axes are explicit rather than collapsed, mirroring how the
	// single-vault envelope keeps validation_summary alongside the surfaced set.
	TotalRawValidationFindings int `json:"total_raw_validation_findings"`
	// surfacedValidationFindings is the sum across vaults of the raw-validation
	// findings that ARE surfaced as text lines (MissingRequiredFields +
	// BrokenReferences). It is NOT serialized — it exists only so the human
	// renderer can compute the raw-only gap exactly as the single-vault path
	// does (gap = TotalRawValidationFindings - surfacedValidationFindings),
	// rather than subtracting the full surfaced headline (which folds in
	// non-validation items and would under-report the gap).
	surfacedValidationFindings int
	// VaultsWithIssues lists the paths of vaults with errors or warnings, in the
	// order the vaults were diagnosed (already deterministic — discovery sorts
	// them). Always non-nil so JSON consumers see [] for a clean workspace
	// rather than null.
	VaultsWithIssues []string `json:"vaults_with_issues"`
}

// BuildDoctorRollup aggregates per-vault DoctorResults into a workspace rollup.
// The headline TotalErrors/TotalWarnings sum each vault's SURFACED counts
// (ResultSurfacedIssueCounts — the same axis every per-vault report prints), so
// `doctor --all` totals reconcile with summing the per-vault reports. A vault
// counts as "having issues" iff its surfaced errors+warnings > 0, keyed on the
// same axis. The RAW schema-validation aggregate is not discarded: it is summed
// separately into TotalRawValidationFindings (each vault's
// ValidationSummary.Errors+Warnings; a nil ValidationSummary — a raw,
// un-validated result — contributes zero and never panics). The failed slice
// (vaults that could not be opened) is folded into the honest count breakdown —
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
		// Headline totals: the SURFACED set, matching each per-vault report.
		errs, warns := ResultSurfacedIssueCounts(v)
		rollup.TotalErrors += errs
		rollup.TotalWarnings += warns
		// Raw schema-validation aggregate: a separate, distinctly-labeled axis.
		// nil ValidationSummary (un-validated result) contributes zero.
		if v.ValidationSummary != nil {
			rollup.TotalRawValidationFindings += v.ValidationSummary.Errors + v.ValidationSummary.Warnings
		}
		// The validation findings that ARE surfaced as text lines — so the
		// raw-only gap can be computed exactly (see RawValidationGap).
		rollup.surfacedValidationFindings += v.Issues.MissingRequiredFields + v.Issues.BrokenReferences
		if errs > 0 || warns > 0 {
			rollup.VaultsWithIssues = append(rollup.VaultsWithIssues, v.VaultPath)
		}
	}
	return rollup
}

// RawValidationGap reports how many RAW schema-validation findings are NOT
// surfaced as per-item text lines across the workspace — the workspace-level
// analogue of the single-vault gap line. It is TotalRawValidationFindings minus
// the surfaced validation findings (MissingRequiredFields + BrokenReferences),
// never the full surfaced headline (which folds in non-validation items and
// would under-report the gap). A non-positive result means nothing is hidden.
func (r DoctorRollup) RawValidationGap() int {
	return r.TotalRawValidationFindings - r.surfacedValidationFindings
}
