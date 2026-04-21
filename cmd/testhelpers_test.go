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
