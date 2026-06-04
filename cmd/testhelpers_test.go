package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
)

// buildIndexedTestVault creates a tempdir vault with a minimal config and
// a handful of linked notes, then runs the indexer once. Returns the vault
// root path. Tests that need query/search/links/status use this helper so
// each test targets behavior, not scaffolding.
//
// Vault shape:
//
//	concepts/alpha.md     type=concept, links to project beta
//	concepts/gamma.md     type=concept, no links
//	projects/beta.md      type=project status=active, links to concept alpha
//	unstructured.md       no id/type (should be ignored by domain queries)
func buildIndexedTestVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  project:
    required: [status, title]
    optional: [owner_id, related_ids]
    statuses: [active, paused, completed]
  concept:
    required: [title]
    optional: [tags, related_ids]
`), 0o644))

	writeTestNote(t, dir, "concepts/alpha.md", `---
id: concept-alpha
type: concept
title: Alpha Concept
tags: [foundational]
related_ids: [proj-beta]
---
Alpha body. See [[proj-beta|Beta]] for context.
`)
	writeTestNote(t, dir, "concepts/gamma.md", `---
id: concept-gamma
type: concept
title: Gamma Concept
---
Gamma body, no links.
`)
	writeTestNote(t, dir, "projects/beta.md", `---
id: proj-beta
type: project
title: Beta Project
status: active
related_ids: [concept-alpha]
---
Beta body. References [[concept-alpha]].
`)
	writeTestNote(t, dir, "unstructured.md", `---
title: Not a domain note
---
Just a scratch note.
`)

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))

	idxr := index.NewIndexer(dir, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err, "indexer rebuild failed")

	return dir
}

func writeTestNote(t *testing.T, vaultRoot, relPath, content string) {
	t.Helper()
	full := filepath.Join(vaultRoot, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// copyBaselineVaultToTemp copies the committed baseline fixture vault into
// a tempdir so tests can index it without mutating the source tree. The
// committed fixture has no index.db (SQLite files aren't committed); tests
// that drive CLI commands must build one, and the tempdir copy is the
// isolation boundary so concurrent tests don't race on the same DB and
// the committed fixture stays immutable.
func copyBaselineVaultToTemp(t *testing.T) string {
	t.Helper()
	src := "../test/fixtures/baseline/vault"
	dst := t.TempDir()

	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		// Skip any stray index.db from previous dirty runs — the tempdir
		// copy must start with no index so the test-controlled rebuild is
		// the only index present.
		if filepath.Base(path) == "index.db" {
			return nil
		}
		content, err := os.ReadFile(path) //nolint:gosec // fixture path under testdata
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, 0o644)
	})
	require.NoError(t, err)
	return dst
}

// indexedBaselineVault returns a tempdir path containing the baseline
// fixture vault with a freshly built index.db. Use this in contract tests
// or any CLI-level test that needs the full baseline content indexed.
func indexedBaselineVault(t *testing.T) string {
	t.Helper()
	dir := copyBaselineVaultToTemp(t)
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	return dir
}

// runRootCmd executes RootCmd with the given args and returns stdout + stderr
// as separate buffers, plus the execute error.
//
// Cobra persists flag values across Execute() calls — a flag set in one test
// leaks into the next. Tests that rely on flag defaults would pick up the
// previous test's value. resetFlagsRecursive walks the command tree once
// before each run so every test sees defaults.
func runRootCmd(t *testing.T, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	resetFlagsRecursive(RootCmd)
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	RootCmd.SetOut(out)
	RootCmd.SetErr(errOut)
	RootCmd.SetArgs(args)
	defer RootCmd.SetArgs(nil)
	err := RootCmd.Execute()
	return out, errOut, err
}

// resetFlagsRecursive walks cmd and all descendants, resetting every flag to
// its declared default. Covers persistent and local flags.
func resetFlagsRecursive(cmd *cobra.Command) {
	reset := func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	}
	cmd.Flags().VisitAll(reset)
	cmd.PersistentFlags().VisitAll(reset)
	for _, c := range cmd.Commands() {
		resetFlagsRecursive(c)
	}
}
