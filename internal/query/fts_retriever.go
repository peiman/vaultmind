package query

import (
	"context"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
)

// FTSRetriever wraps SQLite FTS5 search as a Retriever.
type FTSRetriever struct {
	DB *index.DB
}

// Search runs FTS5 search and count, converting results to ScoredResult.
func (r *FTSRetriever) Search(_ context.Context, query string, limit, offset int, filters index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	results, err := index.SearchFTS(r.DB, query, limit, offset, filters)
	if err != nil {
		return nil, 0, err
	}
	total, err := index.CountFTS(r.DB, query, filters)
	if err != nil {
		return nil, 0, err
	}
	scored := make([]retrieval.ScoredResult, len(results))
	for i, fr := range results {
		scored[i] = retrieval.ScoredResult{
			ID: fr.ID, Type: fr.Type, Title: fr.Title,
			Path: fr.Path, Snippet: fr.Snippet,
			Score: fr.Score, IsDomain: fr.IsDomain,
		}
	}
	return scored, total, nil
}
