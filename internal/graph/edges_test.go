package graph_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinksOut_ReturnsOutboundEdges(t *testing.T) {
	db := buildTestDB(t)

	links, err := graph.LinksOut(db, "concept-act-r", "")
	require.NoError(t, err)
	assert.NotEmpty(t, links)

	// ACT-R has body wikilinks and frontmatter relations
	var edgeTypes []string
	for _, l := range links {
		edgeTypes = append(edgeTypes, l.EdgeType)
	}
	assert.Contains(t, edgeTypes, "explicit_link")
	assert.Contains(t, edgeTypes, "explicit_relation")
}

func TestLinksOut_FilterByEdgeType(t *testing.T) {
	db := buildTestDB(t)

	links, err := graph.LinksOut(db, "concept-act-r", "explicit_relation")
	require.NoError(t, err)

	for _, l := range links {
		assert.Equal(t, "explicit_relation", l.EdgeType)
	}
}

func TestLinksIn_ReturnsInboundEdges(t *testing.T) {
	db := buildTestDB(t)

	// concept-act-r has explicit_relation edges from frontmatter (related_ids)
	// pointing to concept-spreading-activation — these have dst_raw but NULL dst_note_id.
	// LinksIn via frontmatter relations: concept-spreading-activation is referenced
	// in related_ids of concept-act-r. But body wikilinks are not yet resolved.
	// Test with explicit_relation edges which DO have the target ID as dst_raw.
	// We need to also search by dst_raw for unresolved links.
	// concept-spreading-activation is in related_ids of concept-act-r,
	// stored as dst_raw = "concept-spreading-activation"
	links, err := graph.LinksIn(db, "concept-spreading-activation", "")
	require.NoError(t, err)
	assert.NotEmpty(t, links, "should find inbound links via dst_raw matching")
}

func TestLinksOut_EmptyForNonexistent(t *testing.T) {
	db := buildTestDB(t)

	links, err := graph.LinksOut(db, "nonexistent", "")
	require.NoError(t, err)
	assert.Empty(t, links)
}
