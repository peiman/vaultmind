package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Named priority tier constants express the ordering intent of
// edgePriority so a reader doesn't have to chase the function body to
// know what `return 2` means. These tests double as regression guards:
// if someone reorders the tiers, the diff surfaces the change explicitly.

func TestPriorityTiers_OrderedAscending(t *testing.T) {
	// Lower = higher priority; tier ordering IS the ranking contract.
	// If this ordering ever changes, downstream context-pack ranking
	// changes too — the diff must be deliberate, not accidental.
	assert.Less(t, PriorityExplicitRelation, PriorityExplicitLink)
	assert.Less(t, PriorityExplicitLink, PriorityMediumConfidence)
	assert.Less(t, PriorityMediumConfidence, PriorityLowConfidence)
}

func TestPriorityTiers_HaveExpectedNumericValues(t *testing.T) {
	// The numeric values also feed a distance-weighted computation
	// elsewhere (distance*10 + priority). Changing them silently shifts
	// that weighting. Lock the numbers.
	assert.Equal(t, 0, PriorityExplicitRelation)
	assert.Equal(t, 1, PriorityExplicitLink)
	assert.Equal(t, 2, PriorityMediumConfidence)
	assert.Equal(t, 3, PriorityLowConfidence)
}

func TestEdgePriority_MapsEdgeTypesToTiers(t *testing.T) {
	assert.Equal(t, PriorityExplicitRelation, edgePriority("explicit_relation", "high"))
	assert.Equal(t, PriorityExplicitLink, edgePriority("explicit_link", "high"))
	assert.Equal(t, PriorityExplicitLink, edgePriority("explicit_embed", "high"))
	assert.Equal(t, PriorityMediumConfidence, edgePriority("alias_mention", "medium"))
	assert.Equal(t, PriorityLowConfidence, edgePriority("tag_overlap", "low"))
}
