package query_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsk_ReturnsHitsAndContext(t *testing.T) {
	db := buildIndexedDB(t)
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)

	result, err := query.Ask(retriever, resolver, db, query.AskConfig{
		Query:       "memory",
		Budget:      4000,
		MaxItems:    8,
		SearchLimit: 5,
	})

	require.NoError(t, err)
	assert.Equal(t, "memory", result.Query)
	assert.NotEmpty(t, result.TopHits)
	assert.LessOrEqual(t, len(result.TopHits), 5)
	// Context-pack from the top hit should be populated
	assert.NotNil(t, result.Context)
}

func TestAsk_EmptyQuery(t *testing.T) {
	db := buildIndexedDB(t)
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)

	result, err := query.Ask(retriever, resolver, db, query.AskConfig{
		Query:       "",
		Budget:      4000,
		MaxItems:    8,
		SearchLimit: 5,
	})

	require.NoError(t, err)
	assert.Equal(t, "", result.Query)
	assert.Empty(t, result.TopHits)
	assert.Nil(t, result.Context)
}

func TestAsk_NoHitsGivesNilContext(t *testing.T) {
	db := buildIndexedDB(t)
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)

	result, err := query.Ask(retriever, resolver, db, query.AskConfig{
		Query:       "xyzzy_nonexistent_7329",
		Budget:      4000,
		MaxItems:    8,
		SearchLimit: 5,
	})

	require.NoError(t, err)
	assert.Empty(t, result.TopHits)
	assert.Nil(t, result.Context)
}

func TestAsk_LimitsHitsToSearchLimit(t *testing.T) {
	db := buildIndexedDB(t)
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)

	result, err := query.Ask(retriever, resolver, db, query.AskConfig{
		Query:       "memory",
		Budget:      4000,
		MaxItems:    8,
		SearchLimit: 3,
	})

	require.NoError(t, err)
	assert.LessOrEqual(t, len(result.TopHits), 3)
}
