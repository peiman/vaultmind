package query

import (
	"context"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
)

// FilteredRetriever applies fixed type and tag filters to every search. A
// command scopes its retriever once instead of threading filters through each
// path that searches — ask alone searches from the direct, --read and
// federated paths. A filter the caller passes explicitly takes precedence.
type FilteredRetriever struct {
	Base    retrieval.Retriever
	Filters index.SearchFilters
}

// Search runs the base search with the fixed filters filling any the caller
// left empty.
func (f FilteredRetriever) Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	if filters.Type == "" {
		filters.Type = f.Filters.Type
	}
	if filters.Tag == "" {
		filters.Tag = f.Filters.Tag
	}
	return f.Base.Search(ctx, query, limit, offset, filters)
}
