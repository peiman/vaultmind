package query

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LIVE-OBSERVED (2026-09-23, during #148): a stale libonnxruntime 1.25.0 next
// to the binary made every BGE-M3 load fail. ask fell back to keyword search
// and told the agent "this vault has no embeddings — run index --embed" — for
// a vault with 67/67 embeddings, where re-embedding fails the same way. The
// reason was a debug-level log line nobody reads.

func embeddedTestDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(t.TempDir() + "/idx.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`INSERT INTO notes (id, path, hash, mtime, title, is_domain, embedding) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"n1", "n1.md", "h", 0, "T", true, index.EncodeEmbedding(make([]float32, 384)))
	require.NoError(t, err)
	return db
}

func TestBuildAutoRetrieverFull_RecordsWhyTheEmbedderIsDown(t *testing.T) {
	orig := buildHybridRetrieverFn
	t.Cleanup(func() { buildHybridRetrieverFn = orig })
	boom := errors.New("Error setting ORT API base: 2")
	buildHybridRetrieverFn = func(context.Context, *index.DB) (retrieval.Retriever, embedding.Embedder, func(), error) {
		return nil, nil, nil, boom
	}

	res := BuildAutoRetrieverFull(context.Background(), embeddedTestDB(t))
	require.ErrorIs(t, res.EmbedderErr, boom, "the reason must travel with the fallback")
	assert.Nil(t, res.Embedder)
	_, isFTS := res.Retriever.(*FTSRetriever)
	assert.True(t, isFTS, "still answers, keyword-only")
}

func TestBuildAutoRetrieverFull_NoEmbeddingsIsNotAnEmbedderFailure(t *testing.T) {
	db, err := index.Open(t.TempDir() + "/idx.db")
	require.NoError(t, err)
	defer db.Close()
	res := BuildAutoRetrieverFull(context.Background(), db)
	assert.NoError(t, res.EmbedderErr, "a vault never embedded is not a broken runtime")
}

func TestWriteEmbedderDownNotice(t *testing.T) {
	var buf bytes.Buffer
	assert.False(t, WriteEmbedderDownNotice(&buf, nil))
	assert.Empty(t, buf.String())

	assert.True(t, WriteEmbedderDownNotice(&buf, errors.New("Error setting ORT API base: 2")))
	out := buf.String()
	assert.Contains(t, out, "keyword-only")
	assert.Contains(t, out, "has embeddings")
	assert.Contains(t, out, "Error setting ORT API base: 2", "the real reason, verbatim")
	assert.Contains(t, out, "vaultmind doctor")
	assert.NotContains(t, out, "index --embed", "re-embedding fails the same way — do not send them there")
}
