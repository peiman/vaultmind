package index_test

import (
	"encoding/json"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFTSResult_JSONFieldNames(t *testing.T) {
	// SRS-09 requires: id, type, title, path, snippet, score, is_domain_note
	result := index.FTSResult{
		ID:       "concept-act-r",
		Type:     "concept",
		Title:    "ACT-R",
		Path:     "concepts/act-r.md",
		Snippet:  "cognitive architecture",
		Score:    0.92,
		IsDomain: true,
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))

	// Must use SRS-09 field names
	assert.Contains(t, parsed, "id", "must use 'id' not 'note_id'")
	assert.Contains(t, parsed, "type")
	assert.Contains(t, parsed, "path")
	assert.Contains(t, parsed, "score", "must use 'score' not 'rank'")
	assert.Contains(t, parsed, "is_domain_note")
	assert.NotContains(t, parsed, "note_id", "SRS uses 'id' not 'note_id'")
	assert.NotContains(t, parsed, "rank", "SRS uses 'score' not 'rank'")
}

func TestSearchFTS_ReturnsFullFields(t *testing.T) {
	db := rebuildTestIndex(t)

	results, err := index.SearchFTS(db, "cognitive architecture", 20, 0)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	r := results[0]
	assert.NotEmpty(t, r.ID, "must have id")
	assert.NotEmpty(t, r.Type, "must have type")
	assert.NotEmpty(t, r.Path, "must have path")
	assert.NotEmpty(t, r.Snippet, "must have snippet")
}
