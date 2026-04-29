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

// fakeFullEmbedder satisfies FullEmbedder: dense + sparse + ColBERT. The
// BGE-M3 path in EmbedNotes uses this to fan out into three UPDATEs per note.
type fakeFullEmbedder struct {
	fakeDenseEmbedder
}

func (f fakeFullEmbedder) EmbedFullBatch(_ context.Context, texts []string) ([]*embedding.BGEM3Output, error) {
	out := make([]*embedding.BGEM3Output, len(texts))
	for i, tx := range texts {
		_ = tx
		out[i] = &embedding.BGEM3Output{
			Dense:   []float32{1, 0, 0, 0},
			Sparse:  map[int32]float32{1: 0.5},
			ColBERT: [][]float32{{0.1, 0.2}},
		}
	}
	return out, nil
}

var _ embedding.FullEmbedder = fakeFullEmbedder{}

// EmbedNotes with a FullEmbedder exercises the BGE-M3 branch (dense +
// sparse + ColBERT stored). Coverage for this branch asserts that the
// dense-only stub doesn't silently misroute against FullEmbedder callers,
// and that sparse+colbert columns get populated.
func TestEmbedNotes_FullEmbedderStoresAllThreeColumns(t *testing.T) {
	vaultRoot, dbPath := buildEmbedTestVault(t)
	cfg, err := vault.LoadConfig(vaultRoot)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultRoot, dbPath, cfg)

	emb := fakeFullEmbedder{fakeDenseEmbedder: fakeDenseEmbedder{dims: 4}}
	r, err := idxr.EmbedNotes(context.Background(), dbPath, emb)
	require.NoError(t, err)
	assert.Equal(t, 3, r.Embedded)

	// Second pass on a FullEmbedder DB must skip everything — the skipQuery
	// checks ALL THREE columns, so anything less than "all three present"
	// would trigger a re-embed.
	r2, err := idxr.EmbedNotes(context.Background(), dbPath, emb)
	require.NoError(t, err)
	assert.Equal(t, 0, r2.Embedded, "post-full-embed: nothing pending")
	assert.Equal(t, 3, r2.Skipped)

	// Sparse+colbert signals flip too.
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()
	hasSparse, err := index.HasSparseEmbeddings(db)
	require.NoError(t, err)
	assert.True(t, hasSparse, "sparse column must be populated")
	hasColBERT, err := index.HasColBERTEmbeddings(db)
	require.NoError(t, err)
	assert.True(t, hasColBERT, "colbert column must be populated")
}

// fakeEmptyOutputEmbedder satisfies FullEmbedder but returns one note's
// Sparse and ColBERT outputs as empty — the silent-failure shape captured
// in vaultmind#22, where bge-m3's heads produce empty maps/slices for some
// inputs and the indexer used to count those as Embedded successfully.
type fakeEmptyOutputEmbedder struct {
	fakeDenseEmbedder
	emptyIndex int // texts[emptyIndex] gets empty Sparse/ColBERT
}

func (f fakeEmptyOutputEmbedder) EmbedFullBatch(_ context.Context, texts []string) ([]*embedding.BGEM3Output, error) {
	out := make([]*embedding.BGEM3Output, len(texts))
	for i := range texts {
		if i == f.emptyIndex {
			out[i] = &embedding.BGEM3Output{
				Dense:   []float32{1, 0, 0, 0},
				Sparse:  map[int32]float32{}, // empty — heads produced no usable tokens
				ColBERT: nil,                 // empty — heads produced no usable tokens
			}
			continue
		}
		out[i] = &embedding.BGEM3Output{
			Dense:   []float32{1, 0, 0, 0},
			Sparse:  map[int32]float32{1: 0.5},
			ColBERT: [][]float32{{0.1, 0.2}},
		}
	}
	return out, nil
}

var _ embedding.FullEmbedder = fakeEmptyOutputEmbedder{}

// EmbedNotes must NOT count notes with empty Sparse/ColBERT as Embedded —
// they would silently leave NULL in the sparse_embedding/colbert_embedding
// columns, producing the doctor warning "Partial BGE-M3 coverage" with no
// per-note diagnostic at indexing time. See vaultmind#22.
func TestEmbedNotes_FullEmbedder_EmptyOutputCountedSeparately(t *testing.T) {
	vaultRoot, dbPath := buildEmbedTestVault(t)
	cfg, err := vault.LoadConfig(vaultRoot)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultRoot, dbPath, cfg)

	// Three notes; one of them gets an empty-output result from the embedder.
	emb := fakeEmptyOutputEmbedder{
		fakeDenseEmbedder: fakeDenseEmbedder{dims: 4},
		emptyIndex:        1,
	}
	r, err := idxr.EmbedNotes(context.Background(), dbPath, emb)
	require.NoError(t, err)

	// Two notes embedded successfully (full modalities), one had empty
	// Sparse/ColBERT and must be reported separately rather than silently
	// counted as Embedded.
	assert.Equal(t, 2, r.Embedded, "only notes with full modality coverage count as Embedded")
	assert.Equal(t, 1, r.EmptyOutput, "notes with empty Sparse/ColBERT must be surfaced via EmptyOutput")

	// Re-running embed must pick the empty-output note up as still-pending,
	// not treat it as already embedded — the existing skip query already
	// requires all three columns to be NOT NULL on the BGE-M3 path, so this
	// is a regression guard.
	emb2 := fakeFullEmbedder{fakeDenseEmbedder: fakeDenseEmbedder{dims: 4}}
	r2, err := idxr.EmbedNotes(context.Background(), dbPath, emb2)
	require.NoError(t, err)
	assert.Equal(t, 1, r2.Embedded, "the previously empty-output note is still pending and gets embedded now")
}

// RunEmbed must NOT load the embedder when there are no pending notes.
// Loading the BGE-M3 model is ~2.2GB of memory and a CPU-spinning ORT
// session creation; doing that on every no-op index call is wasted heat.
// The lazy-load path counts pending notes first and returns early when
// pending == 0. We verify by calling RunEmbed against a fully-embedded
// vault with a model name that would FAIL to construct (the model files
// don't exist) — if the lazy path works, RunEmbed succeeds without
// touching the model; if it eagerly loads, the construction failure
// surfaces as an error.
func TestRunEmbed_LazyLoad_SkipsModelWhenNoPendingWork(t *testing.T) {
	vaultRoot, dbPath := buildEmbedTestVault(t)
	cfg, err := vault.LoadConfig(vaultRoot)
	require.NoError(t, err)
	idxr := index.NewIndexer(vaultRoot, dbPath, cfg)

	// Fully embed first using the fake dense embedder so all rows have
	// the dense column populated.
	emb := fakeDenseEmbedder{dims: 8}
	r1, err := idxr.EmbedNotes(context.Background(), dbPath, emb)
	require.NoError(t, err)
	require.Equal(t, 3, r1.Embedded)

	// Now call RunEmbed with model name "minilm" (the default Hugot path).
	// Pending count will be 0 because all rows already have dense.
	// If RunEmbed lazy-loads correctly, it returns without invoking the
	// embedder — therefore without requiring the MiniLM model files to
	// exist or any network access. If it eagerly loads, it will try to
	// download or load the model and either succeed (slow, requires
	// model on disk) or fail.
	//
	// We can't directly observe "embedder was not constructed" without
	// instrumenting the indexer. The signal we use: the call is fast and
	// returns Skipped=3, Embedded=0 with no error. Hugot's MiniLM
	// constructor would attempt a model download if the model isn't
	// cached, which would either error out or be slow — neither happens
	// here when lazy-load works.
	r2, err := idxr.RunEmbed(context.Background(), dbPath, "minilm")
	require.NoError(t, err, "lazy-load: RunEmbed must succeed without constructing the embedder when nothing is pending")
	assert.Equal(t, 0, r2.Embedded, "no notes should be embedded — already all done")
	assert.Equal(t, 3, r2.Skipped, "all 3 already-embedded notes counted as skipped")
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
