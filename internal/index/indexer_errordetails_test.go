package index_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Manifesto principle #3 — Good Will — says every silent failure is a lost
// memory. Today Rebuild counts per-file errors in result.Errors but hides
// WHICH files failed and WHY: a user running `vaultmind index` sees
// "Errors: 2" and has no way to investigate without flipping on debug
// logging. These tests pin the contract that ErrorDetails carries the
// specifics so users can act on partial-index failures.

func writeFixtureNote(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// Rebuild on a vault with one unparseable YAML file: the good files
// index, the bad file's failure is counted AND detailed. A naked count
// without details is the exact thing the manifesto warns against.
func TestRebuild_UnparseableFileSurfacesInErrorDetails(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))
	writeFixtureNote(t, dir, "good.md", `---
id: c-good
type: concept
title: Good Note
---
body
`)
	// Malformed YAML: unterminated bracket
	writeFixtureNote(t, dir, "bad.md", `---
id: c-bad
type: concept
title: Bad
tags: [unterminated
---
body
`)

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, "index.db")
	result, err := index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err, "Rebuild must complete despite per-file failures")

	assert.Equal(t, 1, result.Errors, "one file failed to parse")
	require.Len(t, result.ErrorDetails, 1,
		"parse failure must surface in ErrorDetails so users can see WHICH file broke")
	assert.Equal(t, "bad.md", result.ErrorDetails[0].Path)
	assert.Equal(t, "parse", result.ErrorDetails[0].Kind,
		"kind must name the specific failure class — parse vs read vs store affect the fix")
	assert.NotEmpty(t, result.ErrorDetails[0].Error,
		"error string must be present so the user can read the actual YAML parser message")
}

// Incremental on a vault with an unparseable file: same contract. The
// incremental path is symmetric to Rebuild — silent there would mean
// re-indexing a broken file in CI never tells you what's wrong.
func TestIncremental_UnparseableFileSurfacesInErrorDetails(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))
	writeFixtureNote(t, dir, "good.md", `---
id: c-good
type: concept
title: Good
---
body
`)

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, "index.db")
	idxr := index.NewIndexer(dir, dbPath, cfg)
	_, err = idxr.Rebuild() // seed the index
	require.NoError(t, err)

	// Now add a malformed file and run incremental
	writeFixtureNote(t, dir, "bad.md", `---
id: c-bad
type: concept
tags: [unterminated
---
`)
	result, err := idxr.Incremental()
	require.NoError(t, err)
	assert.Equal(t, 1, result.Errors)
	require.Len(t, result.ErrorDetails, 1,
		"incremental must also populate ErrorDetails — symmetric with Rebuild")
	assert.Equal(t, "bad.md", result.ErrorDetails[0].Path)
	assert.Equal(t, "parse", result.ErrorDetails[0].Kind)
}

// A clean vault must produce an empty (or nil) ErrorDetails — a populated
// slice on success would cause JSON consumers to branch incorrectly.
func TestRebuild_CleanVaultHasNoErrorDetails(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))
	writeFixtureNote(t, dir, "ok.md", `---
id: c-ok
type: concept
title: OK
---
body
`)
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, "index.db")
	result, err := index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	assert.Equal(t, 0, result.Errors)
	assert.Empty(t, result.ErrorDetails)
}
