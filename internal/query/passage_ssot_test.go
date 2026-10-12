package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// These are the frozen lexical policy, independent of delivery fixtures.
func TestPassageScoringConstantsSSOT(t *testing.T) {
	assert.Equal(t, 2.0, passageFirstBlockMargin)
	assert.Equal(t, 1.2, passageBM25K1)
	assert.Equal(t, 0.75, passageBM25B)
}
