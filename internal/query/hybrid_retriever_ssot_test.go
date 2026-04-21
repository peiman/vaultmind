package query_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
)

// DefaultRRFK is the Reciprocal Rank Fusion smoothing constant applied
// when the HybridRetriever's K field is zero-value. 60 is the widely-cited
// default from the original RRF paper; changing it here affects every
// hybrid retrieval and would shift the baseline. Locked in a regression
// test so any change shows up in the diff.
func TestDefaultRRFK_IsLockedAt60(t *testing.T) {
	assert.Equal(t, 60, query.DefaultRRFK,
		"DefaultRRFK must be 60 — original RRF paper's default; changing this shifts every hybrid retrieval")
}
