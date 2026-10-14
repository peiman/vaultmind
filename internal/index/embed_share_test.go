package index_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedVecEmbedder stores one vector for every text and counts Close calls.
type fixedVecEmbedder struct {
	vec    []float32
	closed int
}

func (e *fixedVecEmbedder) Embed(context.Context, string) ([]float32, error) {
	return append([]float32(nil), e.vec...), nil
}

func (e *fixedVecEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = append([]float32(nil), e.vec...)
	}
	return out, nil
}

func (e *fixedVecEmbedder) Dims() int { return len(e.vec) }

func (e *fixedVecEmbedder) Close() error {
	e.closed++
	return nil
}

type lendingSource struct{ emb embedding.Embedder }

func (s lendingSource) Borrow(context.Context, string) (embedding.Embedder, bool) {
	return s.emb, true
}

func embeddingBlobs(t *testing.T, dbPath string) map[string][]byte {
	t.Helper()
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	rows, err := db.Query(`SELECT id, embedding FROM notes ORDER BY id`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	out := map[string][]byte{}
	for rows.Next() {
		var id string
		var blob []byte
		require.NoError(t, rows.Scan(&id, &blob))
		out[id] = append([]byte(nil), blob...)
	}
	require.NoError(t, rows.Err())
	return out
}

// An unchanged embed pass, and a pass that builds its own embedder, write the
// same bytes as a pass that borrows one. Borrowing does not close the session.
func TestRunEmbed_UnchangedPassKeepsIdenticalVectorBytes(t *testing.T) {
	vec := make([]float32, embedding.DefaultDims)
	vec[0] = 0.25
	vec[1] = 0.5
	borrowed := &fixedVecEmbedder{vec: vec}
	_, dbPath := buildEmbedTestVault(t)
	// vault root is unused by RunEmbed; the db path is the fixture.
	idxr := index.NewIndexer(".", dbPath, nil)
	idxr.SetEmbedderSource(lendingSource{emb: borrowed})

	prev := embedding.ConstructMiniLM
	embedding.ConstructMiniLM = func(context.Context, embedding.HugotConfig) (embedding.Embedder, error) {
		t.Fatal("a borrowed pass must not build a fresh embedder")
		return nil, nil
	}
	t.Cleanup(func() { embedding.ConstructMiniLM = prev })

	ctx := context.Background()
	res, err := idxr.RunEmbed(ctx, dbPath, embedding.ModelMiniLM, false)
	require.NoError(t, err)
	require.Equal(t, 3, res.Embedded)
	first := embeddingBlobs(t, dbPath)
	want := index.EncodeEmbedding(vec)
	for id, blob := range first {
		assert.True(t, bytes.Equal(want, blob), "note %s", id)
	}

	res, err = idxr.RunEmbed(ctx, dbPath, embedding.ModelMiniLM, false)
	require.NoError(t, err)
	assert.Zero(t, res.Embedded, "nothing pending on the second pass")
	assert.Equal(t, first, embeddingBlobs(t, dbPath))
	assert.Zero(t, borrowed.closed, "the indexer does not close a borrowed embedder")

	owned := &fixedVecEmbedder{vec: vec}
	embedding.ConstructMiniLM = func(context.Context, embedding.HugotConfig) (embedding.Embedder, error) {
		return owned, nil
	}
	_, ownedDB := buildEmbedTestVault(t)
	ownedIdx := index.NewIndexer(".", ownedDB, nil)
	res, err = ownedIdx.RunEmbed(ctx, ownedDB, embedding.ModelMiniLM, false)
	require.NoError(t, err)
	require.Equal(t, 3, res.Embedded)
	assert.Equal(t, 1, owned.closed, "a pass that builds its own embedder releases it")
	assert.Equal(t, first, embeddingBlobs(t, ownedDB))
}
