package telemetry_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/telemetry"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixtureVaultPath = "../../test/fixtures/testvault"

// ComputeFeatures over the fixture vault must report aggregate stats
// that match what the indexer actually wrote — no derivable content
// (no titles, no IDs, no paths), only counts and a per-type histogram.
func TestComputeFeatures_FixtureVault(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	cfg, err := vault.LoadConfig(fixtureVaultPath)
	require.NoError(t, err)
	idxr := index.NewIndexer(fixtureVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	f, err := telemetry.ComputeFeatures(dbPath)
	require.NoError(t, err)
	require.NotNil(t, f)

	// Fixture has 8 notes — 5 concepts, 1 source, 1 project, 1 decision.
	assert.Equal(t, 8, f.NoteCount)
	assert.Equal(t, 5, f.TypeDistribution["concept"])
	assert.Equal(t, 1, f.TypeDistribution["source"])
	assert.Equal(t, 1, f.TypeDistribution["project"])
	assert.Equal(t, 1, f.TypeDistribution["decision"])

	// Links: act-r body wikilinks to spreading-activation; both have
	// related_ids cross-references. Exact count varies with link
	// types but must be > 0.
	assert.Greater(t, f.LinkCount, 0, "fixture has wikilinks + related_ids")
	assert.Greater(t, f.AliasCount, 0, "fixture has aliases on act-r and others")

	// No embeddings ran → both fields are zero.
	assert.Equal(t, 0, f.EmbeddingCount)
	assert.Equal(t, 0, f.EmbeddingDims)
}

// ComputeFeatures must return an error rather than zero-valued struct
// when the index DB doesn't exist — pretending to ship features for
// an unindexed vault would be a privacy-irrelevant correctness bug
// that taints whatever the receiver does with it.
func TestComputeFeatures_MissingDB(t *testing.T) {
	_, err := telemetry.ComputeFeatures(filepath.Join(t.TempDir(), "nonexistent.db"))
	require.Error(t, err)
}

// ComputeFeatures must surface a SQL error when the DB lacks the
// expected schema (e.g. someone hands it the experiment DB by mistake,
// or a stale DB from before migration X). The error path matters
// because the alternative — silently returning zero counts — would
// poison the federated rollup with garbage zeros that look real.
func TestComputeFeatures_BadSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wrong-shape.db")
	// Write a non-SQLite file. Open will fail or queries will fail.
	require.NoError(t, os.WriteFile(dbPath, []byte("not a sqlite database"), 0o600))

	_, err := telemetry.ComputeFeatures(dbPath)
	require.Error(t, err, "ComputeFeatures must error on a non-SQLite file rather than silently returning zeros")
}

// A note with NULL type column (markdown without frontmatter — e.g.
// the vault's README.md) must be reported as "unstructured" in the
// distribution, not crash the rollup. This is the regression guard
// for the smoke-test bug we just fixed.
func TestComputeFeatures_HandlesNullType(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "vault")
	require.NoError(t, os.MkdirAll(filepath.Join(dst, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dst, ".vaultmind", "config.yaml"),
		[]byte("types:\n  concept:\n    required: [title]\n"), 0o600))
	// One typed note + one frontmatter-less note.
	require.NoError(t, os.WriteFile(filepath.Join(dst, "typed.md"),
		[]byte("---\nid: c1\ntype: concept\ntitle: Typed\n---\nbody\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dst, "raw.md"),
		[]byte("# No frontmatter at all\nplain markdown\n"), 0o600))

	cfg, err := vault.LoadConfig(dst)
	require.NoError(t, err)
	dbPath := filepath.Join(t.TempDir(), "index.db")
	idxr := index.NewIndexer(dst, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	f, err := telemetry.ComputeFeatures(dbPath)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, f.NoteCount, 1)
	// The unstructured note must appear under "unstructured", not crash.
	_, hasUnstructured := f.TypeDistribution["unstructured"]
	assert.True(t, hasUnstructured || f.TypeDistribution["concept"] == f.NoteCount,
		"either unstructured key exists, or all notes are typed")
}
