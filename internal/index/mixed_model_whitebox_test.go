package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildWhiteboxVault indexes a 3-note tempdir vault with no embeddings and returns
// its dbPath. Duplicated from the blackbox helper because embedResolved is
// unexported and must be exercised from package index.
func buildWhiteboxVault(t *testing.T) (idx *Indexer, dbPath string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"),
		[]byte("types:\n  concept:\n    required: [title]\n"), 0o644))
	for i, name := range []string{"alpha", "beta", "gamma"} {
		body := "---\nid: concept-" + name + "\ntype: concept\ntitle: " + name + "\n---\nbody " + string(rune('a'+i)) + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644))
	}
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath = filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	idx = NewIndexer(dir, dbPath, cfg)
	_, err = idx.Rebuild()
	require.NoError(t, err)
	return idx, dbPath
}

// TestEmbedResolved_FullPurgesThenReembeds proves the --full round-trip on a
// POPULATED, fully-embedded vault: embedResolved purges every existing embedding
// and re-embeds ALL notes at the new dimension, instead of the pending query
// skipping the wrong-dimension survivors into a mixed index. fakeEmbedder keeps it
// hermetic (no real model load). This is the load-bearing invariant of the fix —
// an incremental pass over the same vault must skip, a --full pass must rebuild.
func TestEmbedResolved_FullPurgesThenReembeds(t *testing.T) {
	idx, dbPath := buildWhiteboxVault(t)

	// Fully embed at 8-dim (a "prior model").
	r1, err := idx.EmbedNotes(context.Background(), dbPath, &fakeEmbedder{dims: 8})
	require.NoError(t, err)
	require.Equal(t, 3, r1.Embedded)

	// Incremental: already embedded -> skip, no purge.
	r2, err := idx.embedResolved(context.Background(), dbPath, &fakeEmbedder{dims: 8}, embedding.ModelMiniLM, false)
	require.NoError(t, err)
	assert.Equal(t, 0, r2.Embedded, "incremental must not re-embed already-embedded notes")
	assert.Equal(t, 3, r2.Skipped)

	// --full: purge every embedding, then re-embed ALL at a NEW dimension.
	r3, err := idx.embedResolved(context.Background(), dbPath, &fakeEmbedder{dims: 4}, embedding.ModelMiniLM, true)
	require.NoError(t, err)
	assert.Equal(t, 3, r3.Embedded, "--full must purge and re-embed every note")
	assert.Equal(t, 0, r3.Skipped)
	assert.Equal(t, embedding.ModelMiniLM, r3.Model, "result reports the model actually used")
}
