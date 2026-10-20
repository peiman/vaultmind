package index

import (
	"bytes"
	"context"
	"errors"
	"os"
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

	skip, _, err := skipIncrementalEmbed(dbPath, embedding.ModelBGEM3, false)
	require.NoError(t, err)
	require.False(t, skip, "a fully embedded stale column is still work")

	miniPath := filepath.Join(t.TempDir(), "mini.db")
	mini, err := Open(miniPath)
	require.NoError(t, err)
	_, err = mini.Exec(`INSERT INTO notes (id, path, hash, mtime, embedding) VALUES ('m', 'm.md', 'h', 1, ?)`,
		EncodeEmbedding(make([]float32, 384)))
	require.NoError(t, err)
	require.NoError(t, mini.Close())
	skip, _, err = skipIncrementalEmbed(miniPath, embedding.ModelMiniLM, false)
	require.NoError(t, err)
	require.True(t, skip, "minilm does not upgrade a sparse column")
}

// denseOnly is an Embedder that cannot produce sparse vectors.
type denseOnly struct{}

func (denseOnly) Embed(context.Context, string) ([]float32, error) {
	return []float32{1, 0, 0, 0}, nil
}
func (denseOnly) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return [][]float32{{1, 0, 0, 0}}, nil
}
func (denseOnly) Dims() int    { return 4 }
func (denseOnly) Close() error { return nil }

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func blockedDBPath(t *testing.T) string {
	t.Helper()
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(parent, []byte("x"), 0o644))
	return filepath.Join(parent, "index.db")
}

func TestReadSparseFormat_ClosedDBAndGarbageValue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	_, err = ReadSparseFormat(db)
	require.Error(t, err)

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`INSERT INTO meta (key, value) VALUES (?, ?)`, metaKeySparseFormat, "nope")
	require.NoError(t, err)
	_, err = ReadSparseFormat(db)
	require.Error(t, err)
	_, err = SparseFormatStale(db)
	require.Error(t, err)
}

func TestSparseFormatStale_CountFailure(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`ALTER TABLE notes RENAME TO notes_kept`)
	require.NoError(t, err)
	_, err = SparseFormatStale(db)
	require.Error(t, err)
}

func TestSparseFormatStaleFile_UnopenablePath(t *testing.T) {
	_, err := SparseFormatStaleFile(blockedDBPath(t))
	require.Error(t, err)
}

func TestReembedStaleSparse_NeedsAFullEmbedder(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, sparse_embedding) VALUES ('a', 'a.md', 'h', 1, ?)`,
		EncodeSparseEmbedding(map[int32]float32{0: 1}))
	require.NoError(t, err)
	_, err = (&Indexer{}).ReembedStaleSparse(context.Background(), db, denseOnly{}, &bytes.Buffer{})
	require.Error(t, err)
}

func TestReembedStaleSparse_ProgressWriteErrorDoesNotStamp(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, body_text, sparse_embedding) VALUES ('a', 'a.md', 'h', 1, 'alpha', ?)`,
		EncodeSparseEmbedding(map[int32]float32{0: 1}))
	require.NoError(t, err)
	script := &whiteboxSparse{}
	_, err = (&Indexer{}).ReembedStaleSparse(context.Background(), db, script, failWriter{})
	require.Error(t, err)
	require.Zero(t, script.calls)
	format, rerr := ReadSparseFormat(db)
	require.NoError(t, rerr)
	require.Equal(t, 0, format)
}

func TestReembedStaleSparse_StampWriteErrorLeavesTheColumnNew(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, body_text, sparse_embedding) VALUES ('a', 'a.md', 'h', 1, 'alpha', ?)`,
		EncodeSparseEmbedding(map[int32]float32{0: 1}))
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TRIGGER meta_refuse BEFORE INSERT ON meta BEGIN SELECT RAISE(ABORT, 'refused'); END`)
	require.NoError(t, err)

	_, err = (&Indexer{}).ReembedStaleSparse(context.Background(), db, &whiteboxSparse{}, &bytes.Buffer{})
	require.Error(t, err)
	format, rerr := ReadSparseFormat(db)
	require.NoError(t, rerr)
	require.Equal(t, 0, format, "a failed stamp write does not claim the column is current")
}

func TestStoreSparseBatch_LengthAndMissingNote(t *testing.T) {
	_, err := storeSparseBatch(nil, nil, []int{0}, nil)
	require.Error(t, err)

	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = storeSparseBatch(db,
		[]sparseNote{{id: "missing"}},
		[]int{0},
		[]*embedding.BGEM3Output{{Sparse: map[int32]float32{10: 0.4}}},
	)
	require.Error(t, err)

	_, err = storeSparseBatch(db,
		[]sparseNote{{id: "missing"}},
		[]int{0},
		[]*embedding.BGEM3Output{{Sparse: map[int32]float32{}}},
	)
	require.Error(t, err)

	_, err = storeSparseBatch(db,
		[]sparseNote{{id: "missing"}},
		[]int{0},
		[]*embedding.BGEM3Output{nil},
	)
	require.Error(t, err)
}

type nWrites struct{ left int }

func (n *nWrites) Write(p []byte) (int, error) {
	if n.left == 0 {
		return 0, errors.New("write failed")
	}
	n.left--
	return len(p), nil
}

func TestReembedStaleSparse_BatchProgressWriteErrorDoesNotStamp(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, body_text, sparse_embedding) VALUES ('a', 'a.md', 'h', 1, 'alpha', ?)`,
		EncodeSparseEmbedding(map[int32]float32{0: 1}))
	require.NoError(t, err)

	_, err = (&Indexer{}).ReembedStaleSparse(context.Background(), db, &whiteboxSparse{}, &nWrites{left: 1})
	require.Error(t, err)
	format, rerr := ReadSparseFormat(db)
	require.NoError(t, rerr)
	require.Equal(t, 0, format)
}

func TestSkipIncrementalEmbed_PendingIsNotIdle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime) VALUES ('a', 'a.md', 'h', 1)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	skip, _, err := skipIncrementalEmbed(dbPath, embedding.ModelMiniLM, false)
	require.NoError(t, err)
	require.False(t, skip)
}

func TestMaybeUpgradeSparse_SkipsADenseOnlyEmbedder(t *testing.T) {
	n, err := NewIndexer(t.TempDir(), "unused", nil).maybeUpgradeSparse(context.Background(), "unused", denseOnly{}, embedding.ModelBGEM3)
	require.NoError(t, err)
	require.Equal(t, 0, n)
}

func TestMaybeUpgradeSparse_OpenError(t *testing.T) {
	_, err := NewIndexer(t.TempDir(), "unused", nil).maybeUpgradeSparse(context.Background(), blockedDBPath(t), &whiteboxSparse{}, embedding.ModelBGEM3)
	require.Error(t, err)
}

func TestStampCurrentSparse_OpenError(t *testing.T) {
	err := stampCurrentSparse(blockedDBPath(t), embedding.ModelBGEM3, 0, &EmbedResult{Embedded: 1})
	require.Error(t, err)
}

func TestEmbedResolved_StampWriteErrorDoesNotClaimSuccess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, body_text) VALUES ('a', 'a.md', 'h', 1, 'alpha body')`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TRIGGER meta_refuse BEFORE INSERT ON meta BEGIN SELECT RAISE(ABORT, 'refused'); END`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	res, err := NewIndexer(t.TempDir(), dbPath, nil).embedResolved(context.Background(), dbPath, &whiteboxSparse{}, embedding.ModelBGEM3, false)
	require.Error(t, err)
	require.NotNil(t, res)
	// The text stamp is written before EmbedNotes. A refused meta write stops
	// the pass before any vector is stored, so success cannot be reported.
	require.Equal(t, 0, res.Embedded)

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	format, rerr := ReadSparseFormat(db)
	require.NoError(t, rerr)
	require.Equal(t, 0, format)
}
