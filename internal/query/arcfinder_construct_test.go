package query_test

import (
	"context"
	"testing"

	"os"
	"path/filepath"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These exercise the ONLY production construction path. The ranking tests build
// the struct directly, which left NewArcFinder at 0% coverage — and a mutation
// dropping its arc-type filter passed the entire suite. That regression would
// silently compare every proposal against principles, references and journal
// notes, so "resembles" would stop meaning "this is already an arc", which is
// the whole premise of the de-duplication aid.

// arcTestDB builds a tiny real vault holding one arc and one concept, indexes
// it, and embeds both with the SAME vector — so only the type filter can tell
// them apart. The shared fixture vault has no arc-typed notes, and mutating it
// would leak across the suite.
func arcTestDB(t *testing.T) (*index.DB, string, string) {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(body), 0o600))
	}
	write("arcs/a.md", "---\nid: arc-under-test\ntype: arc\ntitle: An Arc\ncreated: 2026-08-15\n---\n\nAn arc body.\n")
	write("concepts/c.md", "---\nid: concept-under-test\ntype: concept\ntitle: A Concept\ncreated: 2026-08-15\n---\n\nA concept body.\n")

	dbPath := filepath.Join(t.TempDir(), "idx.db")
	cfg, err := vault.LoadConfig(root)
	require.NoError(t, err)
	_, err = index.NewIndexer(root, dbPath, cfg).Rebuild()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, "arc-under-test", "concept-under-test"
}

func TestNewArcFinder_ComparesAgainstArcsOnly(t *testing.T) {
	db, arcID, otherID := arcTestDB(t)
	// Both aligned with the query, so ONLY the type filter can separate them.
	require.NoError(t, index.StoreEmbedding(db, arcID, []float32{1, 0, 0}))
	require.NoError(t, index.StoreEmbedding(db, otherID, []float32{1, 0, 0}))

	f, err := query.NewArcFinder(db, &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3})
	require.NoError(t, err)

	got, err := f.NearestArcs(context.Background(), "anything", 10)
	require.NoError(t, err)
	require.NotEmpty(t, got, "the embedded arc must be found")
	for _, m := range got {
		assert.NotEqual(t, otherID, m.ID,
			"a concept note is not an arc; resembling one says nothing about whether this is already recorded")
	}
}

// An un-embedded vault must fail with the remedy rather than answer "nothing
// resembles this" — which reads as "go ahead and draft it", the single mistake
// de-duplication exists to prevent.
func TestNewArcFinder_RefusesWithoutAnEmbedder(t *testing.T) {
	db, _, _ := arcTestDB(t)
	_, err := query.NewArcFinder(db, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--embed", "the error must carry the fix")
}

// A vault mid-migration holds vectors from two models. Cosine returns 0 for a
// dimension mismatch, and 0 printed beside real scores reads as "not similar"
// when the truth is "not comparable" — the same class as scores that were
// really fused ranks.
func TestNewArcFinder_MixedModelVaultDoesNotFabricateZeroScores(t *testing.T) {
	db, arcID, _ := arcTestDB(t)
	require.NoError(t, index.StoreEmbedding(db, arcID, []float32{1, 0, 0})) // 3-dim

	f, err := query.NewArcFinder(db, &mockEmbedder{vec: []float32{1, 0}, dims: 2}) // 2-dim query
	require.NoError(t, err)

	got, err := f.NearestArcs(context.Background(), "anything", 3)
	require.Error(t, err, "incomparable vectors must not be rendered as similarity scores")
	assert.Empty(t, got)
	assert.Contains(t, err.Error(), "different model")
}
