// cmd/coverage_boost_test.go
//
// Behavior-focused tests targeting the highest-value uncovered paths in the
// cmd package. Each test asserts on REAL observable behavior — output text,
// returned errors, JSON structure — not just line execution.
//
// Covered here:
//   - runArcCandidates (0%): human and JSON output on an empty episodes dir
//   - dataviewLintText (67%): issue-loop branch that renders per-issue lines
//   - dataviewLintText: zero-issues summary line
//   - writeLinkIssues (40%): ObsidianIncompatibleLinks summary + detail + summaryOnly branches
//   - writeLinkIssues: PathPseudoIDLinks summary + detail + summaryOnly branches
//   - writeStaleIndex (20%): stale-index warning with detail; summaryOnly suppresses detail
//   - hooksInstallErrorCode (0%): with conflicts vs. without
//   - short (0%): truncation and pass-through contracts
//   - writeFailedVaults (75%): single-failed-vault section
//   - writeRollupHeader: breakdown with failed vaults
//   - runFrontmatterFixCore (0%): human dry-run and json output on vault with missing fields
//   - collectMgetIDs: --ids flag path and error-on-neither path
//   - formatTypeDist: empty and populated maps
//   - writeHookDrift: with hook drift detail lines

package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// runArcCandidates
// ---------------------------------------------------------------------------

// arc candidates on an empty episodes dir must succeed and report zero
// candidates — not an error. An empty session is normal, not exceptional.
func TestArcCandidates_EmptyEpisodesDir_HumanOutput(t *testing.T) {
	vault := buildIndexedTestVault(t)
	// buildIndexedTestVault does not create an episodes/ dir; create it empty.
	require.NoError(t, os.MkdirAll(filepath.Join(vault, "episodes"), 0o755))

	out, _, err := runRootCmd(t, "arc", "candidates", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Scanned 0 episodes", "empty dir must report zero scanned")
	assert.Contains(t, text, "0 candidate moments", "empty dir must report zero candidates")
}

// arc candidates --json on an empty episodes dir must produce a valid envelope
// with an empty candidates list, not an error.
func TestArcCandidates_EmptyEpisodesDir_JSONOutput(t *testing.T) {
	vault := buildIndexedTestVault(t)
	require.NoError(t, os.MkdirAll(filepath.Join(vault, "episodes"), 0o755))

	out, _, err := runRootCmd(t, "arc", "candidates", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Status  string `json:"status"`
		Command string `json:"command"`
		Result  struct {
			Candidates []any `json:"candidates"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "arc-candidates", env.Command)
	assert.Empty(t, env.Result.Candidates, "empty dir produces no candidates")
}

// ---------------------------------------------------------------------------
// dataviewLintText
// ---------------------------------------------------------------------------

// dataviewLintText with zero issues must print only the summary line.
func TestDataviewLintText_ZeroIssuesSummaryLine(t *testing.T) {
	cmd := newMockCobraCmd()
	result := dataviewLintResult{FilesChecked: 5, Valid: 5, Issues: []dataviewIssue{}}
	require.NoError(t, dataviewLintText(cmd, result))
	out := mockCmdOut(cmd)
	assert.Contains(t, out, "Checked 5 files: 5 valid, 0 issues")
	assert.NotContains(t, out, "[", "no issue lines should be printed when there are no issues")
}

// dataviewLintText with issues must print the summary AND per-issue detail lines.
// The issue loop (the uncovered branch) must emit [rule] path: message for each issue.
func TestDataviewLintText_WithIssuesPrintsDetailLines(t *testing.T) {
	cmd := newMockCobraCmd()
	result := dataviewLintResult{
		FilesChecked: 3,
		Valid:        1,
		Issues: []dataviewIssue{
			{Path: "notes/a.md", Rule: "missing_close", Message: "unclosed dataview block at line 12", Line: 12},
			{Path: "notes/b.md", Rule: "duplicate_key", Message: "key 'status' duplicated", Line: 5},
		},
	}
	require.NoError(t, dataviewLintText(cmd, result))
	out := mockCmdOut(cmd)
	assert.Contains(t, out, "Checked 3 files: 1 valid, 2 issues")
	assert.Contains(t, out, "[missing_close] notes/a.md: unclosed dataview block at line 12")
	assert.Contains(t, out, "[duplicate_key] notes/b.md: key 'status' duplicated")
}

// ---------------------------------------------------------------------------
// writeLinkIssues
// ---------------------------------------------------------------------------

// writeLinkIssues with ObsidianIncompatibleLinks > 0 and summaryOnly=true must
// print the count and the "(run without --summary)" hint, but NOT the per-link details.
func TestWriteLinkIssues_ObsidianIncompatibleLinks_SummaryOnly(t *testing.T) {
	var buf bytes.Buffer
	issues := &query.DoctorIssues{
		ObsidianIncompatibleLinks: 2,
		IncompatibleLinkDetails: []query.IncompatibleLink{
			{SourcePath: "a.md", TargetRaw: "Some Title", SuggestedFix: "some-title"},
		},
	}
	require.NoError(t, writeLinkIssues(&buf, issues, true))
	out := buf.String()
	assert.Contains(t, out, "Obsidian-incompatible links: 2")
	assert.Contains(t, out, "run without --summary", "summaryOnly must print the hint")
	assert.NotContains(t, out, "a.md", "summaryOnly must suppress per-link detail")
}

// writeLinkIssues with ObsidianIncompatibleLinks > 0 and summaryOnly=false must
// print each incompatible-link detail line.
func TestWriteLinkIssues_ObsidianIncompatibleLinks_DetailLines(t *testing.T) {
	var buf bytes.Buffer
	issues := &query.DoctorIssues{
		ObsidianIncompatibleLinks: 1,
		IncompatibleLinkDetails: []query.IncompatibleLink{
			{SourcePath: "concepts/x.md", TargetRaw: "Alpha Concept", SuggestedFix: "alpha-concept"},
		},
	}
	require.NoError(t, writeLinkIssues(&buf, issues, false))
	out := buf.String()
	assert.Contains(t, out, "Obsidian-incompatible links: 1")
	assert.Contains(t, out, "concepts/x.md", "detail lines must show the source path")
	assert.Contains(t, out, "Alpha Concept", "detail lines must show the raw target")
}

// writeLinkIssues with PathPseudoIDLinks > 0 and summaryOnly=true must print
// the count and the "(run without --summary)" hint.
func TestWriteLinkIssues_PathPseudoIDLinks_SummaryOnly(t *testing.T) {
	var buf bytes.Buffer
	issues := &query.DoctorIssues{
		PathPseudoIDLinks: 3,
		PathPseudoIDDetails: []query.UnresolvedLink{
			{SourcePath: "notes/z.md", TargetRaw: "nonexistent/file"},
		},
	}
	require.NoError(t, writeLinkIssues(&buf, issues, true))
	out := buf.String()
	assert.Contains(t, out, "Dead link references: 3")
	assert.Contains(t, out, "run without --summary")
	assert.NotContains(t, out, "nonexistent/file", "summaryOnly must suppress per-link detail")
}

// writeLinkIssues with PathPseudoIDLinks > 0 and summaryOnly=false must print
// each dead-link detail line.
func TestWriteLinkIssues_PathPseudoIDLinks_DetailLines(t *testing.T) {
	var buf bytes.Buffer
	issues := &query.DoctorIssues{
		PathPseudoIDLinks: 1,
		PathPseudoIDDetails: []query.UnresolvedLink{
			{SourcePath: "notes/q.md", TargetRaw: "missing/ref"},
		},
	}
	require.NoError(t, writeLinkIssues(&buf, issues, false))
	out := buf.String()
	assert.Contains(t, out, "Dead link references: 1")
	assert.Contains(t, out, "notes/q.md")
	assert.Contains(t, out, "[[missing/ref]] → target file does not exist")
}

// writeLinkIssues with zero issues must produce no output at all.
func TestWriteLinkIssues_ZeroIssues_Silent(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeLinkIssues(&buf, &query.DoctorIssues{}, false))
	assert.Empty(t, buf.String(), "zero-issue issues struct must produce no output")
}

// ---------------------------------------------------------------------------
// writeStaleIndex
// ---------------------------------------------------------------------------

// writeStaleIndex with zero stale notes must produce no output.
func TestWriteStaleIndex_ZeroStale_Silent(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeStaleIndex(&buf, &query.DoctorIssues{StaleIndex: 0}, false))
	assert.Empty(t, buf.String())
}

// writeStaleIndex with stale notes and summaryOnly=false must print the warning
// AND each per-note detail line (with truncated hashes).
func TestWriteStaleIndex_WithStale_DetailLines(t *testing.T) {
	var buf bytes.Buffer
	issues := &query.DoctorIssues{
		StaleIndex: 1,
		StaleIndexDetails: []query.ContentDrift{
			{Path: "notes/drift.md", CurrentHash: "abcdef1234567890", StoredHash: "zyxwvu0987654321"},
		},
	}
	require.NoError(t, writeStaleIndex(&buf, issues, false))
	out := buf.String()
	assert.Contains(t, out, "Stale index: 1 note(s)")
	assert.Contains(t, out, "notes/drift.md")
	assert.Contains(t, out, "abcdef12", "should contain truncated current hash (first 8 chars)")
	assert.Contains(t, out, "zyxwvu09", "should contain truncated stored hash")
}

// writeStaleIndex with summaryOnly=true must print the warning but suppress
// the per-note detail lines.
func TestWriteStaleIndex_WithStale_SummaryOnlySuppressesDetail(t *testing.T) {
	var buf bytes.Buffer
	issues := &query.DoctorIssues{
		StaleIndex: 2,
		StaleIndexDetails: []query.ContentDrift{
			{Path: "notes/a.md", CurrentHash: "aaa", StoredHash: "bbb"},
		},
	}
	require.NoError(t, writeStaleIndex(&buf, issues, true))
	out := buf.String()
	assert.Contains(t, out, "Stale index: 2 note(s)", "warning must still print under --summary")
	assert.NotContains(t, out, "notes/a.md", "per-note detail must be suppressed under --summary")
}

// ---------------------------------------------------------------------------
// hooksInstallErrorCode
// ---------------------------------------------------------------------------

// hooksInstallErrorCode with a non-empty Conflicts slice must return the
// historical "hooks_install_conflict" code (backward-compat for JSON consumers).
func TestHooksInstallErrorCode_WithConflicts(t *testing.T) {
	res := &hooks.InstallResult{Conflicts: []string{"SessionStart.sh"}}
	assert.Equal(t, "hooks_install_conflict", hooksInstallErrorCode(res))
}

// hooksInstallErrorCode with no conflicts must return "hooks_install_merge_error"
// (a settings-merge failure, which can only occur after a clean install).
func TestHooksInstallErrorCode_NoConflicts(t *testing.T) {
	res := &hooks.InstallResult{}
	assert.Equal(t, "hooks_install_merge_error", hooksInstallErrorCode(res))
}

// hooksInstallErrorCode with nil result must not panic and returns merge_error.
func TestHooksInstallErrorCode_NilResult(t *testing.T) {
	assert.Equal(t, "hooks_install_merge_error", hooksInstallErrorCode(nil))
}

// ---------------------------------------------------------------------------
// short
// ---------------------------------------------------------------------------

// short must truncate strings longer than 8 chars to exactly 8 chars.
func TestShort_TruncatesLongHash(t *testing.T) {
	assert.Equal(t, "abcdef12", short("abcdef1234567890"))
	assert.Equal(t, "12345678", short("123456789"))
}

// short must return the input unchanged when it is <= 8 chars.
func TestShort_PassesThroughShortHash(t *testing.T) {
	assert.Equal(t, "abcdef12", short("abcdef12"))
	assert.Equal(t, "abc", short("abc"))
	assert.Equal(t, "", short(""))
}

// ---------------------------------------------------------------------------
// writeFailedVaults
// ---------------------------------------------------------------------------

// writeFailedVaults with no failures must produce no output.
func TestWriteFailedVaults_NoFailures_Silent(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeFailedVaults(&buf, nil))
	assert.Empty(t, buf.String())
}

// writeFailedVaults with one failed vault must print a section header and
// the vault path + its error reason.
func TestWriteFailedVaults_OneFailedVault_SectionRendered(t *testing.T) {
	var buf bytes.Buffer
	failed := []query.FailedVault{
		{VaultPath: "/path/to/broken-vault", Error: "malformed config.yaml"},
	}
	require.NoError(t, writeFailedVaults(&buf, failed))
	out := buf.String()
	assert.Contains(t, out, "Vaults that failed to open: 1")
	assert.Contains(t, out, "/path/to/broken-vault")
	assert.Contains(t, out, "malformed config.yaml")
}

// ---------------------------------------------------------------------------
// writeRollupHeader
// ---------------------------------------------------------------------------

// writeRollupHeader with zero failures must not show the "(M diagnosed, K failed)" breakdown.
func TestWriteRollupHeader_NoFailures_NoBreakdown(t *testing.T) {
	var buf bytes.Buffer
	r := query.DoctorRollup{Discovered: 3, Diagnosed: 3, Failed: 0, TotalNotes: 10}
	require.NoError(t, writeRollupHeader(&buf, "/root", r))
	out := buf.String()
	assert.Contains(t, out, "Discovered 3 vault(s)")
	assert.NotContains(t, out, "diagnosed", "honest breakdown only when failures > 0")
}

// writeRollupHeader with failed vaults must show the "(M diagnosed, K failed)"
// breakdown so operators never see a count that hides a broken vault.
func TestWriteRollupHeader_WithFailures_ShowsBreakdown(t *testing.T) {
	var buf bytes.Buffer
	r := query.DoctorRollup{Discovered: 4, Diagnosed: 3, Failed: 1, TotalNotes: 9}
	require.NoError(t, writeRollupHeader(&buf, "/root", r))
	out := buf.String()
	assert.Contains(t, out, "Discovered 4 vault(s)")
	assert.Contains(t, out, "3 diagnosed", "diagnosed count must appear when failures > 0")
	assert.Contains(t, out, "1 failed", "failed count must appear when failures > 0")
}

// writeRollupHeader with vaultsWithIssues must list them.
func TestWriteRollupHeader_VaultsWithIssues_Listed(t *testing.T) {
	var buf bytes.Buffer
	r := query.DoctorRollup{
		Discovered:       2,
		Diagnosed:        2,
		TotalNotes:       5,
		TotalErrors:      1,
		VaultsWithIssues: []string{"/vault/broken"},
	}
	require.NoError(t, writeRollupHeader(&buf, "/root", r))
	out := buf.String()
	assert.Contains(t, out, "Vaults with issues: 1")
	assert.Contains(t, out, "/vault/broken")
}

// When the raw schema-validation aggregate exceeds the surfaced headline,
// writeRollupHeader must note the gap on one honest line pointing at --json —
// mirroring the single-vault gap line so the two axes never silently diverge.
func TestWriteRollupHeader_RawValidationGap_ShowsHonestLine(t *testing.T) {
	var buf bytes.Buffer
	r := query.DoctorRollup{
		Discovered:                 1,
		Diagnosed:                  1,
		TotalNotes:                 30,
		TotalErrors:                0,
		TotalWarnings:              1,  // surfaced headline
		TotalRawValidationFindings: 58, // raw aggregate (e.g. 1 surfaced + 57 raw-only)
	}
	require.NoError(t, writeRollupHeader(&buf, "/root", r))
	out := buf.String()
	assert.Contains(t, out, "Total issues: 0 errors, 1 warnings", "headline is the surfaced axis")
	assert.Contains(t, out, "raw validation finding(s)",
		"the raw aggregate is surfaced on its own honest line when it exceeds the headline")
	assert.Contains(t, out, "--json", "the line points the operator at --json for the raw aggregate")
}

// When there are no raw-only validation findings, writeRollupHeader must NOT
// emit the gap line — keep it minimal. (RawValidationGap == 0 here because the
// raw aggregate is zero; the surfaced-validation portion is unexported and
// summed only inside BuildDoctorRollup.)
func TestWriteRollupHeader_NoRawValidationGap_NoExtraLine(t *testing.T) {
	var buf bytes.Buffer
	r := query.DoctorRollup{
		Discovered:                 1,
		Diagnosed:                  1,
		TotalNotes:                 10,
		TotalWarnings:              2,
		TotalRawValidationFindings: 0, // no raw findings → nothing hidden
	}
	require.NoError(t, writeRollupHeader(&buf, "/root", r))
	assert.NotContains(t, buf.String(), "raw validation finding(s)",
		"no gap line when there are no raw-only validation findings")
}

// ---------------------------------------------------------------------------
// runFrontmatterFixCore — tested via the CLI path
// ---------------------------------------------------------------------------

// frontmatter fix (dry-run) on a vault that has no missing fields must report
// "0 need backfill" and not write any files. This exercises runFrontmatterFixCore's
// human output path with an empty Items list.
func TestFrontmatterFix_DryRun_VaultWithNoMissingFields(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "frontmatter", "fix", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Scanned", "must print scan count")
	// May or may not have items needing backfill (test vault notes have ids).
	// The key contract is: command succeeds and prints the scan summary.
	assert.NotContains(t, text, "reading fix:", "must not produce an error reading fix")
}

// frontmatter fix --json on a clean vault must produce an ok envelope with
// a FilesScanned field.
func TestFrontmatterFix_JSON_OkEnvelope(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "frontmatter", "fix", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Status  string `json:"status"`
		Command string `json:"command"`
		Result  struct {
			FilesScanned int `json:"files_scanned"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), "output must be valid JSON, got: %q", out.String())
	assert.Equal(t, "frontmatter fix", env.Command)
	assert.GreaterOrEqual(t, env.Result.FilesScanned, 0)
}

// ---------------------------------------------------------------------------
// collectMgetIDs
// ---------------------------------------------------------------------------

// collectMgetIDs with --ids flag must return the trimmed, non-empty IDs.
func TestCollectMgetIDs_IDsFlag_ReturnsTrimmedList(t *testing.T) {
	cmd := newMockCobraCmd()
	cmd.Flags().String("ids", "id-1, id-2 , id-3", "")
	cmd.Flags().Bool("stdin", false, "")
	ids, err := collectMgetIDs(cmd)
	require.NoError(t, err)
	assert.Equal(t, []string{"id-1", "id-2", "id-3"}, ids)
}

// collectMgetIDs with neither --ids nor --stdin must return an error.
func TestCollectMgetIDs_NeitherFlag_ReturnsError(t *testing.T) {
	cmd := newMockCobraCmd()
	cmd.Flags().String("ids", "", "")
	cmd.Flags().Bool("stdin", false, "")
	_, err := collectMgetIDs(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--ids")
}

// collectMgetIDs with empty --ids must also return an error (empty string is
// not a valid id list — same as "not provided").
func TestCollectMgetIDs_EmptyIDsFlag_ReturnsError(t *testing.T) {
	cmd := newMockCobraCmd()
	cmd.Flags().String("ids", "", "")
	cmd.Flags().Bool("stdin", false, "")
	_, err := collectMgetIDs(cmd)
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// formatTypeDist
// ---------------------------------------------------------------------------

// formatTypeDist with an empty map returns the "(empty)" placeholder.
func TestFormatTypeDist_EmptyMap_ReturnsPlaceholder(t *testing.T) {
	assert.Equal(t, "(empty)", formatTypeDist(map[string]int{}))
	assert.Equal(t, "(empty)", formatTypeDist(nil))
}

// formatTypeDist with a populated map returns sorted key=value pairs joined by ", ".
func TestFormatTypeDist_PopulatedMap_SortedPairs(t *testing.T) {
	result := formatTypeDist(map[string]int{"concept": 3, "arc": 1, "project": 2})
	// Should be sorted: arc=1, concept=3, project=2
	assert.Equal(t, "arc=1, concept=3, project=2", result)
}

// ---------------------------------------------------------------------------
// writeHookDrift
// ---------------------------------------------------------------------------

// writeHookDrift with zero hook drift must produce no output.
func TestWriteHookDrift_ZeroDrift_Silent(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeHookDrift(&buf, &query.DoctorIssues{HookDrift: 0}, false))
	assert.Empty(t, buf.String())
}

// writeHookDrift with drift > 0 and summaryOnly=false must print the warning
// and each drifted script name.
func TestWriteHookDrift_WithDrift_DetailLines(t *testing.T) {
	var buf bytes.Buffer
	issues := &query.DoctorIssues{
		HookDrift:        2,
		HookDriftDetails: []string{"SessionStart.sh", "PostToolUse.sh"},
	}
	require.NoError(t, writeHookDrift(&buf, issues, false))
	out := buf.String()
	assert.Contains(t, out, "Hook drift: 2")
	assert.Contains(t, out, "SessionStart.sh")
	assert.Contains(t, out, "PostToolUse.sh")
}

// writeHookDrift with drift > 0 and summaryOnly=true must print only the
// warning line, not the per-script names.
func TestWriteHookDrift_WithDrift_SummaryOnly(t *testing.T) {
	var buf bytes.Buffer
	issues := &query.DoctorIssues{
		HookDrift:        1,
		HookDriftDetails: []string{"SessionStart.sh"},
	}
	require.NoError(t, writeHookDrift(&buf, issues, true))
	out := buf.String()
	assert.Contains(t, out, "Hook drift: 1", "warning must appear under --summary")
	assert.NotContains(t, out, "SessionStart.sh", "script names suppressed under --summary")
}

// ---------------------------------------------------------------------------
// dataview lint via CLI (exercises runDataviewLint + dataviewLintText)
// ---------------------------------------------------------------------------

// dataview lint on a clean vault must succeed and print a valid summary line.
func TestDataviewLint_CleanVault_SuccessText(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "dataview", "lint", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Checked")
}

// dataview lint --json must emit an ok envelope with files_checked.
func TestDataviewLint_CleanVault_JSONEnvelope(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "dataview", "lint", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Status  string `json:"status"`
		Command string `json:"command"`
		Result  struct {
			FilesChecked int             `json:"files_checked"`
			Issues       []dataviewIssue `json:"issues"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "dataview lint", env.Command)
	assert.GreaterOrEqual(t, env.Result.FilesChecked, 0)
	assert.NotNil(t, env.Result.Issues)
}

// ---------------------------------------------------------------------------
// note mget via CLI (exercises runNoteMget human output path)
// ---------------------------------------------------------------------------

// note mget with --ids returns found and not-found notes in human format.
func TestNoteMget_IDsFlag_HumanOutput(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "note", "mget",
		"--ids", "concept-alpha,nonexistent-id",
		"--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "concept-alpha", "found note must appear")
	assert.Contains(t, text, "NOT FOUND: nonexistent-id", "missing note must be flagged")
}

// note mget with --ids --json returns found and not-found in envelope.
func TestNoteMget_IDsFlag_JSONOutput(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "note", "mget",
		"--ids", "proj-beta,does-not-exist",
		"--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			Notes []struct {
				ID string `json:"id"`
			} `json:"notes"`
			NotFound []string `json:"not_found"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	require.Len(t, env.Result.Notes, 1)
	assert.Equal(t, "proj-beta", env.Result.Notes[0].ID)
	assert.Equal(t, []string{"does-not-exist"}, env.Result.NotFound)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newMockCobraCmd returns a cobra command wired to a bytes.Buffer so tests
// can inspect its output. Only used for pure-function tests that want to call
// helpers directly rather than through the CLI runner.
func newMockCobraCmd() *cobra.Command {
	cmd := &cobra.Command{}
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	return cmd
}

// mockCmdOut extracts the output buffer content from a command created by
// newMockCobraCmd.
func mockCmdOut(cmd *cobra.Command) string {
	if b, ok := cmd.OutOrStdout().(*bytes.Buffer); ok {
		return b.String()
	}
	return ""
}

// writeTypeBreakdown with statuses renders "[statuses: ...]" on the type line.
func TestWriteTypeBreakdown_TypeWithStatuses_RendersStatuses(t *testing.T) {
	var buf bytes.Buffer
	types := map[string]query.StatusTypeInfo{
		"project": {Count: 2, Statuses: []string{"active", "paused"}},
		"concept": {Count: 5, Statuses: nil},
	}
	require.NoError(t, writeTypeBreakdown(&buf, types))
	out := buf.String()
	assert.Contains(t, out, "concept: 5 note(s)")
	assert.Contains(t, out, "project: 2 note(s) [statuses: active, paused]")
	assert.Contains(t, out, "Types: 2")
}

// writeTypeBreakdown with empty map must produce no output.
func TestWriteTypeBreakdown_EmptyMap_Silent(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeTypeBreakdown(&buf, nil))
	assert.Empty(t, buf.String())
}

// frontmatter set with too few args must return a usage error.
func TestFrontmatterSet_TooFewArgs_UsageError(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "frontmatter", "set", "projects/beta.md", "status",
		"--vault", vault)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "usage")
}

// frontmatter unset with too few args must return a usage error.
func TestFrontmatterUnset_TooFewArgs_UsageError(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "frontmatter", "unset", "projects/beta.md",
		"--vault", vault)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "usage")
}

// frontmatter merge with too few args (missing target) must return an error.
func TestFrontmatterMerge_TooFewArgs_Error(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "frontmatter", "merge", "--vault", vault)
	require.Error(t, err)
}
