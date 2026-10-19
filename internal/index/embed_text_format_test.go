package index_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/require"
)

func TestEmbedNotes_SendsTitleAndBody(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, title, body_text) VALUES
		('kept', 'kept.md', 'h', 1, 'Keep', 'the body'),
		('blank', 'blank.md', 'h', 1, ?, 'only body'),
		('none', 'none.md', 'h', 1, '', 'plain')`, " \t")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	emb := &recordingBodyEmbedder{fakeDenseEmbedder: fakeDenseEmbedder{dims: 4}}
	res, err := index.NewIndexer(t.TempDir(), dbPath, nil).EmbedNotes(context.Background(), dbPath, emb)
	require.NoError(t, err)
	require.Equal(t, 3, res.Embedded)
	require.Equal(t, 0, res.Errors)
	require.ElementsMatch(t, []string{"Keep\n\nthe body", "only body", "plain"}, emb.texts)
}

// budgetRejecter is a full embedder that refuses a batch the memory budget
// rejects. A planner that measured the body and sent title+body fails here.
type budgetRejecter struct{ texts []string }

func (b *budgetRejecter) Embed(context.Context, string) ([]float32, error) {
	return []float32{1, 0, 0, 0}, nil
}
func (b *budgetRejecter) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return [][]float32{{1, 0, 0, 0}}, nil
}
func (b *budgetRejecter) Dims() int    { return 4 }
func (b *budgetRejecter) Close() error { return nil }
func (b *budgetRejecter) EmbedFull(ctx context.Context, text string) (*embedding.BGEM3Output, error) {
	outs, err := b.EmbedFullBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return outs[0], nil
}
func (b *budgetRejecter) EmbedFullBatch(_ context.Context, texts []string) ([]*embedding.BGEM3Output, error) {
	longest := 0
	for _, text := range texts {
		longest = max(longest, len(text))
	}
	if !embedding.BGEM3BatchFits(len(texts), longest) {
		return nil, embedding.ErrBatchOverMemoryBudget
	}
	b.texts = append(b.texts, texts...)
	out := make([]*embedding.BGEM3Output, len(texts))
	for i := range texts {
		out[i] = &embedding.BGEM3Output{
			Dense:   []float32{1, 0, 0, 0},
			Sparse:  map[int32]float32{3: 0.5},
			ColBERT: [][]float32{{0.2, 0.3}},
		}
	}
	return out, nil
}

func TestEmbedNotes_PlansOnEmbeddedLength(t *testing.T) {
	// Two bodies share a batch. The same notes with their titles do not.
	// Planning on the body batches them together and the embedder refuses.
	body := strings.Repeat("word ", 160) // 800 chars
	title := strings.Repeat("T", 400)
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	for _, id := range []string{"a", "b"} {
		_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, title, body_text) VALUES (?, ?, 'h', 1, ?, ?)`,
			id, id+".md", title, body)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())

	emb := &budgetRejecter{}
	res, err := index.NewIndexer(t.TempDir(), dbPath, nil).EmbedNotes(context.Background(), dbPath, emb)
	require.NoError(t, err)
	require.Equal(t, 0, res.Errors)
	require.Equal(t, 2, res.Embedded)
	require.Len(t, emb.texts, 2)
	want := title + "\n\n" + body
	require.Equal(t, want, emb.texts[0])
	require.Equal(t, want, emb.texts[1])
}

func TestReembedStaleSparse_EmbedsTitleAndBody(t *testing.T) {
	db := openSparseDB(t)
	body := strings.Repeat("word ", 160)
	title := strings.Repeat("T", 400)
	for _, id := range []string{"a", "b"} {
		_, err := db.Exec(`INSERT INTO notes (id, path, hash, mtime, is_domain, title, body_text, embedding, sparse_embedding, colbert_embedding)
			VALUES (?, ?, 'h', 1, 0, ?, ?, ?, ?, ?)`,
			id, id+".md", title, body,
			index.EncodeEmbedding([]float32{1, 2, 3, 4}),
			index.EncodeSparseEmbedding(map[int32]float32{0: 1}),
			index.EncodeColBERTEmbedding([][]float32{{5, 6}}))
		require.NoError(t, err)
	}
	// A batch planned on the body alone is over the budget once the title is
	// prefixed. The migration must plan on the text it sends.
	emb := &budgetRejecter{}
	n, err := (&index.Indexer{}).ReembedStaleSparse(context.Background(), db, emb, &bytes.Buffer{})
	require.NoError(t, err)
	require.Equal(t, 2, n)
	want := title + "\n\n" + body
	require.Equal(t, []string{want, want}, emb.texts)
}
