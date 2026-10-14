package index_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/require"
)

// sparseScript is a FullEmbedder whose sparse map is a function of the text.
// Dense and ColBERT in the output are deliberately different from anything
// stored, so a test can see that the re-embed wrote only the sparse column.
type sparseScript struct {
	calls   int
	failOn  string
	emptyOn string
	sparse  map[int32]float32
}

func (s *sparseScript) Embed(context.Context, string) ([]float32, error) {
	return []float32{1, 0, 0, 0}, nil
}
func (s *sparseScript) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1, 0, 0, 0}
	}
	return out, nil
}
func (s *sparseScript) Dims() int    { return 4 }
func (s *sparseScript) Close() error { return nil }
func (s *sparseScript) EmbedFull(ctx context.Context, text string) (*embedding.BGEM3Output, error) {
	outs, err := s.EmbedFullBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return outs[0], nil
}
func (s *sparseScript) EmbedFullBatch(_ context.Context, texts []string) ([]*embedding.BGEM3Output, error) {
	s.calls += len(texts)
	for _, text := range texts {
		if s.failOn != "" && text == s.failOn {
			return nil, errInterrupted
		}
	}
	out := make([]*embedding.BGEM3Output, len(texts))
	for i, text := range texts {
		sp := s.sparse
		if sp == nil {
			sp = map[int32]float32{10: 0.4, 99: 0.2}
		}
		if text == s.emptyOn {
			sp = map[int32]float32{}
		}
		out[i] = &embedding.BGEM3Output{
			Dense:   []float32{9, 9, 9, 9},
			Sparse:  sp,
			ColBERT: [][]float32{{9, 9}},
		}
	}
	return out, nil
}

var errInterrupted = errString("interrupted")

type errString string

func (e errString) Error() string { return string(e) }

func openSparseDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertSparseNote(t *testing.T, db *index.DB, id, body string, sparse map[int32]float32) (dense, colbert []byte) {
	t.Helper()
	dense = index.EncodeEmbedding([]float32{1, 2, 3, 4})
	colbert = index.EncodeColBERTEmbedding([][]float32{{5, 6}})
	_, err := db.Exec(`INSERT INTO notes (id, path, hash, mtime, is_domain, body_text, embedding, sparse_embedding, colbert_embedding)
		VALUES (?, ?, 'h', 1, 0, ?, ?, ?, ?)`,
		id, id+".md", body, dense, index.EncodeSparseEmbedding(sparse), colbert)
	require.NoError(t, err)
	return dense, colbert
}

func blobs(t *testing.T, db *index.DB, id string) (dense, sparse, colbert []byte) {
	t.Helper()
	require.NoError(t, db.QueryRow(`SELECT embedding, sparse_embedding, colbert_embedding FROM notes WHERE id = ?`, id).Scan(&dense, &sparse, &colbert))
	return dense, sparse, colbert
}

func TestReembedStaleSparse_RewritesSparseOnlyAndStamps(t *testing.T) {
	db := openSparseDB(t)
	dense, colbert := insertSparseNote(t, db, "a", "alpha body", map[int32]float32{0: 1, 10: 0.5})
	insertSparseNote(t, db, "b", "beta body", map[int32]float32{10: 0.5}) // stale, and id 0 is absent

	var buf bytes.Buffer
	script := &sparseScript{}
	n, err := (&index.Indexer{}).ReembedStaleSparse(context.Background(), db, script, &buf)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, 2, script.calls)
	require.Equal(t, "re-embedding sparse vectors for 2 notes (format change)\nre-embedded sparse vectors 2/2\n", buf.String())

	gotDense, gotSparse, gotColbert := blobs(t, db, "a")
	require.Equal(t, dense, gotDense)
	require.Equal(t, colbert, gotColbert)
	decoded, err := index.DecodeSparseEmbedding(gotSparse)
	require.NoError(t, err)
	require.Equal(t, map[int32]float32{10: 0.4, 99: 0.2}, decoded)
	require.NotContains(t, decoded, int32(0))

	format, err := index.ReadSparseFormat(db)
	require.NoError(t, err)
	require.Equal(t, index.SparseFormatCurrent, format)

	buf.Reset()
	calls := script.calls
	n, err = (&index.Indexer{}).ReembedStaleSparse(context.Background(), db, script, &buf)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.Equal(t, calls, script.calls, "a fresh stamp does not call the embedder")
	require.Empty(t, buf.String())
}

func TestReembedStaleSparse_FreshStampKeepsIDZero(t *testing.T) {
	db := openSparseDB(t)
	insertSparseNote(t, db, "a", "alpha body", map[int32]float32{0: 1, 10: 0.2})
	require.NoError(t, index.WriteSparseFormat(db, index.SparseFormatCurrent))
	_, before, _ := blobs(t, db, "a")

	script := &sparseScript{}
	var buf bytes.Buffer
	n, err := (&index.Indexer{}).ReembedStaleSparse(context.Background(), db, script, &buf)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.Equal(t, 0, script.calls, "id 0 in the map is not a reason to re-embed")
	require.Empty(t, buf.String())
	_, after, _ := blobs(t, db, "a")
	require.Equal(t, before, after)
}

func TestReembedStaleSparse_EmptyMapDoesNotBlockTheStamp(t *testing.T) {
	db := openSparseDB(t)
	// 4096-byte dense is the migration-006 case: NULL sparse is refused, a
	// zero-length blob is not. A short sibling must still be rewritten.
	dense := make([]byte, 4096)
	colbert := index.EncodeColBERTEmbedding([][]float32{{5, 6}})
	_, err := db.Exec(`INSERT INTO notes (id, path, hash, mtime, is_domain, body_text, embedding, sparse_embedding, colbert_embedding)
		VALUES ('empty', 'empty.md', 'h', 1, 0, 'stub', ?, ?, ?)`,
		dense, index.EncodeSparseEmbedding(map[int32]float32{0: 1, 7: 0.3}), colbert)
	require.NoError(t, err)
	insertSparseNote(t, db, "full", "alpha body", map[int32]float32{0: 1})

	script := &sparseScript{emptyOn: "stub"}
	n, err := (&index.Indexer{}).ReembedStaleSparse(context.Background(), db, script, &bytes.Buffer{})
	require.NoError(t, err)
	require.Equal(t, 2, n)

	format, rerr := index.ReadSparseFormat(db)
	require.NoError(t, rerr)
	require.Equal(t, index.SparseFormatCurrent, format)

	var isNull int
	var got []byte
	require.NoError(t, db.QueryRow(`SELECT sparse_embedding, sparse_embedding IS NULL FROM notes WHERE id = 'empty'`).Scan(&got, &isNull))
	require.Equal(t, 0, isNull, "the empty map is stored, not NULL")
	decoded, err := index.DecodeSparseEmbedding(got)
	require.NoError(t, err)
	require.Empty(t, decoded)
	require.NotContains(t, decoded, int32(0))

	_, fullSparse, _ := blobs(t, db, "full")
	decoded, err = index.DecodeSparseEmbedding(fullSparse)
	require.NoError(t, err)
	require.Equal(t, map[int32]float32{10: 0.4, 99: 0.2}, decoded)

	calls := script.calls
	n, err = (&index.Indexer{}).ReembedStaleSparse(context.Background(), db, script, &bytes.Buffer{})
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.Equal(t, calls, script.calls, "the stamp stops the next pass")
}

func TestReembedStaleSparse_EmbedErrorDoesNotStamp(t *testing.T) {
	db := openSparseDB(t)
	insertSparseNote(t, db, "a", "alpha", map[int32]float32{0: 1})
	insertSparseNote(t, db, "b", "beta", map[int32]float32{0: 1})
	_, before, _ := blobs(t, db, "b")

	script := &sparseScript{failOn: "beta"}
	_, err := (&index.Indexer{}).ReembedStaleSparse(context.Background(), db, script, &bytes.Buffer{})
	require.Error(t, err)
	format, rerr := index.ReadSparseFormat(db)
	require.NoError(t, rerr)
	require.Equal(t, 0, format)
	_, after, _ := blobs(t, db, "b")
	require.Equal(t, before, after, "the note that failed is not rewritten")
}

func TestReembedStaleSparse_PrintsProgressEveryBatch(t *testing.T) {
	db := openSparseDB(t)
	const notes = 33 // one past the regular batch cap
	for i := range notes {
		insertSparseNote(t, db, fmt.Sprintf("n%02d", i), "x", map[int32]float32{0: 1})
	}
	var buf bytes.Buffer
	n, err := (&index.Indexer{}).ReembedStaleSparse(context.Background(), db, &sparseScript{}, &buf)
	require.NoError(t, err)
	require.Equal(t, notes, n)
	require.Contains(t, buf.String(), "re-embedded sparse vectors 32/33\n")
	require.Contains(t, buf.String(), "re-embedded sparse vectors 33/33\n")
	require.Equal(t, 2, strings.Count(buf.String(), "re-embedded sparse vectors"))
}

func TestSparseFormatStale_NoSparseIsNotStale(t *testing.T) {
	db := openSparseDB(t)
	_, err := db.Exec(`INSERT INTO notes (id, path, hash, mtime) VALUES ('a', 'a.md', 'h', 1)`)
	require.NoError(t, err)
	stale, err := index.SparseFormatStale(db)
	require.NoError(t, err)
	require.False(t, stale)
}
