package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
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
func (r *SparseRetriever) Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	querySparse, err := r.EmbedSparse(ctx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("embedding query (sparse): %w", err)
	}

	// Units, not notes: a long note is scored by each of its sections.
	all, err := index.LoadUnitSparseEmbeddings(r.DB)
	if err != nil {
		return nil, 0, fmt.Errorf("loading sparse embeddings: %w", err)
	}
	if len(all) == 0 {
		return nil, 0, nil
	}

	keep, err := noteFilter(r.DB, filters)
	if err != nil {
		return nil, 0, err
	}

	var results []retrieval.ScoredResult
	for _, ne := range all {
		if !keep(ne.NoteID, ne.Type) {
			continue
		}
		results = append(results, retrieval.ScoredResult{
			ID: ne.NoteID, Section: ne.SectionID, Type: ne.Type, Title: ne.Title,
			Path: ne.Path, Snippet: truncate(ne.BodyText, snippetMaxLen),
			Score: embedding.SparseDotProduct(querySparse, ne.Sparse), IsDomain: ne.IsDomain,
		})
	}

	// Best first, then one hit per note (its best section).
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	out, total := page(firstPerNote(results), limit, offset)
	return out, total, nil
}
