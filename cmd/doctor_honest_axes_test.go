package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDoctor_JSONHasDistinctlyLabeledAxes asserts that the --json envelope
// exposes two explicitly-named fields for the two different warnings totals,
// so callers never encounter two unlabeled "warnings" values for the same run.
//
// The AX-design requirement: distinct labeled axes ("make hidden state visible").
// Before the fix, result.issues_summary and result.issues carried different
// unlabeled "warnings" counts with no name to distinguish them. After the fix:
//   - result.validation_summary.warnings = the raw validation aggregate
//   - The surfaced count lives only in result.issues.* (summed by SurfacedIssueCounts)
//
// This test uses buildValidationWarningVault (3 notes with unknown_type), which
// produces 3 raw validation warnings and 0 surfaced warnings.
func TestDoctor_JSONHasDistinctlyLabeledAxes(t *testing.T) {
	chdirToTemp(t)
	isolateMeshEnv(t)
	vaultDir := buildValidationWarningVault(t)

	jsonOut, _, err := runRootCmd(t, "doctor", "--vault", vaultDir, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			// The raw validation aggregate must be under a key that names it
			// unambiguously. Before the fix it was "issues_summary" — the same
			// word used colloquially for the surfaced count — which is the bug.
			ValidationSummary *struct {
				Errors   int `json:"errors"`
				Warnings int `json:"warnings"`
			} `json:"validation_summary"`
			// The old "issues_summary" key must be gone so there is no
			// ambiguity between the two totals.
			IssuesSummary *json.RawMessage `json:"issues_summary"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &env))

	// The raw validation aggregate must be present under the explicit key.
	require.NotNil(t, env.Result.ValidationSummary,
		"result.validation_summary must be present in --json so the raw aggregate is visible")
	require.Equal(t, 3, env.Result.ValidationSummary.Warnings,
		"result.validation_summary.warnings must hold the raw validation aggregate (3 unknown_type)")

	// The ambiguous old key must be absent from the output.
	require.Nil(t, env.Result.IssuesSummary,
		"result.issues_summary must be removed; the raw aggregate lives at result.validation_summary")
}

// TestDoctor_HumanReportNotesRawValidationGap asserts that when the raw
// validation aggregate is non-zero but the surfaced count is zero, the human
// report prints a one-line note so the operator knows more findings exist in
// --json. This prevents the hidden-state failure where the terminal output
// shows "0 warnings" while --json carries a non-zero aggregate.
func TestDoctor_HumanReportNotesRawValidationGap(t *testing.T) {
	chdirToTemp(t)
	isolateMeshEnv(t)
	vaultDir := buildValidationWarningVault(t)

	textOut, _, err := runRootCmd(t, "doctor", "--vault", vaultDir)
	require.NoError(t, err)
	text := textOut.String()

	// The human report must surface the gap so the operator is not surprised.
	// Exact phrasing is flexible; the key signal is the raw-validation count
	// and a pointer to --json.
	require.Contains(t, text, "--json",
		"human report must mention --json when there are raw validation findings not shown in the surfaced set")
}
