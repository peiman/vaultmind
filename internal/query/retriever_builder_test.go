package query_test

import (
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRetriever_Keyword(t *testing.T) {
	db := buildRetrieverTestDB(t)
	ret, cleanup, err := query.BuildRetriever("keyword", db)
	require.NoError(t, err)
	assert.Nil(t, cleanup, "keyword mode needs no cleanup")
	assert.IsType(t, &query.FTSRetriever{}, ret)
}

func TestBuildRetriever_EmptyModeDefaultsToKeyword(t *testing.T) {
	db := buildRetrieverTestDB(t)
	ret, cleanup, err := query.BuildRetriever("", db)
	require.NoError(t, err)
	assert.Nil(t, cleanup)
	assert.IsType(t, &query.FTSRetriever{}, ret)
}

func TestBuildRetriever_UnknownMode(t *testing.T) {
	db := buildRetrieverTestDB(t)
	_, _, err := query.BuildRetriever("bogus", db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown search mode")
	assert.Contains(t, err.Error(), "bogus")
}

func TestBuildRetriever_SemanticNoEmbeddings(t *testing.T) {
	db := buildRetrieverTestDB(t)
	_, _, err := query.BuildRetriever("semantic", db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no embeddings found")
}

func TestBuildRetriever_HybridNoEmbeddings(t *testing.T) {
	db := buildRetrieverTestDB(t)
	_, _, err := query.BuildRetriever("hybrid", db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no embeddings found")
}

func TestBuildRetriever_SemanticWithEmbeddings(t *testing.T) {
	// Skip under -race: BuildRetriever's embedder-init path calls
	// into hugot, which calls into github.com/gomlx/go-huggingface.
	// That library has a known data race in (*Repo).DownloadFilesCtx
	// — two goroutines spawned in func3 read+write the same slot in
	// hub/files.go:250. The race fires whenever the model isn't
	// already cached (the CI default). The test ITSELF doesn't care
	// about the embedder succeeding — it explicitly tolerates the
	// download failure (assert.NotContains "no embeddings found"),
	// testing only that BuildRetriever clears the empty-embeddings
	// check first. Skipping under race avoids CI-failing on third-
	// party noise; the non-race build still exercises the code path.
	// Re-enable when hugot updates past the racy go-huggingface or a
	// CI step pre-caches the model.
	if raceEnabled {
		t.Skip("skipping under -race: upstream go-huggingface@v0.3.5 has a known race in DownloadFilesCtx; not vaultmind code")
	}

	db := buildRetrieverTestDB(t)
	// Store a dummy embedding so HasEmbeddings returns true
	row, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row)
	require.NoError(t, index.StoreEmbedding(db, row.ID, []float32{0.1, 0.2, 0.3}))

	// BuildRetriever's embedder-init path may download the embedding model
	// (BGE-M3 is ~2.2 GB). On an uncached CI runner that download can stall
	// for minutes and blow the package test timeout — a flaky failure that has
	// nothing to do with this test's intent (verifying that the embeddings
	// check passes BEFORE embedder init). Bound it: if the embedder doesn't
	// init within the window, skip rather than hang. When the model is cached
	// (local runs, warm CI) the flow is still exercised fully.
	type buildResult struct {
		ret     retrieval.Retriever
		cleanup func()
		err     error
	}
	done := make(chan buildResult, 1)
	go func() {
		r, c, e := query.BuildRetriever("semantic", db)
		done <- buildResult{r, c, e}
	}()

	select {
	case res := <-done:
		// If it errors, it should be about creating the embedder, not about
		// embeddings — we're testing the flow, not the embedder init.
		if res.err != nil {
			assert.NotContains(t, res.err.Error(), "no embeddings found",
				"should pass the embeddings check and fail on embedder init instead")
			return
		}
		assert.IsType(t, &query.EmbeddingRetriever{}, res.ret)
		if res.cleanup != nil {
			res.cleanup()
		}
	case <-time.After(90 * time.Second):
		t.Skip("embedder model init exceeded 90s (slow/uncached model download in this environment); flow-ordering check skipped")
	}
}
