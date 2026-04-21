package index_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDenseEmbedder returns a deterministic float32 vector for any input.
// EmbedNotes only cares about the dense-only (non-FullEmbedder) path when
// given this type.
type fakeDenseEmbedder struct {
	dims int
}

func (f fakeDenseEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	out := make([]float32, f.dims)
	for i := range out {
		out[i] = float32(len(text) + i)
	}
	return out, nil
}

func (f fakeDenseEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v, _ := f.Embed(ctx, t)
		out[i] = v
	}
	return out, nil
}

func (f fakeDenseEmbedder) Dims() int    { return f.dims }
func (f fakeDenseEmbedder) Close() error { return nil }

var _ embedding.Embedder = fakeDenseEmbedder{}

// buildEmbedTestVault returns the dbPath of a freshly indexed tempdir vault
// with three domain notes, no embeddings yet.
func buildEmbedTestVault(t *testing.T) (vaultRoot, dbPath string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))
	for i, name := range []string{"alpha", "beta", "gamma"} {
		content := `---
id: concept-` + name + `
type: concept
title: ` + name + `
---
body ` + string(rune('a'+i)) + `
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".md"), []byte(content), 0o644))
	}
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath = filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	idxr := index.NewIndexer(dir, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)
	return dir, dbPath
}

// EmbedNotes on a freshly indexed vault embeds every note and stores the
// vectors. Running it a second time must skip every one — that's the
// "don't re-embed what's already embedded" contract users (and costs)
// depend on.
func TestEmbedNotes_EmbedsThenSkipsOnRerun(t *testing.T) {
	vaultRoot, dbPath := buildEmbedTestVault(t)
	cfg, err := vault.LoadConfig(vaultRoot)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultRoot, dbPath, cfg)
	emb := fakeDenseEmbedder{dims: 8}

	ctx := context.Background()
	r1, err := idxr.EmbedNotes(ctx, dbPath, emb)
	require.NoError(t, err)
	assert.Equal(t, 3, r1.Embedded, "all three notes must be embedded on first pass")
	assert.Equal(t, 0, r1.Errors)

	r2, err := idxr.EmbedNotes(ctx, dbPath, emb)
	require.NoError(t, err)
	assert.Equal(t, 0, r2.Embedded, "second pass must not re-embed already-embedded notes")
	assert.Equal(t, 3, r2.Skipped, "all three must be counted as skipped")
}

// After EmbedNotes runs, HasEmbeddings must report true — this is the
// signal BuildAutoRetrieverFull reads to decide whether to wire the hybrid
// retriever or fall back to keyword.
func TestEmbedNotes_MarksHasEmbeddingsTrue(t *testing.T) {
	vaultRoot, dbPath := buildEmbedTestVault(t)
	cfg, err := vault.LoadConfig(vaultRoot)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultRoot, dbPath, cfg)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	has, err := index.HasEmbeddings(db)
	require.NoError(t, err)
	assert.False(t, has, "pre-embed: HasEmbeddings is false")
	require.NoError(t, db.Close())

	_, err = idxr.EmbedNotes(context.Background(), dbPath, fakeDenseEmbedder{dims: 4})
	require.NoError(t, err)

	db, err = index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()
	has, err = index.HasEmbeddings(db)
	require.NoError(t, err)
	assert.True(t, has, "post-embed: HasEmbeddings must flip to true")
}
