package query_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
)

// Scoring-constant SSOT (AGENTS.md). De-duplication's neighbour count is a
// tunable in the retrieval path, so it is locked here: a change must show up as
// a failing diff rather than as quiet drift in what the reader is shown.
func TestArcFinderConstantsSSOT(t *testing.T) {
	assert.Equal(t, 3, query.DefaultNearestArcs,
		"changing this changes how much evidence every arc proposal carries")
}
