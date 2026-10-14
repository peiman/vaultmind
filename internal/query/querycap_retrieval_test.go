package query_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cappedQueryRetriever is a stub retriever (ADR-003). It records the query
// string it was asked to rank and returns one fixed hit.
type cappedQueryRetriever struct {
	mu    sync.Mutex
	query string
	hit   retrieval.ScoredResult
}

func (r *cappedQueryRetriever) Search(_ context.Context, q string, limit, _ int, _ index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	r.mu.Lock()
	r.query = q
	r.mu.Unlock()
	if limit <= 0 {
		return nil, 0, nil
	}
	return []retrieval.ScoredResult{r.hit}, 1, nil
}

func (r *cappedQueryRetriever) seen() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.query
}

// recordingEmbedder records the text passed to Embed. Spreading activation
// and the federation judge both embed the ranking query, which never goes
// through Retriever.Search.
type recordingEmbedder struct {
	mu   sync.Mutex
	text string
}

func (e *recordingEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.mu.Lock()
	e.text = text
	e.mu.Unlock()
	return []float32{1, 0}, nil
}

func (e *recordingEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := e.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		out[i] = vec
	}
	return out, nil
}

func (e *recordingEmbedder) Dims() int    { return 2 }
func (e *recordingEmbedder) Close() error { return nil }
func (e *recordingEmbedder) seen() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.text
}

func words(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "word"
	}
	return strings.Join(parts, " ")
}

func hybridLanes(t *testing.T) (*query.HybridRetriever, map[string]*cappedQueryRetriever) {
	t.Helper()
	names := []string{"dense", "sparse", "colbert", "fts", "activation"}
	lanes := make(map[string]*cappedQueryRetriever, len(names))
	named := make([]retrieval.NamedRetriever, 0, len(names))
	hit := retrieval.ScoredResult{ID: "note-1", Title: "Note", Score: 1}
	for _, name := range names {
		lane := &cappedQueryRetriever{hit: hit}
		lanes[name] = lane
		named = append(named, retrieval.NamedRetriever{Name: name, Retriever: lane})
	}
	return &query.HybridRetriever{Retrievers: named}, lanes
}

func TestAskHits_RanksEveryLaneOnTheCappedQuery(t *testing.T) {
	hybrid, lanes := hybridLanes(t)
	original := "keep  " + words(70)
	capped := "keep " + words(63) // first word plus 63 more = 64

	res, err := query.AskHits(context.Background(), hybrid, original, 5, 64)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, original, res.Query, "the echoed query stays the original text")
	require.NotNil(t, res.QueryCapped)
	assert.Equal(t, 64, res.QueryCapped.UsedWords)
	assert.Equal(t, 71, res.QueryCapped.TotalWords)
	for name, lane := range lanes {
		assert.Equal(t, capped, lane.seen(), "lane %s", name)
	}
}

func TestAskHits_UnderCapAndOffLeaveTheQueryAlone(t *testing.T) {
	original := "alpha   beta\n\tgamma"

	t.Run("under the cap", func(t *testing.T) {
		hybrid, lanes := hybridLanes(t)
		res, err := query.AskHits(context.Background(), hybrid, original, 5, 64)
		require.NoError(t, err)
		assert.Nil(t, res.QueryCapped)
		assert.Equal(t, original, res.Query)
		for name, lane := range lanes {
			assert.Equal(t, original, lane.seen(), "lane %s", name)
		}
	})

	t.Run("zero is off", func(t *testing.T) {
		hybrid, lanes := hybridLanes(t)
		long := words(80)
		res, err := query.AskHits(context.Background(), hybrid, long, 5, 0)
		require.NoError(t, err)
		assert.Nil(t, res.QueryCapped)
		assert.Equal(t, long, res.Query)
		for name, lane := range lanes {
			assert.Equal(t, long, lane.seen(), "lane %s", name)
		}
	})
}

func TestAsk_EmbedsTheCappedQueryAndEchoesTheOriginal(t *testing.T) {
	db := buildRetrieverTestDB(t)
	lane := &cappedQueryRetriever{hit: retrieval.ScoredResult{ID: "missing-note", Title: "Missing", Score: 1}}
	emb := &recordingEmbedder{}
	original := words(80)

	res, err := query.Ask(context.Background(), lane, graph.NewResolver(db), db, query.AskConfig{
		Query:         original,
		SearchLimit:   5,
		Embedder:      emb,
		QueryMaxWords: 64,
	})
	require.NoError(t, err)
	assert.Equal(t, original, res.Query)
	require.NotNil(t, res.QueryCapped)
	assert.Equal(t, 64, res.QueryCapped.UsedWords)
	assert.Equal(t, 80, res.QueryCapped.TotalWords)
	assert.Equal(t, words(64), lane.seen())
	assert.Equal(t, words(64), emb.seen(), "spreading activation embeds the ranking text")
}

func TestSearchAndJudge_EmbedsTheCappedQuery(t *testing.T) {
	db := buildRetrieverTestDB(t)
	lane := &cappedQueryRetriever{hit: retrieval.ScoredResult{ID: "missing-note", Title: "Missing", Score: 1}}
	emb := &recordingEmbedder{}
	original := words(40)

	_, _, _, err := query.SearchAndJudge(
		context.Background(), lane, emb, db,
		func(int) (float64, float64) { return 0, 0.1 },
		original, 5, 8,
	)
	require.NoError(t, err)
	assert.Equal(t, words(8), lane.seen())
	assert.Equal(t, words(8), emb.seen(), "the federation judge embeds the ranking text")
}
