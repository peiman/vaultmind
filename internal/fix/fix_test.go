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

// vaultmind frontmatter fix backfill tests. After the 2026-05-04
// chain retraction (vm_updated retired entirely; only `created`
// survives as a tolerated optional field), the command's contract is:
// audit domain notes for missing `created` and optionally write it
// via the mutator. Default mode is dry-run; --apply is opt-in. Per
// the extend-don't-overwrite principle, vaultmind never silently rewrites
// user files.

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

// staticResolver returns a fixed value — used by tests so we don't
// depend on the test environment's filesystem mtime or git history.
func staticResolver(date string) fix.CreatedDateResolver {
	return func(_ string) (string, string) { return date, "test" }
}

// TestRunBackfill_NoteMissingCreated — domain note lacks `created`.
// Dry-run reports it; apply writes it; existing user fields untouched.
func TestRunBackfill_NoteMissingCreated(t *testing.T) {
	vault := setupFixVault(t)
	notePath := writeNote(t, vault, "n.md", `---
id: ref-1
type: reference
title: T
---
body
`)

	// Dry-run: report only, no write.
	res, err := fix.RunBackfill(fix.Config{
		VaultPath:       vault,
		Apply:           false,
		CreatedResolver: staticResolver("2024-01-01"),
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	item := res.Items[0]
	assert.Equal(t, []string{"created"}, item.MissingFields)
	assert.Equal(t, "2024-01-01", item.ProposedValues["created"])
	assert.Equal(t, "test", item.Sources["created"])

	dryContent, _ := os.ReadFile(notePath)
	assert.NotContains(t, string(dryContent), "created:",
		"dry-run must NOT modify the file")

	// Apply: writes the field.
	res2, err := fix.RunBackfill(fix.Config{
		VaultPath:       vault,
		Apply:           true,
		CreatedResolver: staticResolver("2024-01-01"),
	})
	require.NoError(t, err)
	require.Len(t, res2.Items, 1)

	applied, _ := os.ReadFile(notePath)
	s := string(applied)
	assert.Regexp(t, `created: "?2024-01-01"?`, s)
}

// TestRunBackfill_NoteWithCreated — note already has `created`; no
// item, no write.
func TestRunBackfill_NoteWithCreated(t *testing.T) {
	vault := setupFixVault(t)
	writeNote(t, vault, "n.md", `---
id: ref-1
type: reference
title: T
created: 2023-06-15
---
body
`)
	res, err := fix.RunBackfill(fix.Config{
		VaultPath:       vault,
		Apply:           false,
		CreatedResolver: staticResolver("2024-01-01"),
	})
	require.NoError(t, err)
	assert.Empty(t, res.Items, "note with created already present needs no backfill")
}

// TestRunBackfill_NonDomainNoteSkipped — non-domain notes (no id+type)
// are skipped entirely. Vaultmind doesn't track non-domain content.
func TestRunBackfill_NonDomainNoteSkipped(t *testing.T) {
	vault := setupFixVault(t)
	notePath := writeNote(t, vault, "README.md", `---
title: Just a Readme
---
content
`)
	res, err := fix.RunBackfill(fix.Config{
		VaultPath:       vault,
		Apply:           true,
		CreatedResolver: staticResolver("2024-01-01"),
	})
	require.NoError(t, err)
	assert.Empty(t, res.Items, "non-domain note skipped")

	content, _ := os.ReadFile(notePath)
	assert.NotContains(t, string(content), "created:",
		"non-domain file must remain untouched")
}

// TestRunBackfill_DryRunIncludesDiff — dry-run output includes the
// per-note diff so the user can audit what would change before applying.
func TestRunBackfill_DryRunIncludesDiff(t *testing.T) {
	vault := setupFixVault(t)
	writeNote(t, vault, "n.md", `---
id: ref-1
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
	assert.NotEmpty(t, res.Items[0].Diff, "dry-run includes diff preview")
	assert.Contains(t, res.Items[0].Diff, "+created:")
}

// TestRunBackfill_PreservesUserOwnedFields — title, status, custom
// fields, tags are NEVER touched by fix. Pins the contract.
func TestRunBackfill_PreservesUserOwnedFields(t *testing.T) {
	vault := setupFixVault(t)
	notePath := writeNote(t, vault, "n.md", `---
id: ref-1
type: reference
title: My Title
status: active
custom_field: user value
tags:
  - a
  - b
---
body content
`)
	_, err := fix.RunBackfill(fix.Config{
		VaultPath:       vault,
		Apply:           true,
		CreatedResolver: staticResolver("2024-01-01"),
	})
	require.NoError(t, err)

	content, _ := os.ReadFile(notePath)
	s := string(content)
	assert.Contains(t, s, `title: My Title`)
	assert.Contains(t, s, `status: active`)
	assert.Contains(t, s, `custom_field: user value`)
	assert.Contains(t, s, "tags:")
	assert.Contains(t, s, "- a")
	assert.Contains(t, s, "- b")
	assert.Contains(t, s, "body content")
}

// TestRunBackfill_FilesScannedReportsCorrectly — FilesScanned counts
// every .md file walked; Items only the affected ones.
func TestRunBackfill_FilesScannedReportsCorrectly(t *testing.T) {
	vault := setupFixVault(t)
	writeNote(t, vault, "domain-incomplete.md", `---
id: ref-1
type: reference
title: T
---
`)
	writeNote(t, vault, "domain-complete.md", `---
id: ref-2
type: reference
title: T2
created: 2023-01-01
---
`)
	writeNote(t, vault, "non-domain.md", `---
title: Just a Note
---
`)

	res, err := fix.RunBackfill(fix.Config{
		VaultPath:       vault,
		Apply:           false,
		CreatedResolver: staticResolver("2024-01-01"),
	})
	require.NoError(t, err)
	assert.Equal(t, 3, res.FilesScanned, "all .md files counted as scanned")
	assert.Len(t, res.Items, 1, "only the incomplete domain note is in items")
}

// TestDefaultCreatedDateResolver_UsesMtimeFallback — when git is not
// available (file not in a git repo), the default resolver falls back
// to file mtime. Pins the fallback chain for production callers that
// don't inject a custom resolver.
func TestDefaultCreatedDateResolver_UsesMtimeFallback(t *testing.T) {
	dir := t.TempDir() // not a git repo
	path := filepath.Join(dir, "n.md")
	require.NoError(t, os.WriteFile(path, []byte("body"), 0o644))

	value, source := fix.DefaultCreatedDateResolver(path)
	assert.Regexp(t, `^20\d{2}-\d{2}-\d{2}$`, value)
	assert.Contains(t, []string{"mtime", "today"}, source,
		"resolver fell back to mtime or today, not git")
	assert.NotContains(t, strings.ToLower(source), "git",
		"git path should not be claimed when no .git dir exists")
}

// TestRunBackfill_CorruptFrontmatterCapturedAndContinues — when one
// note has malformed YAML frontmatter, RunBackfill captures the error
// in that Item.Error and CONTINUES the walk, producing valid Items
// for the remaining domain notes. Pins the //nolint:nilerr
// suppressions in fix.go (principle 1: surface partial failures
// rather than silently dropping data).
func TestRunBackfill_CorruptFrontmatterCapturedAndContinues(t *testing.T) {
	vault := setupFixVault(t)
	corruptPath := writeNote(t, vault, "corrupt.md", `---
id: bad
type: reference
title: T
  this: is: not: valid: yaml: [
---
body
`)
	writeNote(t, vault, "valid.md", `---
id: ref-good
type: reference
title: Good
---
body
`)

	res, err := fix.RunBackfill(fix.Config{
		VaultPath:       vault,
		Apply:           false,
		CreatedResolver: staticResolver("2024-01-01"),
	})
	require.NoError(t, err, "walk-level error must NOT abort on per-note failures")
	require.NotNil(t, res)

	var corruptItem, validItem *fix.Item
	for i := range res.Items {
		switch res.Items[i].Path {
		case filepath.Base(corruptPath):
			corruptItem = &res.Items[i]
		case "valid.md":
			validItem = &res.Items[i]
		}
	}
	require.NotNil(t, corruptItem, "corrupt note must surface in items with Error")
	assert.NotEmpty(t, corruptItem.Error)
	require.NotNil(t, validItem, "valid note must still be processed")
	assert.Empty(t, validItem.Error)
	assert.Contains(t, validItem.MissingFields, "created")
	assert.Equal(t, schema.CreatedDateFormat, "2006-01-02",
		"sanity: SSOT constant unchanged")
}
