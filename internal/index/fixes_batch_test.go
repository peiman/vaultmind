package index_test

import (
	"encoding/json"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchFTS_SnippetContainsBodyText(t *testing.T) {
	db := rebuildTestIndex(t)

	results, err := index.SearchFTS(db, "cognitive architecture", 5, 0)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	// Snippet should contain body text, not just title
	found := false
	for _, r := range results {
		if r.ID == "concept-act-r" && len(r.Snippet) > len(r.Title) {
			found = true
		}
	}
	assert.True(t, found, "snippet should contain body context, not just title")
}

func TestSearchFTS_ScoreIsNonNegative(t *testing.T) {
	db := rebuildTestIndex(t)

	results, err := index.SearchFTS(db, "memory", 5, 0)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	for _, r := range results {
		assert.GreaterOrEqual(t, r.Score, float64(0), "score must be >= 0 (normalized), not raw negative BM25")
		assert.LessOrEqual(t, r.Score, float64(1), "score must be <= 1 after min-max normalization")
	}
}

func TestTypeDef_JSONTagsSnakeCase(t *testing.T) {
	td := vault.TypeDef{
		Required: []string{"title"},
		Optional: []string{"tags"},
		Statuses: []string{"active"},
		Template: "templates/concept.md",
	}

	data, err := json.Marshal(td)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.Contains(t, parsed, "required", "must use snake_case JSON field names")
	assert.Contains(t, parsed, "optional")
	assert.Contains(t, parsed, "statuses")
	assert.Contains(t, parsed, "template")
	assert.NotContains(t, parsed, "Required", "must not use PascalCase")
}
