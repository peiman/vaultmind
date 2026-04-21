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

// Post-index passes (link resolution, alias mention detection, tag
// overlap) run AFTER the note-store transaction commits. Today their
// failures are log.Debug-and-continue: notes get stored but the
// links/edges pass silently failed, leaving a partially-connected graph
// the user can't distinguish from a fully-successful run.
//
// PostIndexWarnings surfaces each failure as a structured entry so
// downstream tooling and humans both see "edges weren't fully computed
// on this run" without having to enable debug logging.

func setupPostIndexFixture(t *testing.T) (vaultRoot, dbPath string, cfg *vault.Config) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte(`---
id: c-a
type: concept
title: Alpha
---
body
`), 0o644))
	cfgLoaded, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	return dir, filepath.Join(dir, "index.db"), cfgLoaded
}

// A clean vault must produce zero post-index warnings. Phantom warnings
// on success would noise up every CI run.
func TestRebuild_CleanVaultHasNoPostIndexWarnings(t *testing.T) {
	vaultRoot, dbPath, cfg := setupPostIndexFixture(t)
	result, err := index.NewIndexer(vaultRoot, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	assert.Empty(t, result.PostIndexWarnings)
}

func TestIncremental_CleanVaultHasNoPostIndexWarnings(t *testing.T) {
	vaultRoot, dbPath, cfg := setupPostIndexFixture(t)
	idxr := index.NewIndexer(vaultRoot, dbPath, cfg)
	_, err := idxr.Rebuild()
	require.NoError(t, err)
	result, err := idxr.Incremental()
	require.NoError(t, err)
	assert.Empty(t, result.PostIndexWarnings)
}

// The type PostIndexWarning carries a Step (so callers can filter by
// category) and an Error (the underlying message). Documented step
// values: "link_resolution", "alias_mention", "tag_overlap".
func TestPostIndexWarning_ShapeIsStepPlusError(t *testing.T) {
	w := index.PostIndexWarning{Step: "link_resolution", Error: "sql failure"}
	assert.Equal(t, "link_resolution", w.Step)
	assert.Equal(t, "sql failure", w.Error)
}
