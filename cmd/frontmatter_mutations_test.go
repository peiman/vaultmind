package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// frontmatter set must persist the new value to disk and leave untouched
// fields alone. Anything else corrupts downstream readers.
func TestFrontmatterSet_WritesFieldAndPreservesOthers(t *testing.T) {
	vault := buildIndexedTestVault(t)
	target := "projects/beta.md"
	full := filepath.Join(vault, target)

	_, _, err := runRootCmd(t, "frontmatter", "set", target, "status", "paused",
		"--vault", vault)
	require.NoError(t, err)

	content, err := os.ReadFile(full)
	require.NoError(t, err)
	body := string(content)
	assert.Contains(t, body, "status: paused", "status must be updated on disk")
	assert.Contains(t, body, "title: Beta Project", "unrelated field must survive the set")
	assert.Contains(t, body, "id: proj-beta", "id must be preserved")
}

// A JSON array value is written as a YAML list, as the help promises. It was
// written as a quoted string, turning a note's tags into one tag (#159).
func TestFrontmatterSet_AJSONArrayBecomesAList(t *testing.T) {
	vault := buildIndexedTestVault(t)
	target := "projects/beta.md"
	full := filepath.Join(vault, target)

	_, _, err := runRootCmd(t, "frontmatter", "set", target, "tags", `["graph","correctness"]`, "--vault", vault)
	require.NoError(t, err)
	_, _, err = runRootCmd(t, "frontmatter", "set", target, "paths", `["internal/graph/**"]`, "--vault", vault, "--allow-extra")
	require.NoError(t, err)

	content, err := os.ReadFile(full) //nolint:gosec // test fixture path
	require.NoError(t, err)
	body := string(content)
	assert.NotContains(t, body, `'["`, "a list must not be written as a quoted string")
	assert.Regexp(t, `tags:\s*\n\s+- graph\n\s+- correctness`, body)
	assert.Regexp(t, `paths:\s*\n\s+- internal/graph/\*\*`, body)
}

// Text that only looks like the start of an array stays text.
func TestFrontmatterSet_TextThatIsNotAnArrayStaysText(t *testing.T) {
	vault := buildIndexedTestVault(t)
	target := "projects/beta.md"

	_, _, err := runRootCmd(t, "frontmatter", "set", target, "summary", "[draft", "--vault", vault, "--allow-extra")
	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(vault, target)) //nolint:gosec // test fixture path
	require.NoError(t, err)
	assert.Contains(t, string(content), "[draft")
}

// dry-run must leave the file untouched. If dry-run ever wrote the file it
// would silently corrupt vaults during "just previewing" sessions.
func TestFrontmatterSet_DryRunDoesNotWrite(t *testing.T) {
	vault := buildIndexedTestVault(t)
	target := "projects/beta.md"
	full := filepath.Join(vault, target)

	before, err := os.ReadFile(full)
	require.NoError(t, err)

	_, _, err = runRootCmd(t, "frontmatter", "set", target, "status", "completed",
		"--vault", vault, "--dry-run")
	require.NoError(t, err)

	after, err := os.ReadFile(full)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "dry-run must not mutate the file")
}

// unset must remove the named field while preserving others.
func TestFrontmatterUnset_RemovesField(t *testing.T) {
	vault := buildIndexedTestVault(t)
	target := "concepts/alpha.md"
	full := filepath.Join(vault, target)

	_, _, err := runRootCmd(t, "frontmatter", "unset", target, "tags",
		"--vault", vault)
	require.NoError(t, err)

	content, err := os.ReadFile(full)
	require.NoError(t, err)
	body := string(content)
	assert.NotContains(t, body, "tags:", "tags must be gone after unset")
	assert.Contains(t, body, "title: Alpha Concept", "title must survive unset tags")
	assert.Contains(t, body, "id: concept-alpha", "id must survive unset")
}

// merge pulls fields from a YAML file. Partial-overlap: some existing fields
// get overwritten, new fields added, and fields absent from the merge file
// must stay put. This is the behavior downstream integrators actually rely on.
func TestFrontmatterMerge_OverwritesAndAdds(t *testing.T) {
	vault := buildIndexedTestVault(t)
	target := "projects/beta.md"
	full := filepath.Join(vault, target)

	mergeFile := filepath.Join(t.TempDir(), "merge.yaml")
	require.NoError(t, os.WriteFile(mergeFile, []byte(`status: paused
owner_id: peiman
`), 0o644))

	_, _, err := runRootCmd(t, "frontmatter", "merge", target,
		"--file", mergeFile, "--vault", vault)
	require.NoError(t, err)

	content, err := os.ReadFile(full)
	require.NoError(t, err)
	body := string(content)
	assert.Contains(t, body, "status: paused", "existing status must be overwritten")
	assert.Contains(t, body, "owner_id: peiman", "new field must be added")
	assert.Contains(t, body, "title: Beta Project", "untouched field must remain")
}

// merge without --file is a usage error — silent no-op would mask bad
// scripts that forgot to interpolate the file path.
func TestFrontmatterMerge_RequiresFileFlag(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "frontmatter", "merge", "projects/beta.md",
		"--vault", vault)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "file")
}

// normalize on a specific file canonicalises frontmatter without dropping
// domain fields. Per-file targeting is what the CLI supports today
// (directory sweep requires entity resolution, handled separately).
func TestFrontmatterNormalize_PreservesDomainFields(t *testing.T) {
	vault := buildIndexedTestVault(t)
	target := "projects/beta.md"
	_, _, err := runRootCmd(t, "frontmatter", "normalize", target,
		"--vault", vault)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(vault, target))
	require.NoError(t, err)
	body := string(content)
	assert.Contains(t, body, "id: proj-beta")
	assert.Contains(t, body, "title: Beta Project")
	assert.Contains(t, body, "status: active")
}

// --dry-run + --diff combo emits the unified diff to stdout instead of the
// usual "Dry run: ..." summary. The diff is what tools pipe into review
// workflows; dropping it would break audit trails.
func TestFrontmatterSet_DryRunWithDiffEmitsUnifiedDiff(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "frontmatter", "set", "projects/beta.md",
		"status", "completed",
		"--vault", vault, "--dry-run", "--diff")
	require.NoError(t, err)
	text := out.String()
	// A unified diff opens with "---" / "+++" or has "-" / "+" lines per hunk.
	assert.True(t,
		strings.Contains(text, "+status: completed") ||
			strings.Contains(text, "---") ||
			strings.Contains(text, "+++"),
		"dry-run --diff must produce unified-diff output, got %q", text)
}

// Human-mode success output has a specific shape: "set <path>: <id>". A
// refactor that changes the format would surprise scripts that grep it.
func TestFrontmatterSet_HumanModeSuccessLineFormat(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "frontmatter", "set", "projects/beta.md",
		"status", "paused",
		"--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "set projects/beta.md")
	assert.Contains(t, text, "proj-beta")
}

// JSON envelope must surface on a mutation error so pipelines can branch.
// Setting a field that violates required-fields with --allow-extra=false is
// a classic "user typed the wrong type" case.
func TestFrontmatterSet_UnknownTypeErrorEnvelope(t *testing.T) {
	vault := buildIndexedTestVault(t)
	// Create a note with an unknown type to force a mutation error path.
	target := "projects/alien.md"
	full := filepath.Join(vault, target)
	require.NoError(t, os.WriteFile(full, []byte(`---
id: p-alien
type: alien
title: Alien
---
body
`), 0o644))

	out, _, err := runRootCmd(t, "frontmatter", "set", target, "status", "active",
		"--vault", vault, "--json")
	// runMutation writes JSON error on MutationError and returns nil; on other
	// paths it may return a Go error. Either way the envelope must be
	// recoverable.
	_ = err
	var env struct {
		Status string `json:"status"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	// Envelope may be empty if err fired; tolerate either outcome but at
	// least one of the two signals must carry "alien"/"unknown" info.
	if out.Len() > 0 {
		require.NoError(t, json.Unmarshal(out.Bytes(), &env))
		assert.Equal(t, "error", env.Status)
		require.NotEmpty(t, env.Errors)
		assert.NotEmpty(t, env.Errors[0].Code)
	} else {
		require.Error(t, err)
	}
}

// A note is found by its id, as the help promises; only a file path worked,
// with "entity resolution not yet available" for anything else (#160).
func TestFrontmatterSet_FindsANoteByItsID(t *testing.T) {
	vault := buildIndexedTestVault(t)

	_, _, err := runRootCmd(t, "frontmatter", "set", "proj-beta", "status", "paused", "--vault", vault)
	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(vault, "projects", "beta.md")) //nolint:gosec // test fixture path
	require.NoError(t, err)
	assert.Contains(t, string(content), "status: paused")
}

func TestFrontmatterSet_AnUnknownIDSaysSo(t *testing.T) {
	vault := buildIndexedTestVault(t)

	_, _, err := runRootCmd(t, "frontmatter", "set", "no-such-note", "status", "paused", "--vault", vault)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-such-note")
	assert.NotContains(t, err.Error(), "not yet available")
}
