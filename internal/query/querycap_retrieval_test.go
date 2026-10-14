package query_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cappedQueryRetriever is a stub retriever (ADR-003). It records the query
// string it was asked to rank and returns one fixed hit. Activation ignores
// that string; the argument staying original is what this stub records.
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

// recordingEmbedder records the text passed to Embed. The cosine that feeds
// spreading activation embeds the forward-bounded text, which never goes
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

type recordingSparse struct {
	mu   sync.Mutex
	text string
}

func (s *recordingSparse) embed(_ context.Context, text string) (map[int32]float32, error) {
	s.mu.Lock()
	s.text = text
	s.mu.Unlock()
	return map[int32]float32{1: 1}, nil
}

func (s *recordingSparse) seen() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text
}

func words(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "word"
	}
	return strings.Join(parts, " ")
}

func TestAskHits_SearchArgumentStaysTheOriginalQuery(t *testing.T) {
	lane := &cappedQueryRetriever{hit: retrieval.ScoredResult{ID: "note-1", Title: "Note", Score: 1}}
	original := "keep  " + words(70)

	res, err := query.AskHits(context.Background(), lane, original, 5, 64, 0, nil)
	require.NoError(t, err)
	assert.Equal(t, original, res.Query)
	assert.Equal(t, original, lane.seen(), "activation and any lane that ignores the plan still see the full query")
	require.NotNil(t, res.QueryCapped)
	assert.Equal(t, 64, res.QueryCapped.EmbeddedWords)
	assert.Equal(t, 71, res.QueryCapped.TotalWords)
	assert.Equal(t, 0, res.QueryCapped.SalientTerms)
}

func TestAskHits_UnderTheBoundsLeavesTheQueryAlone(t *testing.T) {
	original := "alpha   beta\n\tgamma"
	lane := &cappedQueryRetriever{hit: retrieval.ScoredResult{ID: "note-1", Title: "Note", Score: 1}}

	res, err := query.AskHits(context.Background(), lane, original, 5, 512, 64, nil)
	require.NoError(t, err)
	assert.Nil(t, res.QueryCapped)
	assert.Equal(t, original, res.Query)
	assert.Equal(t, original, lane.seen())
}

func TestAskHits_EmbedsTheForwardTextAndSearchesSalientTerms(t *testing.T) {
	db := buildRetrieverTestDB(t)
	dense := &recordingEmbedder{}
	sparse := &recordingSparse{}
	analyzer := &weightAnalyzer{tokens: embedding.QueryTokens{
		Weights: []float32{0.1, 9, 0.2, 8},
		Word:    []int{0, 1, 2, 3},
		Words:   []string{"one", "episodic", "three", "four"},
	}}
	original := "one episodic three four five"

	_, err := query.AskHits(context.Background(), &query.EmbeddingRetriever{DB: db, Embedder: dense}, original, 5, 4, 1, analyzer)
	require.NoError(t, err)
	assert.Equal(t, "one episodic three four", dense.seen(), "dense embeds the forward-bounded text")

	_, err = query.AskHits(context.Background(), &query.SparseRetriever{DB: db, EmbedSparse: sparse.embed}, original, 5, 4, 1, analyzer)
	require.NoError(t, err)
	assert.Equal(t, "one episodic three four", sparse.seen(), "sparse embeds the same forward-bounded text")

	fts, err := query.AskHits(context.Background(), &query.FTSRetriever{DB: db}, original, 5, 4, 1, analyzer)
	require.NoError(t, err)
	require.NotEmpty(t, fts.TopHits)
	assert.Equal(t, "concept-episodic-memory", fts.TopHits[0].ID, "FTS searches the highest-weight word")
	for _, hit := range fts.TopHits {
		assert.NotEqual(t, "concept-spreading-activation", hit.ID)
	}
}

func TestAsk_EmbedsTheForwardTextAndEchoesTheOriginal(t *testing.T) {
	db := buildRetrieverTestDB(t)
	lane := &cappedQueryRetriever{hit: retrieval.ScoredResult{ID: "missing-note", Title: "Missing", Score: 1}}
	emb := &recordingEmbedder{}
	original := words(8) + " TAIL"

	res, err := query.Ask(context.Background(), lane, graph.NewResolver(db), db, query.AskConfig{
		Query:             original,
		SearchLimit:       5,
		Embedder:          emb,
		QueryForwardWords: 4,
		QuerySalientTerms: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, original, res.Query)
	require.NotNil(t, res.QueryCapped)
	assert.Equal(t, 4, res.QueryCapped.EmbeddedWords)
	assert.Equal(t, 9, res.QueryCapped.TotalWords)
	assert.Equal(t, 0, res.QueryCapped.SalientTerms, "this embedder has no sparse weights")
	assert.Equal(t, original, lane.seen())
	assert.Equal(t, words(4), emb.seen(), "the cosine embeds the forward text, not a salient subset")
}

func TestSearchAndJudge_EmbedsTheForwardText(t *testing.T) {
	db := buildRetrieverTestDB(t)
	lane := &cappedQueryRetriever{hit: retrieval.ScoredResult{ID: "missing-note", Title: "Missing", Score: 1}}
	emb := &recordingEmbedder{}
	original := words(6)

	_, _, _, err := query.SearchAndJudge(
		context.Background(), lane, emb, db,
		func(int) (float64, float64) { return 0, 0.1 },
		original, 5, 3, 0,
	)
	require.NoError(t, err)
	assert.Equal(t, original, lane.seen())
	assert.Equal(t, words(3), emb.seen())
}

func TestAskHits_ColBERTRanksOnTheHighestWeightToken(t *testing.T) {
	db := buildRetrieverTestDB(t)
	low, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	high, err := db.QueryNoteByPath("concepts/episodic-memory.md")
	require.NoError(t, err)
	require.NoError(t, index.StoreColBERTEmbedding(db, low.ID, [][]float32{{1, 0}}))
	require.NoError(t, index.StoreColBERTEmbedding(db, high.ID, [][]float32{{0, 1}}))

	embed := func(_ context.Context, text string) ([][]float32, error) {
		assert.Equal(t, "one episodic", text)
		return [][]float32{{1, 0}, {0, 1}}, nil
	}
	analyzer := &weightAnalyzer{tokens: embedding.QueryTokens{
		Weights: []float32{0.1, 9},
		Word:    []int{0, 1},
		Words:   []string{"one", "episodic"},
	}}
	ret := &query.ColBERTRetriever{DB: db, EmbedColBERT: embed, Dims: 2}

	res, err := query.AskHits(context.Background(), ret, "one episodic", 5, 0, 1, analyzer)
	require.NoError(t, err)
	require.NotEmpty(t, res.TopHits)
	assert.Equal(t, high.ID, res.TopHits[0].ID, "the kept token is the high-weight one")
}
