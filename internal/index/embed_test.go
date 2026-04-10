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

func TestEmbedNotes(t *testing.T) {
	if os.Getenv("VAULTMIND_TEST_EMBEDDING") == "" {
		t.Skip("skipping embedding test (set VAULTMIND_TEST_EMBEDDING=1)")
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	vaultPath := "../../vaultmind-vault"
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(vaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	embedder, err := embedding.NewHugotEmbedder(embedding.HugotConfig{
		ModelName:    "sentence-transformers/all-MiniLM-L6-v2",
		CacheDir:     t.TempDir(),
		Dims:         384,
		OnnxFilePath: "onnx/model.onnx",
	})
	require.NoError(t, err)
	defer func() { _ = embedder.Close() }()

	result, err := idxr.EmbedNotes(context.Background(), dbPath, embedder)
	require.NoError(t, err)
	assert.Greater(t, result.Embedded, 0, "should embed at least one note")
	assert.Equal(t, 0, result.Errors)

	// Verify embeddings were stored
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	has, err := index.HasEmbeddings(db)
	require.NoError(t, err)
	assert.True(t, has)

	all, err := index.LoadAllEmbeddings(db)
	require.NoError(t, err)
	assert.Equal(t, result.Embedded, len(all))
	for _, ne := range all {
		assert.Len(t, ne.Embedding, 384)
	}
}

func TestEmbedNotes_Incremental(t *testing.T) {
	if os.Getenv("VAULTMIND_TEST_EMBEDDING") == "" {
		t.Skip("skipping embedding test (set VAULTMIND_TEST_EMBEDDING=1)")
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	vaultPath := "../../vaultmind-vault"
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(vaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	embedder, err := embedding.NewHugotEmbedder(embedding.HugotConfig{
		ModelName:    "sentence-transformers/all-MiniLM-L6-v2",
		CacheDir:     t.TempDir(),
		Dims:         384,
		OnnxFilePath: "onnx/model.onnx",
	})
	require.NoError(t, err)
	defer func() { _ = embedder.Close() }()

	// First embed
	result1, err := idxr.EmbedNotes(context.Background(), dbPath, embedder)
	require.NoError(t, err)
	assert.Greater(t, result1.Embedded, 0)

	// Second embed — all should be skipped
	result2, err := idxr.EmbedNotes(context.Background(), dbPath, embedder)
	require.NoError(t, err)
	assert.Equal(t, 0, result2.Embedded)
	assert.Equal(t, result1.Embedded, result2.Skipped)
}
