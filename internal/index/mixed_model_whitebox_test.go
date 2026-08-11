package index

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingEmbedder always errors on its batch call — used to exercise the embed
// pass's failure handling without a real model.
type failingEmbedder struct{}

func (failingEmbedder) Embed(context.Context, string) ([]float32, error) {
	return nil, errors.New("embed boom")
}
func (failingEmbedder) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("embed boom")
}
func (failingEmbedder) Dims() int    { return 8 }
func (failingEmbedder) Close() error { return nil }

// emptyFullEmbedder is a FullEmbedder (bge-m3 shape) that returns dense but EMPTY
// sparse/colbert, so EmbedNotes counts every note as EmptyOutput (not Errors, not
// Embedded) — the "all notes produce nothing usable" total-wipe edge.
type emptyFullEmbedder struct{}

func (emptyFullEmbedder) Embed(context.Context, string) ([]float32, error) {
	return make([]float32, 8), nil
}
func (emptyFullEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, 8)
	}
	return out, nil
}
func (emptyFullEmbedder) Dims() int    { return 8 }
func (emptyFullEmbedder) Close() error { return nil }
func (emptyFullEmbedder) EmbedFullBatch(_ context.Context, texts []string) ([]*embedding.BGEM3Output, error) {
	out := make([]*embedding.BGEM3Output, len(texts))
	for i := range out {
		out[i] = &embedding.BGEM3Output{Dense: make([]float32, 8), Sparse: nil, ColBERT: nil}
	}
	return out, nil
}

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

// TestEmbedResolved_FullFailsLoudWhenReembedFails: a --full run purges every
// embedding first, so if the re-embed then fails, EmbedNotes' failure-tolerant
// nil-error would otherwise leave the CLI reporting success (exit 0) with a wiped
// index. embedResolved must convert that into a hard error.
func TestEmbedResolved_FullFailsLoudWhenReembedFails(t *testing.T) {
	idx, dbPath := buildWhiteboxVault(t)

	// Seed embeddings so the purge has something real to clear.
	r1, err := idx.EmbedNotes(context.Background(), dbPath, &fakeEmbedder{dims: 8})
	require.NoError(t, err)
	require.Equal(t, 3, r1.Embedded)

	_, err = idx.embedResolved(context.Background(), dbPath, failingEmbedder{}, embedding.ModelMiniLM, true)
	require.Error(t, err, "--full whose re-embed fails must not report success with a purged index")
	assert.Contains(t, err.Error(), "purged", "the error must state the index was purged")
}

// TestEmbedResolved_FullFailsLoudWhenAllOutputEmpty covers the residual total-wipe
// path: a --full run whose re-embed produces only empty-modality output (Errors==0,
// Embedded==0, EmptyOutput>0) purged real embeddings yet stored none, so it must
// still fail loud rather than exit 0 with an empty index.
func TestEmbedResolved_FullFailsLoudWhenAllOutputEmpty(t *testing.T) {
	idx, dbPath := buildWhiteboxVault(t)

	// Seed embeddings so the purge clears real rows (purged > 0).
	r1, err := idx.EmbedNotes(context.Background(), dbPath, &fakeEmbedder{dims: 8})
	require.NoError(t, err)
	require.Equal(t, 3, r1.Embedded)

	res, err := idx.embedResolved(context.Background(), dbPath, emptyFullEmbedder{}, embedding.ModelBGEM3, true)
	require.Error(t, err, "--full that purged then re-embedded zero notes must fail loud")
	assert.Contains(t, err.Error(), "purged")
	assert.Equal(t, 0, res.Embedded)
}

// TestEmbedResolved_IncrementalReembedFailureIsNotLoud: an INCREMENTAL run does
// not purge, so a failed embed is non-destructive (pending notes simply stay
// pending). It must NOT be converted into a hard error — that would change
// long-standing, non-destructive behavior.
func TestEmbedResolved_IncrementalReembedFailureIsNotLoud(t *testing.T) {
	idx, dbPath := buildWhiteboxVault(t) // fresh vault: all notes pending

	res, err := idx.embedResolved(context.Background(), dbPath, failingEmbedder{}, embedding.ModelMiniLM, false)
	require.NoError(t, err, "an incremental embed failure is non-destructive and must stay soft")
	assert.Greater(t, res.Errors, 0, "the failure is still counted in the result")
}
