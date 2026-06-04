package graph_test

import (
	"encoding/json"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinksOut_JSONShape(t *testing.T) {
	db := buildTestDB(t)

	links, err := graph.LinksOut(db, "concept-act-r", "")
	require.NoError(t, err)
	require.NotEmpty(t, links)

	data, err := json.Marshal(links[0])
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))

	// Outbound links must have target_* fields, NOT source_*
	assert.Contains(t, parsed, "edge_type")
	assert.Contains(t, parsed, "confidence")

	// Should NOT have source fields on outbound links
	_, hasSourceID := parsed["source_id"]
	assert.False(t, hasSourceID, "outbound links must not have source_id")
}

func TestLinksIn_JSONShape(t *testing.T) {
	db := buildTestDB(t)

	links, err := graph.LinksIn(db, "concept-spreading-activation", "")
	require.NoError(t, err)
	require.NotEmpty(t, links)

	data, err := json.Marshal(links[0])
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))

	// Inbound links must have source_* fields
	assert.Contains(t, parsed, "source_id")
	assert.Contains(t, parsed, "source_title")
	assert.Contains(t, parsed, "source_path")
	assert.Contains(t, parsed, "edge_type")

	// Should NOT have target fields on inbound links
	_, hasTargetID := parsed["target_id"]
	assert.False(t, hasTargetID, "inbound links must not have target_id")
}
