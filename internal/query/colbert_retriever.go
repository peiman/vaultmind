package query

import (
	"context"
	"fmt"
	"runtime"
	"sort"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
)

// ColBERTEmbedFunc produces per-token ColBERT vectors for a query string.
type ColBERTEmbedFunc func(ctx context.Context, text string) ([][]float32, error)

// ColBERTRetriever searches by MaxSim scoring between query and stored ColBERT token matrices.
type ColBERTRetriever struct {
	DB           *index.DB
	EmbedColBERT ColBERTEmbedFunc
	Dims         int
}

// Search embeds the query as per-token ColBERT vectors, computes MaxSim scoring against
// all stored ColBERT embeddings, and returns the top results sorted by score descending.
func (r *ColBERTRetriever) Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	queryTokens, err := r.EmbedColBERT(ctx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("embedding query (ColBERT): %w", err)
	}

	all, err := index.LoadAllColBERTEmbeddings(r.DB, r.Dims)
	if err != nil {
		return nil, 0, fmt.Errorf("loading ColBERT embeddings: %w", err)
	}
	return r.score(queryTokens, all, limit, offset, filters)
}

// SearchAmong scores only the notes named in ids, reading only their vectors.
// The hybrid retriever passes the other lanes' results (see CandidateSearcher).
func (r *ColBERTRetriever) SearchAmong(ctx context.Context, query string, limit int, filters index.SearchFilters, ids []string) ([]retrieval.ScoredResult, int, error) {
	queryTokens, err := r.EmbedColBERT(ctx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("embedding query (ColBERT): %w", err)
	}
	some, err := index.LoadColBERTEmbeddingsFor(r.DB, r.Dims, ids)
	if err != nil {
		return nil, 0, err
	}
	return r.score(queryTokens, some, limit, 0, filters)
}

// score ranks notes by MaxSim against the query tokens.
func (r *ColBERTRetriever) score(queryTokens [][]float32, all []index.NoteColBERTEmbedding, limit, offset int, filters index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	if len(all) == 0 {
		return nil, 0, nil
	}

	keep, err := noteFilter(r.DB, filters)
	if err != nil {
		return nil, 0, err
	}

	type scored struct {
		result retrieval.ScoredResult
		score  float64
	}
	var kept []index.NoteColBERTEmbedding
	var docs [][][]float32
	for _, ne := range all {
		if keep(ne.NoteID, ne.Type) {
			kept = append(kept, ne)
			docs = append(docs, ne.ColBERT)
		}
	}
	sims := embedding.MaxSimAll(queryTokens, docs, runtime.GOMAXPROCS(0))

	results := make([]scored, 0, len(kept))
	for i, ne := range kept {
		sim := sims[i]
		results = append(results, scored{
			result: retrieval.ScoredResult{
				ID: ne.NoteID, Type: ne.Type, Title: ne.Title,
				Path: ne.Path, Snippet: truncate(ne.BodyText, snippetMaxLen),
				Score: sim, IsDomain: ne.IsDomain,
			},
			score: sim,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	total := len(results)
	if offset >= len(results) {
		return nil, total, nil
	}
	results = results[offset:]
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	out := make([]retrieval.ScoredResult, len(results))
	for i, s := range results {
		out[i] = s.result
	}
	return out, total, nil
}
