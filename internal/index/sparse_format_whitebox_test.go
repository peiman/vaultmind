package index

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/require"
)

type whiteboxSparse struct {
	calls  int
	fail   bool
	sparse map[int32]float32
}

func (w *whiteboxSparse) Embed(context.Context, string) ([]float32, error) {
	return []float32{1, 0, 0, 0}, nil
}
func (w *whiteboxSparse) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1, 0, 0, 0}
	}
	return out, nil
}
func (w *whiteboxSparse) Dims() int    { return 4 }
func (w *whiteboxSparse) Close() error { return nil }
func (w *whiteboxSparse) EmbedFull(ctx context.Context, text string) (*embedding.BGEM3Output, error) {
	outs, err := w.EmbedFullBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return outs[0], nil
}
func (w *whiteboxSparse) EmbedFullBatch(_ context.Context, texts []string) ([]*embedding.BGEM3Output, error) {
	w.calls += len(texts)
	if w.fail {
		return nil, errString("interrupted")
	}
	sp := w.sparse
	if sp == nil {
		sp = map[int32]float32{10: 0.4}
	}
	out := make([]*embedding.BGEM3Output, len(texts))
	for i := range texts {
		out[i] = &embedding.BGEM3Output{
			Dense:   []float32{1, 0, 0, 0},
			Sparse:  sp,
			ColBERT: [][]float32{{0.1, 0.2}},
		}
	}
	return out, nil
}

type errString string

func (e errString) Error() string { return string(e) }

func TestEmbedResolved_FreshNotesStampTheFormat(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, body_text) VALUES ('a', 'a.md', 'h', 1, 'alpha body')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	script := &whiteboxSparse{}
	res, err := NewIndexer(t.TempDir(), dbPath, nil).embedResolved(context.Background(), dbPath, script, embedding.ModelBGEM3, false)
	require.NoError(t, err)
	require.Equal(t, 1, res.Embedded)
	require.Equal(t, 0, res.SparseReembedded)
	require.Equal(t, 1, script.calls)

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	format, err := ReadSparseFormat(db)
	require.NoError(t, err)
	require.Equal(t, SparseFormatCurrent, format)
}

func TestEmbedResolved_InterruptDoesNotEmbedPendingOrStamp(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, body_text, embedding, sparse_embedding, colbert_embedding)
		VALUES ('old', 'old.md', 'h', 1, 'old body', ?, ?, ?)`,
		EncodeEmbedding([]float32{1, 2, 3, 4}),
		EncodeSparseEmbedding(map[int32]float32{0: 1}),
		EncodeColBERTEmbedding([][]float32{{5, 6}}))
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, body_text) VALUES ('new', 'new.md', 'h', 1, 'new body')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	script := &whiteboxSparse{fail: true}
	_, err = NewIndexer(t.TempDir(), dbPath, nil).embedResolved(context.Background(), dbPath, script, embedding.ModelBGEM3, false)
	require.Error(t, err)

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	format, err := ReadSparseFormat(db)
	require.NoError(t, err)
	require.Equal(t, 0, format)
	var dense []byte
	require.NoError(t, db.QueryRow(`SELECT embedding FROM notes WHERE id = 'new'`).Scan(&dense))
	require.Nil(t, dense, "EmbedNotes must not run after a failed sparse pass")
}

func TestEmbedResolved_FullDoesNotAnnounceAZeroReembed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, body_text, embedding, sparse_embedding, colbert_embedding)
		VALUES ('old', 'old.md', 'h', 1, 'old body', ?, ?, ?)`,
		EncodeEmbedding([]float32{1, 2, 3, 4}),
		EncodeSparseEmbedding(map[int32]float32{0: 1}),
		EncodeColBERTEmbedding([][]float32{{5, 6}}))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	var buf bytes.Buffer
	idx := NewIndexer(t.TempDir(), dbPath, nil)
	idx.Progress = &buf
	script := &whiteboxSparse{}
	res, err := idx.embedResolved(context.Background(), dbPath, script, embedding.ModelBGEM3, true)
	require.NoError(t, err)
	require.Equal(t, 1, res.Embedded)
	require.NotContains(t, buf.String(), "re-embedding sparse vectors")

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	format, err := ReadSparseFormat(db)
	require.NoError(t, err)
	require.Equal(t, SparseFormatCurrent, format)
}

func TestSkipIncrementalEmbed_StaleBGEM3IsNotIdle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, embedding, sparse_embedding, colbert_embedding)
		VALUES ('a', 'a.md', 'h', 1, ?, ?, ?)`,
		EncodeEmbedding(make([]float32, 1024)),
		EncodeSparseEmbedding(map[int32]float32{0: 1}),
		EncodeColBERTEmbedding([][]float32{{1}}))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	skip, _, err := skipIncrementalEmbed(dbPath, embedding.ModelBGEM3)
	require.NoError(t, err)
	require.False(t, skip, "a fully embedded stale column is still work")

	miniPath := filepath.Join(t.TempDir(), "mini.db")
	mini, err := Open(miniPath)
	require.NoError(t, err)
	_, err = mini.Exec(`INSERT INTO notes (id, path, hash, mtime, embedding) VALUES ('m', 'm.md', 'h', 1, ?)`,
		EncodeEmbedding(make([]float32, 384)))
	require.NoError(t, err)
	require.NoError(t, mini.Close())
	skip, _, err = skipIncrementalEmbed(miniPath, embedding.ModelMiniLM)
	require.NoError(t, err)
	require.True(t, skip, "minilm does not upgrade a sparse column")
}
