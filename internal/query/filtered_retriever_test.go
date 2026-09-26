package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingRetriever struct{ got index.SearchFilters }

func (r *recordingRetriever) Search(_ context.Context, _ string, _, _ int, f index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	r.got = f
	return nil, 0, nil
}

func TestFilteredRetriever_FillsEmptyFiltersAndKeepsTheCallers(t *testing.T) {
	base := &recordingRetriever{}
	r := query.FilteredRetriever{Base: base, Filters: index.SearchFilters{Type: "decision", Tag: "engineering"}}

	_, _, err := r.Search(context.Background(), "q", 5, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, index.SearchFilters{Type: "decision", Tag: "engineering"}, base.got, "empty filters take the fixed ones")

	_, _, err = r.Search(context.Background(), "q", 5, 0, index.SearchFilters{Type: "concept"})
	require.NoError(t, err)
	assert.Equal(t, index.SearchFilters{Type: "concept", Tag: "engineering"}, base.got, "a filter the caller passes wins")
}
