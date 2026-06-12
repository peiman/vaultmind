package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/require"
)

// TestWriteDoctorHuman_GapLineExcludesNonValidationSurfaced is the regression
// test for the under-reporting gap-line bug (3-lens review of PR #31, Fix 1).
//
// The gap line should report exactly the raw validation findings doctor does
// NOT surface as per-item lines (unknown_type / invalid_status). The surfaced
// validation findings are precisely MissingRequiredFields + BrokenReferences.
// The OLD formula subtracted the full surfaced rollup (errCount+warnCount),
// which folds in NON-validation surfaced items (e.g. ObsidianIncompatibleLinks,
// UnresolvedLinks) that are NOT in the validation aggregate — under-reporting
// the gap, and wrongly suppressing the line entirely when non-validation
// surfaced items are numerous.
//
// This fixture has:
//   - Non-validation surfaced warnings: ObsidianIncompatibleLinks=2, UnresolvedLinks=1
//   - Validation findings (raw aggregate): Warnings=5 (e.g. 5 unknown_type)
//   - Surfaced validation findings: MissingRequiredFields=1 + BrokenReferences=1 = 2
//
// rawTotal = 0 errors + 5 warnings = 5
// validationSurfaced = MissingRequiredFields(1) + BrokenReferences(1) = 2
// CORRECT gap = 5 - 2 = 3
//
// The OLD formula computed surfacedTotal = errCount+warnCount which, with the
// non-validation surfaced items, is much larger than rawTotal (5), so it would
// suppress the gap line entirely (rawTotal > surfacedTotal is false).
func TestWriteDoctorHuman_GapLineExcludesNonValidationSurfaced(t *testing.T) {
	result := &query.DoctorResult{
		VaultPath: "/tmp/v",
		ValidationSummary: &query.StatusIssuesSummary{
			Errors:   0,
			Warnings: 5, // raw aggregate: 5 validation findings
		},
		Issues: query.DoctorIssues{
			// Surfaced validation findings (these ARE rendered / counted).
			MissingRequiredFields: 1,
			BrokenReferences:      1,
			// Non-validation surfaced items — NOT part of the validation aggregate.
			ObsidianIncompatibleLinks: 2,
			IncompatibleLinkDetails: []query.IncompatibleLink{
				{SourcePath: "x.md", TargetRaw: "Y", SuggestedFix: "y"},
				{SourcePath: "z.md", TargetRaw: "W", SuggestedFix: "w"},
			},
			UnresolvedLinks: 1,
		},
	}

	var buf bytes.Buffer
	require.NoError(t, writeDoctorHuman(&buf, result, false))
	out := buf.String()

	// CORRECT gap = rawTotal(5) - validationSurfaced(MissingRequired 1 + BrokenRef 1 = 2) = 3.
	require.Contains(t, out, "+3 raw validation finding(s)",
		"gap must be rawTotal - (MissingRequiredFields+BrokenReferences), not rawTotal - (errCount+warnCount); got:\n%s", out)
	// It must NOT print the (wrong) under-reported / suppressed value: the OLD
	// formula would have suppressed the line entirely here.
	require.Contains(t, out, "--json result.validation_summary",
		"gap line must point the operator at the --json aggregate")
}

// TestWriteDoctorHuman_GapLineSuppressedWhenNoUnsurfacedValidation proves the
// gap line is NOT printed when every validation finding is already surfaced
// (rawTotal == MissingRequiredFields+BrokenReferences) — guards against a fix
// that always prints the line.
func TestWriteDoctorHuman_GapLineSuppressedWhenNoUnsurfacedValidation(t *testing.T) {
	result := &query.DoctorResult{
		VaultPath: "/tmp/v",
		ValidationSummary: &query.StatusIssuesSummary{
			Errors:   1, // 1 missing_required_field
			Warnings: 1, // 1 broken_reference
		},
		Issues: query.DoctorIssues{
			MissingRequiredFields: 1,
			BrokenReferences:      1,
		},
	}

	var buf bytes.Buffer
	require.NoError(t, writeDoctorHuman(&buf, result, false))
	out := buf.String()

	require.NotContains(t, out, "raw validation finding(s)",
		"gap line must be suppressed when all validation findings are surfaced; got:\n%s", out)
}

// TestWriteDoctorHuman_BrokenReferencesLine is the regression test for Fix 3:
// because BrokenReferences now feeds the surfaced WARNING rollup, the human
// report must also render a per-issue line so the count is not invisible. The
// line must name the real validate subcommand so the operator can drill in.
func TestWriteDoctorHuman_BrokenReferencesLine(t *testing.T) {
	result := &query.DoctorResult{
		VaultPath: "/tmp/v",
		Issues: query.DoctorIssues{
			BrokenReferences: 3,
		},
	}

	var buf bytes.Buffer
	require.NoError(t, writeDoctorHuman(&buf, result, false))
	out := buf.String()

	require.Contains(t, out, "Broken references: 3",
		"human report must surface a broken-references line when BrokenReferences > 0; got:\n%s", out)
	require.Contains(t, out, "frontmatter validate",
		"broken-references line must name the real validate subcommand; got:\n%s", out)
	// Sanity: the surfaced rollup counts it as a warning, so the line must not
	// leave the count invisible.
	require.True(t, strings.Contains(out, "Broken references:"),
		"broken-references detail line is required when count > 0")
}

// TestWriteDoctorHuman_BrokenReferencesLineOmittedWhenZero proves the line is
// absent when there are no broken references (Fix 3, the omit case).
func TestWriteDoctorHuman_BrokenReferencesLineOmittedWhenZero(t *testing.T) {
	result := &query.DoctorResult{
		VaultPath: "/tmp/v",
		Issues:    query.DoctorIssues{BrokenReferences: 0},
	}

	var buf bytes.Buffer
	require.NoError(t, writeDoctorHuman(&buf, result, false))
	require.NotContains(t, buf.String(), "Broken references:",
		"broken-references line must be omitted when BrokenReferences == 0")
}
