package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
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
func (r *ColBERTRetriever) Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]ScoredResult, int, error) {
	queryTokens, err := r.EmbedColBERT(ctx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("embedding query (ColBERT): %w", err)
	}

	all, err := index.LoadAllColBERTEmbeddings(r.DB, r.Dims)
	if err != nil {
		return nil, 0, fmt.Errorf("loading ColBERT embeddings: %w", err)
	}
	if len(all) == 0 {
		return nil, 0, nil
	}

	type scored struct {
		result ScoredResult
		score  float64
	}
	var results []scored
	for _, ne := range all {
		if filters.Type != "" && ne.Type != filters.Type {
			continue
		}
		sim := embedding.MaxSimScore(queryTokens, ne.ColBERT)
		results = append(results, scored{
			result: ScoredResult{
				ID: ne.NoteID, Type: ne.Type, Title: ne.Title,
				Path: ne.Path, Score: sim, IsDomain: ne.IsDomain,
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

	out := make([]ScoredResult, len(results))
	for i, s := range results {
		out[i] = s.result
	}
	return out, total, nil
}
