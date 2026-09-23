package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LIVE-OBSERVED (2026-09-23): with a stale ONNX runtime next to the binary,
// every model load failed and ask ran keyword-only — while doctor reported
// "dense 67/67 (bge-m3)" and the session-start hook said "full BGE-M3 hybrid
// recall". Doctor counted stored embeddings; it never asked whether the model
// could load. Starting the runtime (no model) answers that cheaply.

func resetRuntimeCheckMemo() { runtimeCheck.Once = sync.Once{}; runtimeCheck.err = nil }

func withRuntimeCheck(t *testing.T, fn func(context.Context) error) {
	t.Helper()
	orig := checkEmbeddingRuntime
	checkEmbeddingRuntime = fn
	resetRuntimeCheckMemo()
	t.Cleanup(func() { checkEmbeddingRuntime = orig; resetRuntimeCheckMemo() })
}

func TestAnnotateEmbeddingRuntime_RecordsAFailedRuntime(t *testing.T) {
	withRuntimeCheck(t, func(context.Context) error { return errors.New("Error setting ORT API base: 2") })
	emb := &query.DoctorEmbeddings{SemanticReady: true, Model: embedding.ModelBGEM3}
	annotateEmbeddingRuntime(context.Background(), emb, embedding.BackendNameORT)
	assert.Contains(t, emb.RuntimeError, "Error setting ORT API base: 2")
	assert.True(t, emb.SemanticReady, "the embeddings still exist; do not relabel them 'none'")
}

func TestAnnotateEmbeddingRuntime_OnlyWhereItMatters(t *testing.T) {
	calls := 0
	withRuntimeCheck(t, func(context.Context) error { calls++; return errors.New("x") })
	for _, emb := range []*query.DoctorEmbeddings{
		nil,
		{SemanticReady: false},
		{SemanticReady: true, Model: embedding.ModelMiniLM},
	} {
		annotateEmbeddingRuntime(context.Background(), emb, embedding.BackendNameORT)
	}
	annotateEmbeddingRuntime(context.Background(),
		&query.DoctorEmbeddings{SemanticReady: true, Model: embedding.ModelBGEM3}, embedding.BackendNameGo)
	assert.Zero(t, calls, "no BGE-M3 index, or no native runtime: nothing to check")
}

// doctor --all checks many vaults in one process; the runtime is per process.
func TestAnnotateEmbeddingRuntime_ChecksOncePerProcess(t *testing.T) {
	calls := 0
	withRuntimeCheck(t, func(context.Context) error { calls++; return nil })
	for i := 0; i < 3; i++ {
		annotateEmbeddingRuntime(context.Background(),
			&query.DoctorEmbeddings{SemanticReady: true, Model: embedding.ModelBGEM3}, embedding.BackendNameORT)
	}
	assert.Equal(t, 1, calls)
}

func TestWriteEmbeddingStatus_SaysSemanticSearchIsDown(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, &query.DoctorEmbeddings{
		SemanticReady: true, Model: embedding.ModelBGEM3, TotalNotes: 67, DenseCount: 67, SparseCount: 67, ColBERTCount: 67,
		RuntimeError: "Error setting ORT API base: 2",
	}))
	out := buf.String()
	assert.Contains(t, out, "semantic search is DOWN")
	assert.Contains(t, out, "Error setting ORT API base: 2")
}

// Through the real command: a BGE-M3 vault, an ORT build, a runtime that will
// not start. The helper tests above pass even with the doctor call removed —
// verified — so the wiring needs its own test.
func TestDoctorCommand_ReportsADeadRuntime(t *testing.T) {
	withRuntimeCheck(t, func(context.Context) error { return errors.New("Error setting ORT API base: 2") })
	origBackend := doctorBackend
	doctorBackend = func() string { return embedding.BackendNameORT }
	t.Cleanup(func() { doctorBackend = origBackend })

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "n.md"), []byte("---\nid: n\ntype: concept\ntitle: N\n---\nbody\n"), 0o600))
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	v := make([]float32, embedding.BGEM3Dims)
	_, err = db.Exec(`UPDATE notes SET embedding = ?, sparse_embedding = ?, colbert_embedding = ?`,
		index.EncodeEmbedding(v), index.EncodeSparseEmbedding(map[int32]float32{0: 1}), index.EncodeColBERTEmbedding([][]float32{v}))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	buf, _, err := runRootCmd(t, "doctor", "--vault", dir)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "(bge-m3)", "fixture must be a BGE-M3 vault")
	assert.Contains(t, buf.String(), "semantic search is DOWN")
}
