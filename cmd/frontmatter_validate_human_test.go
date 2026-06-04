package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// frontmatter validate in non-JSON (human) mode emits a "Checked N files"
// summary line followed by one line per issue. Terminal users rely on this
// output; without it, running `frontmatter validate` silently would look
// identical whether the vault is clean or broken.
func TestFrontmatterValidate_HumanOutputHeaderAndIssues(t *testing.T) {
	vault := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(vault, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(vault, ".vaultmind", "config.yaml"), []byte(`
types:
  source:
    required: [url]
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(vault, "bad.md"), []byte(`---
id: s-1
type: source
---
body
`), 0o644))

	// --live so we don't need the index DB
	out, _, err := runRootCmd(t, "frontmatter", "validate", "--vault", vault, "--live")
	require.NoError(t, err)
	text := out.String()

	assert.Contains(t, text, "Checked 1 files",
		"human mode must emit a 'Checked N files' header line")
	assert.Contains(t, text, "missing_required_field",
		"each issue must appear as a line — scripts tail this")
	assert.Contains(t, text, "[error]",
		"severity in brackets is part of the machine-parseable shape")
}

// frontmatter validate in human mode on a clean vault emits "0 issues".
// Regression: a formatter that skipped the header line on the clean path
// would make "did the validator run?" ambiguous.
func TestFrontmatterValidate_HumanOutputCleanVault(t *testing.T) {
	vault := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(vault, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(vault, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(vault, "ok.md"), []byte(`---
id: c-1
type: concept
title: Fine
---
body
`), 0o644))

	out, _, err := runRootCmd(t, "frontmatter", "validate", "--vault", vault, "--live")
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Checked 1 files")
	assert.Contains(t, text, "0 issues",
		"clean vault must report '0 issues' explicitly — silence would be ambiguous")
}

// Non-live path (default) human-mode output on the indexed vault. Covers
// the runValidation DB-backed branch + the human formatter together.
func TestFrontmatterValidate_NonLiveHumanOutput(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "frontmatter", "validate", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Checked",
		"default (non-live) mode must produce the same header shape")
}
