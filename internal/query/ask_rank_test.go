package query

import (
	"context"
	"fmt"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingRetriever struct{}

func (failingRetriever) Search(context.Context, string, int, int, index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	return nil, 0, fmt.Errorf("retriever error")
}

func TestAsk_ReturnsTheSearchError(t *testing.T) {
	_, err := Ask(context.Background(), failingRetriever{}, nil, nil, AskConfig{Query: "q"})
	require.EqualError(t, err, "retriever error")
}

func TestAskFromRank_RefusesAMissingRank(t *testing.T) {
	_, err := AskFromRank(context.Background(), nil, nil, AskConfig{Query: "q"}, nil)
	require.ErrorIs(t, err, errNoPreparedRank)
}

func TestWithholdsRelevanceJudgement_NilResult(t *testing.T) {
	assert.False(t, (*AskResult)(nil).withholdsRelevanceJudgement())
}
