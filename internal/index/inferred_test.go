package index_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripForAliasMatch_CodeFence(t *testing.T) {
	body := "before\n```\nRetry Engine code\n```\nafter"
	result := index.StripForAliasMatch(body)
	assert.NotContains(t, result, "Retry Engine code")
	assert.Contains(t, result, "before")
	assert.Contains(t, result, "after")
}

func TestStripForAliasMatch_InlineCode(t *testing.T) {
	body := "see `Retry Engine` for details"
	result := index.StripForAliasMatch(body)
	assert.NotContains(t, result, "Retry Engine")
	assert.Contains(t, result, "see")
}

func TestStripForAliasMatch_Wikilink(t *testing.T) {
	body := "related to [[Retry Engine]] and more"
	result := index.StripForAliasMatch(body)
	assert.NotContains(t, result, "Retry Engine")
}

func TestStripForAliasMatch_AliasedWikilink(t *testing.T) {
	body := "see [[Retry Engine|the engine]] works"
	result := index.StripForAliasMatch(body)
	assert.Contains(t, result, "the engine")
	assert.NotContains(t, result, "Retry Engine")
}

func TestStripForAliasMatch_HTMLComment(t *testing.T) {
	body := "before <!-- Retry Engine --> after"
	result := index.StripForAliasMatch(body)
	assert.NotContains(t, result, "Retry Engine")
}

func TestStripForAliasMatch_PreservesNormalText(t *testing.T) {
	body := "The Retry Engine handles transient failures gracefully."
	result := index.StripForAliasMatch(body)
	assert.Equal(t, body, result)
}

func TestStripForAliasMatch_LanguageFence(t *testing.T) {
	body := "before\n```go\nRetry Engine\n```\nafter"
	result := index.StripForAliasMatch(body)
	assert.NotContains(t, result, "Retry Engine")
}

func TestComputeAliasMentions_Basic(t *testing.T) {
	db := buildIndexedDB(t)
	count, err := index.ComputeAliasMentions(db, 3)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 0)
}

func TestComputeAliasMentions_SkipsShortAliases(t *testing.T) {
	db := buildIndexedDB(t)
	count, err := index.ComputeAliasMentions(db, 1000)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestComputeAliasMentions_ClearsOldEdges(t *testing.T) {
	db := buildIndexedDB(t)
	count1, err := index.ComputeAliasMentions(db, 3)
	require.NoError(t, err)
	count2, err := index.ComputeAliasMentions(db, 3)
	require.NoError(t, err)
	assert.Equal(t, count1, count2)
}

func TestComputeAliasMentions_EdgesInLinksTable(t *testing.T) {
	db := buildIndexedDB(t)
	_, err := index.ComputeAliasMentions(db, 3)
	require.NoError(t, err)
	var edgeCount int
	err = db.QueryRow("SELECT COUNT(*) FROM links WHERE edge_type = 'alias_mention'").Scan(&edgeCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, edgeCount, 0)
}
