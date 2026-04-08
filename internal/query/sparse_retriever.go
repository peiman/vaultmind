package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
)

// SparseEmbedFunc produces a sparse vector for a query string.
type SparseEmbedFunc func(ctx context.Context, text string) (map[int32]float32, error)

// SparseRetriever searches by sparse dot-product between query and stored sparse embeddings.
type SparseRetriever struct {
	DB          *index.DB
	EmbedSparse SparseEmbedFunc
}

// Search embeds the query as a sparse vector, computes dot-product similarity against
// all stored sparse embeddings, and returns the top results sorted by score descending.
func (r *SparseRetriever) Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]ScoredResult, int, error) {
	querySparse, err := r.EmbedSparse(ctx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("embedding query (sparse): %w", err)
	}

	all, err := index.LoadAllSparseEmbeddings(r.DB)
	if err != nil {
		return nil, 0, fmt.Errorf("loading sparse embeddings: %w", err)
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
		sim := embedding.SparseDotProduct(querySparse, ne.Sparse)
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
