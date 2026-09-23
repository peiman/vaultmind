package embedding_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/require"
)

// STRANGER TEST (2026-09-23), a `go install` (MiniLM) build indexing 60 notes
// from a research vault: "Embedded 32 notes (0 skipped, 28 errors)". The model
// was handed 639 tokens against a 512-position table:
//
//	got shapes (Float32)[28 639 384] and (Float32)[1 512 384]
//
// #69 relaxed the char pre-cut from 2 to 4 chars/token on the premise that an
// exact token count and a tokenizer clamp stood behind it. Both existed only in
// the BGE-M3 embedder. For MiniLM the estimate was the ONLY guard, and
// citation-dense notes (DOIs, arXiv ids, URLs) tokenize well under 4 chars per
// token. It was verified on BGE-M3 and never on MiniLM.
//
// Configured exactly as production is (DefaultMaxTokens), and batched with
// short notes, because one oversized row fails the WHOLE batch.
func TestHugotEmbedder_DenseLongNoteStillEmbeds(t *testing.T) {
	if os.Getenv("VAULTMIND_TEST_EMBEDDING") == "" {
		t.Skip("skipping embedding test (set VAULTMIND_TEST_EMBEDDING=1 to run, downloads ~90MB model)")
	}
	embedder, err := embedding.NewHugotEmbedder(context.Background(), embedding.HugotConfig{
		ModelName:    testModelName,
		CacheDir:     t.TempDir(),
		Dims:         testModelDims,
		OnnxFilePath: "onnx/model.onnx",
		MaxTokens:    embedding.DefaultMaxTokens,
	})
	require.NoError(t, err)
	defer func() { _ = embedder.Close() }()

	// Citation-dense prose, ~2-3 chars/token: well past 512 tokens once the
	// 4-chars/token pre-cut (2040 chars) lets it through.
	var b strings.Builder
	for i := 0; b.Len() < 6000; i++ {
		b.WriteString("See arXiv:2104.08663v4, doi:10.1145/3397271.3401075 and https://aclanthology.org/2020.emnlp-main.550/ (BM25; α=0.7, k₁=1.2). ")
	}
	texts := []string{"A short note about memory.", b.String(), "Another short one."}

	vecs, err := embedder.EmbedBatch(context.Background(), texts)
	require.NoError(t, err, "a dense long note must be shrunk to fit, not fail its batch")
	require.Len(t, vecs, len(texts))
	for i, v := range vecs {
		require.Len(t, v, testModelDims, "text %d", i)
	}
}

// hugot 0.7.8 copies downloads into <cache>/<model>/ without creating it, so
// VaultMind creates it first (#148). It must be the SAME directory hugot uses,
// or the first download still fails — including the ":revision" form.
func TestHugotModelDir_MatchesHugotLayout(t *testing.T) {
	require.Equal(t, "/c/sentence-transformers_all-MiniLM-L6-v2",
		embedding.HugotModelDirForTest("/c", "sentence-transformers/all-MiniLM-L6-v2"))
	require.Equal(t, "/c/BAAI_bge-m3",
		embedding.HugotModelDirForTest("/c", "BAAI/bge-m3:main"))
}
