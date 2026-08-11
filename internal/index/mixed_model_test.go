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

// TestRunEmbed_FailsClosed_OnExplicitModelDimMismatch is the core mixed-model
// guard: an INCREMENTAL embed that asks for a model whose dimension differs from
// the vault's existing embeddings must fail closed instead of silently producing
// a mixed-dimension index. The prior guard only caught the bge-m3->minilm
// FALLBACK; an explicit `--model X` walked straight past it.
func TestRunEmbed_FailsClosed_OnExplicitModelDimMismatch(t *testing.T) {
	vaultRoot, dbPath := buildEmbedTestVault(t)
	cfg, err := vault.LoadConfig(vaultRoot)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultRoot, dbPath, cfg)

	// Seed the vault with 384-dim (MiniLM-shaped) dense embeddings.
	r, err := idxr.EmbedNotes(context.Background(), dbPath, fakeDenseEmbedder{dims: embedding.DefaultDims})
	require.NoError(t, err)
	require.Equal(t, 3, r.Embedded)

	// Ask, incrementally, for bge-m3 (1024-dim). Must refuse before loading any
	// embedder — the guard runs on the lazy path, so no model files are needed.
	_, err = idxr.RunEmbed(context.Background(), dbPath, embedding.ModelBGEM3, false)
	require.Error(t, err, "explicit model change to a different dimension must fail closed")
	msg := err.Error()
	assert.Contains(t, msg, "--full", "error must point at the --full remedy")
	assert.Contains(t, msg, "mixed-model", "error must name the failure it prevents")
	assert.Contains(t, msg, embedding.ModelBGEM3, "error must name the requested model")
}

// TestRunEmbed_AllowsSameDimensionIncremental proves the guard does not false-
// positive on the normal re-index path: same model dimension proceeds, and with
// everything already embedded it takes the lazy-skip path (no model load).
func TestRunEmbed_AllowsSameDimensionIncremental(t *testing.T) {
	vaultRoot, dbPath := buildEmbedTestVault(t)
	cfg, err := vault.LoadConfig(vaultRoot)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultRoot, dbPath, cfg)

	r, err := idxr.EmbedNotes(context.Background(), dbPath, fakeDenseEmbedder{dims: embedding.DefaultDims})
	require.NoError(t, err)
	require.Equal(t, 3, r.Embedded)

	// minilm (384) == stored dim -> guard passes; nothing pending -> lazy skip.
	res, err := idxr.RunEmbed(context.Background(), dbPath, embedding.ModelMiniLM, false)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Embedded)
	assert.Equal(t, 3, res.Skipped)
}

// TestPurgeEmbeddings_ClearsAndAllowsReembed models what --full does: purge every
// stored embedding so a subsequent pass re-embeds ALL notes under the new model,
// rather than skipping the wrong-dimension survivors into a mixed index.
func TestPurgeEmbeddings_ClearsAndAllowsReembed(t *testing.T) {
	vaultRoot, dbPath := buildEmbedTestVault(t)
	cfg, err := vault.LoadConfig(vaultRoot)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultRoot, dbPath, cfg)

	// Seed 384-dim embeddings (a prior model).
	r, err := idxr.EmbedNotes(context.Background(), dbPath, fakeDenseEmbedder{dims: embedding.DefaultDims})
	require.NoError(t, err)
	require.Equal(t, 3, r.Embedded)

	n, err := index.PurgeEmbeddings(dbPath)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n, "purge clears every stored embedding")

	// A second purge clears nothing — proves the first actually nulled them.
	n2, err := index.PurgeEmbeddings(dbPath)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n2)

	// Re-embed under a DIFFERENT dimension: all three come back (none skipped),
	// so the vault ends uniformly at the new dimension — not a mix.
	r2, err := idxr.EmbedNotes(context.Background(), dbPath, fakeDenseEmbedder{dims: 8})
	require.NoError(t, err)
	assert.Equal(t, 3, r2.Embedded, "every note re-embeds after a purge")
	assert.Equal(t, 0, r2.Skipped)
}

// TestRunEmbed_FullOnEmptyVault exercises RunEmbed's --full/purge branch without
// a model load: an empty vault purges nothing, has nothing pending, and returns
// cleanly (the lazy-load short-circuit still fires after the purge).
func TestRunEmbed_FullOnEmptyVault(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"),
		[]byte("types:\n  concept:\n    required: [title]\n"), 0o644))
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	idxr := index.NewIndexer(dir, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	res, err := idxr.RunEmbed(context.Background(), dbPath, embedding.ModelBGEM3, true)
	require.NoError(t, err, "full re-embed of an empty vault must not load a model")
	assert.Equal(t, 0, res.Embedded)
}

// TestPurgeEmbeddings_HandlesBGEM3ShapedRows guards the single-UPDATE purge (G2)
// against the migration-006 modality-parity trigger, which aborts an UPDATE that
// leaves a 4096-byte bge-m3 dense without sparse+colbert. Every other test seeds
// dense-only, so this is the only one that produces real bge-m3-shaped rows and
// proves purging them clears all three columns at once without tripping RAISE(ABORT).
func TestPurgeEmbeddings_HandlesBGEM3ShapedRows(t *testing.T) {
	_, dbPath := buildEmbedTestVault(t)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	dense := index.EncodeEmbedding(make([]float32, embedding.BGEM3Dims)) // 4096 bytes = the trigger threshold
	sparse := index.EncodeSparseEmbedding(map[int32]float32{1: 0.5})
	colbert := index.EncodeColBERTEmbedding([][]float32{{0.1, 0.2}})
	_, err = db.Exec(`UPDATE notes SET embedding = ?, sparse_embedding = ?, colbert_embedding = ?`,
		dense, sparse, colbert)
	require.NoError(t, err, "seeding bge-m3-shaped rows must itself satisfy the parity trigger")
	require.NoError(t, db.Close())

	n, err := index.PurgeEmbeddings(dbPath)
	require.NoError(t, err, "purging bge-m3-shaped rows must not trip the modality-parity trigger")
	assert.Equal(t, int64(3), n)
}

// TestRunEmbed_FailsClosed_OnUnknownStoredDimension: a vault whose stored
// embeddings are an unrecognized dimension (neither minilm's 384 nor bge-m3's
// 1024) still fails closed on a model switch, naming the raw dimension since no
// model token maps to it.
func TestRunEmbed_FailsClosed_OnUnknownStoredDimension(t *testing.T) {
	vaultRoot, dbPath := buildEmbedTestVault(t)
	cfg, err := vault.LoadConfig(vaultRoot)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultRoot, dbPath, cfg)

	// Seed 8-dim embeddings — a dimension no known model produces.
	r, err := idxr.EmbedNotes(context.Background(), dbPath, fakeDenseEmbedder{dims: 8})
	require.NoError(t, err)
	require.Equal(t, 3, r.Embedded)

	_, err = idxr.RunEmbed(context.Background(), dbPath, embedding.ModelMiniLM, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "8-dimension", "an unrecognized stored dim is named by its raw size")
}

// TestPurgeEmbeddings_OpenError: PurgeEmbeddings surfaces a clear error when the
// index DB can't be opened (a db path under a regular file can't have its parent
// directory created).
func TestPurgeEmbeddings_OpenError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	_, err := index.PurgeEmbeddings(filepath.Join(blocker, "index.db"))
	require.Error(t, err, "purge must error when the index DB can't be opened")
}

// TestRunEmbed_SurfacesGuardOpenError: an incremental RunEmbed whose dimension
// guard can't open the DB returns the error rather than proceeding.
func TestRunEmbed_SurfacesGuardOpenError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	badDB := filepath.Join(blocker, "index.db")
	idxr := index.NewIndexer(t.TempDir(), badDB, &vault.Config{})
	_, err := idxr.RunEmbed(context.Background(), badDB, embedding.ModelMiniLM, false)
	require.Error(t, err)
}

// TestRunEmbed_RejectsUnknownModel: an unrecognized model token is refused up
// front. loadEmbedder would otherwise silently coerce anything != bge-m3 to
// minilm, so a typo'd --model could write 384-dim vectors into a bge-m3 vault (a
// mixed index under a bogus model name) — exactly what this fix exists to prevent.
func TestRunEmbed_RejectsUnknownModel(t *testing.T) {
	vaultRoot, dbPath := buildEmbedTestVault(t)
	cfg, err := vault.LoadConfig(vaultRoot)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultRoot, dbPath, cfg)

	_, err = idxr.RunEmbed(context.Background(), dbPath, "some-future-model", false)
	require.Error(t, err, "an unrecognized model token must be rejected, not coerced to minilm")
	assert.Contains(t, err.Error(), "unrecognized embedding model")
}
