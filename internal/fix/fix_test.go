package fix_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/fix"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vaultmind frontmatter fix --backfill (commits 3+4 of the schema
// foundation chain). Tests pin the contract: the command surfaces
// missing vaultmind-owned fields (created, vm_updated) on domain
// notes; with apply=true, writes them; never touches user-owned
// fields; default mode is dry-run (per arc-extending-not-overwriting,
// vaultmind never silently rewrites user files).

func setupFixVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  reference:
    required: [title]
  source:
    required: [title, url]
`), 0o644))
	return dir
}

func writeNote(t *testing.T, vault, name, body string) string {
	t.Helper()
	path := filepath.Join(vault, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

// staticResolver returns a fixed value — used by tests so we don't depend
// on the test environment's filesystem mtime or git history.
func staticResolver(date string) fix.CreatedDateResolver {
	return func(_ string) (string, string) { return date, "test" }
}

// TestRunBackfill_NoteMissingBothFields — domain note lacks both `created`
// and `vm_updated`. Dry-run reports both missing; apply writes both.
func TestRunBackfill_NoteMissingBothFields(t *testing.T) {
	vault := setupFixVault(t)
	notePath := writeNote(t, vault, "n.md", `---
id: ref-1
type: reference
title: T
---
body
`)

	t.Run("dry-run reports without writing", func(t *testing.T) {
		res, err := fix.RunBackfill(fix.Config{
			VaultPath:       vault,
			Apply:           false,
			CreatedResolver: staticResolver("2024-01-01"),
		})
		require.NoError(t, err)
		require.Len(t, res.Items, 1)
		item := res.Items[0]
		assert.ElementsMatch(t, []string{"created", "vm_updated"}, item.MissingFields)
		assert.Equal(t, "2024-01-01", item.ProposedValues["created"])
		assert.NotEmpty(t, item.ProposedValues["vm_updated"])

		// File on disk is unchanged.
		content, err := os.ReadFile(notePath)
		require.NoError(t, err)
		assert.NotContains(t, string(content), "created:")
		assert.NotContains(t, string(content), "vm_updated:")
	})

	t.Run("apply writes both", func(t *testing.T) {
		res, err := fix.RunBackfill(fix.Config{
			VaultPath:       vault,
			Apply:           true,
			CreatedResolver: staticResolver("2024-01-01"),
		})
		require.NoError(t, err)
		require.Len(t, res.Items, 1)
		assert.True(t, res.Applied)

		content, err := os.ReadFile(notePath)
		require.NoError(t, err)
		s := string(content)
		// YAML auto-quotes date-form strings to avoid timestamp-type
		// ambiguity. The mutator's SetKey emits `created: "2024-01-01"`
		// (with quotes) — both quoted and unquoted are valid YAML;
		// vaultmind picks quoted for safety. Test pins what's actually
		// produced.
		assert.Regexp(t, `created: "?2024-01-01"?`, s)
		assert.Contains(t, s, "vm_updated:")
	})
}

// TestRunBackfill_NoteMissingOnlyVMUpdated — note has created but no
// vm_updated. Only vm_updated reported missing; backfill adds only
// vm_updated. Existing created is left untouched.
func TestRunBackfill_NoteMissingOnlyVMUpdated(t *testing.T) {
	vault := setupFixVault(t)
	notePath := writeNote(t, vault, "n.md", `---
id: ref-2
type: reference
title: T
created: 2024-01-01
---
body
`)

	res, err := fix.RunBackfill(fix.Config{
		VaultPath:       vault,
		Apply:           true,
		CreatedResolver: staticResolver("WRONG-SHOULD-NOT-USE"),
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.Equal(t, []string{"vm_updated"}, res.Items[0].MissingFields)

	content, err := os.ReadFile(notePath)
	require.NoError(t, err)
	s := string(content)
	// Original `created: 2024-01-01` (unquoted) was already in the file
	// — and since the fix command doesn't touch it, the original
	// representation is preserved verbatim (no re-quoting).
	assert.Contains(t, s, "created: 2024-01-01", "existing created must be preserved verbatim")
	assert.Contains(t, s, "vm_updated:")
}

// TestRunBackfill_NoteMissingOnlyCreated — note has vm_updated but no
// created. Only created reported missing; backfill adds created. The
// mutator's auto-bump WILL update vm_updated to today (that's the
// vm_updated contract — every operation bumps it). The fix item's
// MissingFields list reflects only what was originally missing.
func TestRunBackfill_NoteMissingOnlyCreated(t *testing.T) {
	vault := setupFixVault(t)
	notePath := writeNote(t, vault, "n.md", `---
id: ref-3
type: reference
title: T
vm_updated: "2024-01-01T00:00:00Z"
---
body
`)

	res, err := fix.RunBackfill(fix.Config{
		VaultPath:       vault,
		Apply:           true,
		CreatedResolver: staticResolver("2020-06-15"),
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.Equal(t, []string{"created"}, res.Items[0].MissingFields)

	content, err := os.ReadFile(notePath)
	require.NoError(t, err)
	s := string(content)
	// New `created` written by the fix → YAML-quoted by the mutator's
	// SetKey to avoid date-string-as-timestamp ambiguity.
	assert.Regexp(t, `created: "?2020-06-15"?`, s)
	// vm_updated gets bumped by the mutator to today (auto-maintenance
	// contract from commit b5ff2ea). The old 2024-01-01 value must not
	// remain — confirms vm_updated is vaultmind-owned, not preserved.
	assert.NotContains(t, s, "2024-01-01")
}

// TestRunBackfill_NoteWithBothFields — note has both fields; not in items.
func TestRunBackfill_NoteWithBothFields(t *testing.T) {
	vault := setupFixVault(t)
	writeNote(t, vault, "n.md", `---
id: ref-4
type: reference
title: T
created: 2024-01-01
vm_updated: "2024-01-01T00:00:00Z"
---
body
`)

	res, err := fix.RunBackfill(fix.Config{
		VaultPath: vault,
		Apply:     false,
	})
	require.NoError(t, err)
	assert.Empty(t, res.Items, "complete note must not appear in the items list")
}

// TestRunBackfill_NonDomainNoteSkipped — file with no id+type is not
// classified as domain and gets nothing. Vaultmind doesn't track non-
// domain content.
func TestRunBackfill_NonDomainNoteSkipped(t *testing.T) {
	vault := setupFixVault(t)
	notePath := writeNote(t, vault, "stray.md", `---
some_field: yes
---
body
`)

	res, err := fix.RunBackfill(fix.Config{
		VaultPath: vault,
		Apply:     true,
	})
	require.NoError(t, err)
	assert.Empty(t, res.Items, "non-domain notes never appear in fix items")

	content, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.NotContains(t, string(content), "vm_updated:")
	assert.NotContains(t, string(content), "created:")
}

// TestRunBackfill_VMUpdatedFormatIsRFC3339 — the vm_updated value
// proposed (and written on apply) uses schema.VMUpdatedFormat.
// Per principle 7 (SSOT), every write site uses the same format.
func TestRunBackfill_VMUpdatedFormatIsRFC3339(t *testing.T) {
	vault := setupFixVault(t)
	notePath := writeNote(t, vault, "n.md", `---
id: ref-5
type: reference
title: T
---
body
`)

	res, err := fix.RunBackfill(fix.Config{
		VaultPath:       vault,
		Apply:           true,
		CreatedResolver: staticResolver("2024-01-01"),
	})
	require.NoError(t, err)
	proposed := res.Items[0].ProposedValues["vm_updated"]
	// Pattern from schema.VMUpdatedFormat = "2006-01-02T15:04:05Z".
	// Verify the proposed value matches, AND the written file has it
	// in YAML-quoted form (RFC3339 contains colons; YAML auto-quotes).
	assert.Regexp(t, `^20\d{2}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`, proposed)
	_ = schema.VMUpdatedFormat // ensure the SSOT constant is referenced

	content, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Regexp(t,
		`vm_updated: "20\d{2}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z"`,
		string(content),
	)
}

// TestRunBackfill_DryRunIncludesDiff — dry-run output includes the
// per-note diff so the user can audit what would change before applying.
func TestRunBackfill_DryRunIncludesDiff(t *testing.T) {
	vault := setupFixVault(t)
	writeNote(t, vault, "n.md", `---
id: ref-6
type: reference
title: T
---
body
`)

	res, err := fix.RunBackfill(fix.Config{
		VaultPath:       vault,
		Apply:           false,
		CreatedResolver: staticResolver("2024-01-01"),
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	// Diff shows the additions; YAML quotes the date-form string.
	assert.Regexp(t, `\+created: "?2024-01-01"?`, res.Items[0].Diff)
	assert.Contains(t, res.Items[0].Diff, "+vm_updated:")
}

// TestRunBackfill_PreservesUserOwnedFields — vaultmind only writes
// vaultmind-owned fields. User-owned fields (title, status, tags,
// related_ids, etc.) must be untouched even when the note is being
// backfilled.
func TestRunBackfill_PreservesUserOwnedFields(t *testing.T) {
	vault := setupFixVault(t)
	notePath := writeNote(t, vault, "n.md", `---
id: ref-7
type: reference
title: My Special Title
tags: [a, b]
related_ids: [other-ref]
some_custom_field: keep-me
---
body
`)

	_, err := fix.RunBackfill(fix.Config{
		VaultPath:       vault,
		Apply:           true,
		CreatedResolver: staticResolver("2024-01-01"),
	})
	require.NoError(t, err)

	content, err := os.ReadFile(notePath)
	require.NoError(t, err)
	s := string(content)
	assert.Contains(t, s, "title: My Special Title")
	assert.Contains(t, s, "some_custom_field: keep-me")
	// Tags preserved — YAML inline-sequence style is kept as-is by the
	// mutator (it doesn't re-format inline → block or vice versa).
	assert.Contains(t, s, "tags:")
	// Confirm both elements survive regardless of inline/block style.
	assert.Regexp(t, `tags:.*\ba\b`, s)
	assert.Regexp(t, `tags:.*\bb\b`, s)
}

// TestRunBackfill_FilesScannedReportsCorrectly — the result reports
// total files walked and notes-affected separately.
func TestRunBackfill_FilesScannedReportsCorrectly(t *testing.T) {
	vault := setupFixVault(t)
	// 2 domain notes (one needs fix, one complete); 1 non-domain.
	writeNote(t, vault, "needs-fix.md", "---\nid: a\ntype: reference\ntitle: A\n---\nbody\n")
	writeNote(t, vault, "complete.md", "---\nid: b\ntype: reference\ntitle: B\ncreated: 2024-01-01\nvm_updated: \"2024-01-01T00:00:00Z\"\n---\nbody\n")
	writeNote(t, vault, "stray.md", "---\nfoo: yes\n---\nbody\n")

	res, err := fix.RunBackfill(fix.Config{
		VaultPath:       vault,
		Apply:           false,
		CreatedResolver: staticResolver("2024-01-01"),
	})
	require.NoError(t, err)
	assert.Equal(t, 3, res.FilesScanned, "all .md files counted")
	assert.Len(t, res.Items, 1, "only the incomplete domain note is in items")
}

// TestDefaultCreatedDateResolver_UsesMtimeFallback — when git is not
// available (file not in a git repo), the default resolver falls back
// to file mtime. This pins the fallback chain for production callers
// that don't inject a custom resolver.
func TestDefaultCreatedDateResolver_UsesMtimeFallback(t *testing.T) {
	dir := t.TempDir() // not a git repo
	path := filepath.Join(dir, "n.md")
	require.NoError(t, os.WriteFile(path, []byte("body"), 0o644))

	value, source := fix.DefaultCreatedDateResolver(path)
	// Date-only YYYY-MM-DD format from mtime fallback.
	assert.Regexp(t, `^20\d{2}-\d{2}-\d{2}$`, value)
	// Source is "mtime" (not "git", since no git repo).
	assert.Contains(t, []string{"mtime", "today"}, source,
		"resolver fell back to mtime or today, not git")
	assert.NotContains(t, strings.ToLower(source), "git",
		"git path should not be claimed when no .git dir exists")
}
