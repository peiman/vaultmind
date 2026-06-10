package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doctor (read-only diagnosis) now GAINS the per-type breakdown that
// `vault status` used to produce: per-type note counts plus the required
// fields and valid statuses for each type. The breakdown must appear in the
// JSON envelope regardless of --summary.
func TestDoctor_JSONIncludesPerTypeBreakdown(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			Types map[string]struct {
				Count    int      `json:"count"`
				Required []string `json:"required"`
				Statuses []string `json:"statuses"`
			} `json:"types"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.Contains(t, env.Result.Types, "concept")
	require.Contains(t, env.Result.Types, "project")
	assert.Equal(t, 2, env.Result.Types["concept"].Count, "two concept notes in the test vault")
	assert.Equal(t, 1, env.Result.Types["project"].Count, "one project note in the test vault")
	assert.Contains(t, env.Result.Types["concept"].Required, "title")
	assert.Contains(t, env.Result.Types["project"].Statuses, "active",
		"valid statuses must carry through from the registry")
}

// doctor JSON also carries the errors/warnings rollup that the cold-start
// view needs. The clean test vault has zero errors.
func TestDoctor_JSONIncludesIssuesSummary(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			IssuesSummary struct {
				Errors   int `json:"errors"`
				Warnings int `json:"warnings"`
			} `json:"issues_summary"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, 0, env.Result.IssuesSummary.Errors, "clean vault has no errors")
}

// doctor human output (no --summary) prints the per-type breakdown so the
// terminal user sees the type registry, not just the totals. Assert the
// breakdown's DISTINCTIVE "name: N note(s)" format (what writeTypeBreakdown
// emits) — a bare Contains("concept") also matches the incompatible-link
// detail lines, so it wouldn't actually prove the breakdown rendered.
func TestDoctor_HumanOutputShowsPerTypeBreakdown(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	// project carries statuses; concept does not — match the real rendered
	// lines (the fixture has 2 concepts, 1 project).
	assert.Contains(t, text, "concept: 2 note(s)",
		"per-type breakdown must render the concept count in its distinctive format")
	assert.Contains(t, text, "project: 1 note(s)",
		"per-type breakdown must render the project count in its distinctive format")
}

// doctor --summary is the cold-start view: totals + per-type breakdown +
// the errors/warnings rollup, and it still suppresses the noisy per-link
// detail lines (that's what --summary always did).
func TestDoctorSummary_ColdStartView(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--summary", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Vault: ", "cold-start view shows the vault path")
	assert.Contains(t, text, "Notes: ", "cold-start view shows note counts")
	assert.Contains(t, text, "concept", "cold-start view shows the per-type breakdown")
	assert.Contains(t, text, "Issues:", "cold-start view shows the errors/warnings rollup")
	// --summary keeps suppressing the verbose per-link enumeration.
	assert.NotContains(t, text, "→ [[",
		"--summary must not print per-link incompatible-link detail lines")
}

// The deprecated `vault status` is a hidden alias that prints a one-line
// stderr deprecation notice mentioning `doctor --summary`, then delegates to
// the doctor summary path — producing the same cold-start view on stdout.
func TestDeprecated_VaultStatus_WarnsAndDelegatesToDoctorSummary(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, errOut, err := runRootCmd(t, "vault", "status", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Vault: ", "delegated cold-start view must reach stdout")
	assert.Contains(t, text, "concept", "delegated view must carry the per-type breakdown")
	assert.Contains(t, text, "Issues:", "delegated view must carry the errors/warnings rollup")
	assertOneLineDeprecation(t, errOut.String(), "doctor --summary")
}

// The deprecated `vault status --json` must still emit a machine-readable
// envelope (delegating to doctor's JSON path) alongside the stderr notice.
func TestDeprecated_VaultStatus_JSONDelegates(t *testing.T) {
	isolateMeshEnv(t)
	vault := buildIndexedTestVault(t)
	out, errOut, err := runRootCmd(t, "vault", "status", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			Types map[string]json.RawMessage `json:"types"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.Contains(t, env.Result.Types, "concept",
		"delegated JSON must carry the per-type breakdown")
	assertOneLineDeprecation(t, errOut.String(), "doctor --summary")
}

// The `vault` parent dissolves: it only ever hosted `status`, so it is now
// hidden from the root listing (its single subcommand survives as the
// deprecated alias).
func TestVaultParent_HiddenFromRootListing(t *testing.T) {
	for _, c := range RootCmd.Commands() {
		if c.Name() == "vault" {
			assert.True(t, c.Hidden, "the 'vault' parent must be hidden from the root listing")
			return
		}
	}
	t.Fatal("vault parent command not found")
}

// The deprecation notice for vault status must be exactly one line and must
// mention the new path. (Shared assert lives in memory_taxonomy_test.go;
// this guards the contract locally too.)
func TestVaultStatus_DeprecationNoticeIsSingleLine(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, errOut, err := runRootCmd(t, "vault", "status", "--vault", vault)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(errOut.String(), "\n"), "\n")
	require.Len(t, lines, 1, "deprecation notice must be exactly one line: %q", errOut.String())
	assert.Contains(t, lines[0], "deprecated")
}
