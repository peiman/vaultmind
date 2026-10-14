package cmd

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadCountEmbedder is a deterministic stand-in for the ONNX session.
// One note propose must construct it once and still store its vectors.
type loadCountEmbedder struct {
	vec    []float32
	closed int
}

func (e *loadCountEmbedder) Embed(context.Context, string) ([]float32, error) {
	return append([]float32(nil), e.vec...), nil
}

func (e *loadCountEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = append([]float32(nil), e.vec...)
	}
	return out, nil
}

func (e *loadCountEmbedder) Dims() int { return len(e.vec) }

func (e *loadCountEmbedder) Close() error {
	e.closed++
	return nil
}

func useLoadCountEmbedder(t *testing.T) (emb *loadCountEmbedder, loads *int) {
	t.Helper()
	query.CloseSharedEmbedder()
	t.Cleanup(query.CloseSharedEmbedder)
	vec := make([]float32, embedding.DefaultDims)
	vec[0] = 0.5
	emb = &loadCountEmbedder{vec: vec}
	n := 0
	prevMini := embedding.ConstructMiniLM
	prevBGE := embedding.ConstructBGEM3
	embedding.ConstructMiniLM = func(context.Context, embedding.HugotConfig) (embedding.Embedder, error) {
		n++
		return emb, nil
	}
	embedding.ConstructBGEM3 = func(context.Context, embedding.HugotConfig) (embedding.Embedder, error) {
		t.Fatal("a MiniLM vault must not load BGE-M3")
		return nil, nil
	}
	t.Cleanup(func() {
		embedding.ConstructMiniLM = prevMini
		embedding.ConstructBGEM3 = prevBGE
	})
	return emb, &n
}

func seedMiniLMEmbeddings(t *testing.T, dir string, vec []float32) {
	t.Helper()
	db, err := index.Open(filepath.Join(dir, ".vaultmind", "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`UPDATE notes SET embedding = ? WHERE embedding IS NULL`, index.EncodeEmbedding(vec))
	require.NoError(t, err)
}

// One note propose used to cold-load the model for the closest-note search,
// the rank check, the embed-on-write, and the re-embed after the rank keys.
// Those four calls share one session.
func TestNotePropose_LoadsTheEmbedderOnceAndEmbedsTheDraft(t *testing.T) {
	emb, loads := useLoadCountEmbedder(t)
	dir := proposeVault(t)
	seedMiniLMEmbeddings(t, dir, emb.vec)

	out, errOut, err := runRootCmd(t, "note", "propose",
		"--vault", dir,
		"--question", "Where does the embedder session live?",
		"--finding", "The process keeps one session for ranking and for the draft.",
		"--close-band", "not-a-real-band",
		"--json")
	require.NoError(t, err, "stdout: %s\nstderr: %s", out.String(), errOut.String())
	assert.Equal(t, 1, *loads, "note propose loads the model once")
	assert.Contains(t, errOut.String(), "embedded ")

	var env NoteProposeEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.NotEmpty(t, env.Result.ID)

	db, err := index.Open(filepath.Join(dir, ".vaultmind", "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	var blob []byte
	require.NoError(t, db.QueryRow(`SELECT embedding FROM notes WHERE id = ?`, env.Result.ID).Scan(&blob))
	assert.Equal(t, index.EncodeEmbedding(emb.vec), blob, "embed-on-write stored the draft vector")
	assert.Zero(t, emb.closed, "propose does not close the process session")
}

func TestAskRankerClose_DoesNotReleaseASessionItDidNotAcquire(t *testing.T) {
	emb, loads := useLoadCountEmbedder(t)
	dir := t.TempDir()
	db, err := index.Open(filepath.Join(dir, "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, embedding) VALUES ('n1', 'n1.md', 'h', 0, ?)`,
		index.EncodeEmbedding(emb.vec))
	require.NoError(t, err)

	ret := query.BuildAutoRetrieverFull(context.Background(), db)
	require.NotNil(t, ret.Embedder)
	ret.Cleanup()
	require.Equal(t, 1, *loads)

	ranker := newAskRanker(&cobra.Command{}, db, dir).(io.Closer)
	require.NoError(t, ranker.Close())
	require.NoError(t, ranker.Close())
	assert.Zero(t, emb.closed, "a ranker that never acquired the session leaves it loaded")

	query.CloseSharedEmbedder()
	assert.Equal(t, 1, emb.closed)
	query.CloseSharedEmbedder()
	assert.Equal(t, 1, emb.closed)
}
