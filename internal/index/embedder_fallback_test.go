package index

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEmbedder satisfies embedding.Embedder for fallback-path tests.
type fakeEmbedder struct{ dims int }

func (f *fakeEmbedder) Embed(context.Context, string) ([]float32, error) {
	return make([]float32, f.dims), nil
}
func (f *fakeEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, f.dims)
	}
	return out, nil
}
func (f *fakeEmbedder) Dims() int    { return f.dims }
func (f *fakeEmbedder) Close() error { return nil }

func okCtor(dims int) embedderCtor {
	return func() (embedding.Embedder, error) { return &fakeEmbedder{dims: dims}, nil }
}
func failCtor(msg string) embedderCtor {
	return func() (embedding.Embedder, error) { return nil, errors.New(msg) }
}

// The headline behavior: bge-m3 requested but unavailable at runtime → use
// minilm, and report minilm as the model actually used (so the caller stamps
// and calibrates against reality, not the request).
func TestLoadEmbedder_FallsBackToMinilmWhenBGEUnavailable(t *testing.T) {
	e, used, err := loadEmbedder("bge-m3", failCtor("ort runtime unavailable"), okCtor(384))
	require.NoError(t, err)
	require.NotNil(t, e)
	assert.Equal(t, "minilm", used, "fallback must report the model actually used")
}

func TestLoadEmbedder_UsesBGEWhenAvailable(t *testing.T) {
	e, used, err := loadEmbedder("bge-m3", okCtor(1024), failCtor("minilm should not be called"))
	require.NoError(t, err)
	require.NotNil(t, e)
	assert.Equal(t, "bge-m3", used)
}

func TestLoadEmbedder_NonBGERequestUsesThatModelDirectly(t *testing.T) {
	e, used, err := loadEmbedder("minilm", failCtor("bge should not be called"), okCtor(384))
	require.NoError(t, err)
	require.NotNil(t, e)
	assert.Equal(t, "minilm", used)
}

// If even the fallback fails, surface a clear combined error (no silent
// success, no nil embedder).
func TestLoadEmbedder_BothFail_ReturnsCombinedError(t *testing.T) {
	e, used, err := loadEmbedder("bge-m3", failCtor("bge boom"), failCtor("minilm boom"))
	require.Error(t, err)
	assert.Nil(t, e)
	assert.Empty(t, used)
	assert.Contains(t, err.Error(), "minilm fallback", "error must name the failed fallback")
}

// hasBGEM3Embeddings is the guardrail that refuses a bge-m3→minilm fallback on a
// vault that already holds bge-m3 embeddings (which would create a mixed index).
func TestHasBGEM3Embeddings(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")

	insert := func(col string, val []byte) {
		db, err := Open(dbPath)
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		_, err = db.Exec(
			"INSERT INTO notes (id, path, hash, mtime, "+col+") VALUES (?, ?, 'h', 0, ?)",
			col, col+".md", val)
		require.NoError(t, err)
	}

	// Fresh vault — no embeddings of any kind.
	db, err := Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	has, err := hasBGEM3Embeddings(dbPath)
	require.NoError(t, err)
	assert.False(t, has, "empty vault holds no bge-m3 embeddings")

	// A dense-only (minilm) note must NOT count as bge-m3.
	insert("embedding", []byte{1, 2, 3})
	has, err = hasBGEM3Embeddings(dbPath)
	require.NoError(t, err)
	assert.False(t, has, "dense-only note is minilm, not bge-m3")

	// A note with a sparse embedding marks the vault as bge-m3.
	insert("sparse_embedding", []byte{9})
	has, err = hasBGEM3Embeddings(dbPath)
	require.NoError(t, err)
	assert.True(t, has, "sparse_embedding present → bge-m3 vault → fallback must refuse")
}
