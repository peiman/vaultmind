package index_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// batchRecorder is a FullEmbedder that records the size and longest text of
// every batch it is sent. It never refuses one: the test asserts on what the
// indexer chose to send, so a caller that skips the planner shows up here
// even though the real embedder would refuse its batch.
type batchRecorder struct {
	sparseScript
	batches []recordedBatch
}

type recordedBatch struct{ count, longest int }

func (b *batchRecorder) EmbedFullBatch(ctx context.Context, texts []string) ([]*embedding.BGEM3Output, error) {
	longest := 0
	for _, t := range texts {
		longest = max(longest, len(t))
	}
	b.batches = append(b.batches, recordedBatch{len(texts), longest})
	return b.sparseScript.EmbedFullBatch(ctx, texts)
}

func (b *batchRecorder) requireEveryBatchFits(t *testing.T, notes int) {
	t.Helper()
	require.NotEmpty(t, b.batches, "the embedder was never called")
	sent := 0
	for i, rb := range b.batches {
		assert.True(t, embedding.BGEM3BatchFits(rb.count, rb.longest),
			"batch %d: %d notes, longest %d chars, is over the BGE-M3 memory budget", i, rb.count, rb.longest)
		sent += rb.count
	}
	assert.Equal(t, notes, sent, "every note is embedded exactly once")
}

// memoryTestBodies has the shape that broke v0.10.13: a few notes past the
// token cap among many mid-length ones. Batches of 8 or 32 of these are over
// the budget, so a call site that plans by count alone fails this test.
func memoryTestBodies() []string {
	var bodies []string
	for i := range 4 {
		bodies = append(bodies, fmt.Sprintf("long %d ", i)+strings.Repeat("word ", 2000))
	}
	for i := range 30 {
		bodies = append(bodies, fmt.Sprintf("mid %d ", i)+strings.Repeat("word ", 140))
	}
	for i := range 10 {
		bodies = append(bodies, fmt.Sprintf("short %d", i))
	}
	return bodies
}

func TestEmbedNotes_EveryBGEM3BatchFitsTheMemoryBudget(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"),
		[]byte("types:\n  concept:\n    required: [title]\n"), 0o644))
	bodies := memoryTestBodies()
	for i, body := range bodies {
		content := fmt.Sprintf("---\nid: concept-n%02d\ntype: concept\ntitle: n%02d\n---\n%s\n", i, i, body)
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("n%02d.md", i)), []byte(content), 0o644))
	}
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	idxr := index.NewIndexer(dir, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	rec := &batchRecorder{}
	res, err := idxr.EmbedNotes(context.Background(), dbPath, rec)
	require.NoError(t, err)
	require.Equal(t, len(bodies), res.Embedded)
	rec.requireEveryBatchFits(t, len(bodies))
}

func TestReembedStaleSparse_EveryBatchFitsTheMemoryBudget(t *testing.T) {
	db := openSparseDB(t)
	bodies := memoryTestBodies()
	for i, body := range bodies {
		insertSparseNote(t, db, fmt.Sprintf("n%02d", i), body, map[int32]float32{0: 1})
	}
	rec := &batchRecorder{}
	n, err := (&index.Indexer{}).ReembedStaleSparse(context.Background(), db, rec, &bytes.Buffer{})
	require.NoError(t, err)
	require.Equal(t, len(bodies), n)
	rec.requireEveryBatchFits(t, len(bodies))
}
