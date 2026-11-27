package memory_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArcSections_RecognizesHeadingVariantsAndPreservesOrder(t *testing.T) {
	body := `## The Trigger

The trigger text.

## Push

The push text.

## The trigger

This is not a second trigger section.

Deeper sight

The deeper sight text.

### The Principle

The principle text.`

	sections := memory.ExtractArcSections(body)
	require.Len(t, sections, 4)
	assert.Equal(t, "trigger", sections[0].Name)
	assert.Equal(t, "## The Trigger", sections[0].Heading)
	assert.Equal(t, "The trigger text.", sections[0].Text)
	assert.Equal(t, "push", sections[1].Name)
	assert.Equal(t, "deeper-sight", sections[2].Name)
	assert.Equal(t, "### The Principle", sections[3].Heading)
	assert.Equal(t, "principle", sections[3].Name)
	assert.Equal(t, "The principle text.", sections[3].Text)
}

func TestArcSections_StripsFrontmatterAndHTMLComments(t *testing.T) {
	body := `---
id: arc-example
type: arc
---
<!-- vaultmind:accepted proposal -->
## Trigger

Visible trigger.

## Principle

Visible principle.`

	sections := memory.ExtractArcSections(body)
	require.Len(t, sections, 2)
	assert.NotContains(t, sections[0].Text, "proposal")
	assert.Equal(t, "Visible principle.", sections[1].Text)
}

func TestParseArcSectionSelection_RejectsUnknownNames(t *testing.T) {
	got, err := memory.ParseArcSectionSelection("trigger,deeper-sight,principle")
	require.NoError(t, err)
	assert.Equal(t, []string{"trigger", "deeper-sight", "principle"}, got)

	_, err = memory.ParseArcSectionSelection("trigger,unknown")
	assert.EqualError(t, err, `unknown arc section "unknown" (want trigger, push, principle, or deeper-sight)`)
}

func TestPlanArcShards_IsDeterministicByteBalancedAndComplete(t *testing.T) {
	weights := []int{40, 80, 45, 120, 55, 60}
	first, err := memory.PlanArcShards(weights, 3)
	require.NoError(t, err)
	second, err := memory.PlanArcShards(weights, 3)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Equal(t, [][]int{{0, 1}, {2, 3}, {4, 5}}, first)

	seen := make([]int, 0, len(weights))
	for _, shard := range first {
		seen = append(seen, shard...)
	}
	assert.Equal(t, []int{0, 1, 2, 3, 4, 5}, seen)
	assert.InDelta(t, 460.0/3.0, float64(weights[0]+weights[1]), 70)
}

func TestPlanArcShards_ValidatesShardIndexAndCount(t *testing.T) {
	assert.NoError(t, memory.ValidateShard(1, 3))
	assert.EqualError(t, memory.ValidateShard(0, 3), "shard index must be between 1 and 3")
	assert.EqualError(t, memory.ValidateShard(4, 3), "shard index must be between 1 and 3")
	assert.EqualError(t, memory.ValidateShard(1, 0), "shard count must be positive")
}

func TestRecite_SectionsUseFullSelectedTextAndFallbackForOtherNotes(t *testing.T) {
	db := buildTestDB(t)
	got, err := memory.Recite(db, memory.ReciteConfig{Type: "concept", Sections: []string{"trigger", "principle"}})
	require.NoError(t, err)
	require.NotEmpty(t, got.Items)
	assert.True(t, got.Items[0].Fallback)
	assert.Empty(t, got.Items[0].Sections)
	assert.Contains(t, got.Items[0].Excerpt, "#")
}

func TestRenderRecitation_SectionsAndShardHeaderStayWithinMaxBytes(t *testing.T) {
	r := &memory.ReciteResult{
		Type: "arc", Total: 1, Budget: 0,
		Shard: &memory.ReciteShard{Index: 2, Count: 3},
		Items: []memory.ReciteItem{{
			ID: "arc-one", Title: "One", Excerpt: "## Trigger\n\nA trigger.\n\n## Principle\n\nA rule.",
			Sections: []memory.ReciteSection{{Name: "trigger", Heading: "## Trigger", Text: "A trigger."}, {Name: "principle", Heading: "## Principle", Text: "A rule."}},
		}},
	}
	out := memory.RenderRecitation(r)
	assert.Contains(t, out, "arcs part 2 of 3")
	assert.Contains(t, out, "## Trigger")
	assert.Contains(t, out, "## Principle")
}
