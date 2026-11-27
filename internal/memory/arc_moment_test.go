package memory_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
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

## Deeper sight

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
<!-- vaultmind:accepted proposal
## Push
comment-only heading -->
## Trigger

Visible trigger.

## Principle

Visible principle.`

	sections := memory.ExtractArcSections(body)
	require.Len(t, sections, 2)
	assert.NotContains(t, sections[0].Text, "proposal")
	assert.NotContains(t, sections[0].Text, "comment-only heading")
	for _, section := range sections {
		assert.NotEqual(t, "push", section.Name)
	}
	assert.Equal(t, "Visible principle.", sections[1].Text)
}

func TestArcSections_IgnoresHeadingsInsideBacktickAndTildeFences(t *testing.T) {
	body := "## Trigger\n\n" +
		"Visible trigger before the example.\n\n" +
		"```\n## The Principle\n\nThis is fenced text, not a section.\n```\n\n" +
		"Visible trigger after the example.\n\n" +
		"## Principle\n\nThe real principle.\n\n" +
		"~~~\n## Push\n\nThis is also fenced text.\n~~~"

	sections := memory.ExtractArcSections(body)
	require.Len(t, sections, 2)
	assert.Equal(t, "trigger", sections[0].Name)
	assert.Contains(t, sections[0].Text, "This is fenced text, not a section.")
	assert.Contains(t, sections[0].Text, "Visible trigger after the example.")
	assert.Equal(t, "principle", sections[1].Name)
}

func TestArcSections_BareThePrincipleIsNotAHeading(t *testing.T) {
	sections := memory.ExtractArcSections("## Trigger\n\nTrigger text.\n\nThe Principle\n\nThis remains trigger text.")
	require.Len(t, sections, 1)
	assert.Equal(t, "trigger", sections[0].Name)
	assert.Contains(t, sections[0].Text, "The Principle")
}

func TestPrincipleSectionAndExcerptPreserveOriginBareHeadingBehavior(t *testing.T) {
	body := "## The Principle\n\nThis is prose under a non-principle heading."
	assert.Empty(t, memory.PrincipleSection(body))
	assert.Equal(t, "This is prose under a non-principle heading.", memory.Excerpt(body, 100))
}

func TestParseArcSectionSelection_RejectsUnknownNames(t *testing.T) {
	got, err := memory.ParseArcSectionSelection("trigger,deeper-sight,principle")
	require.NoError(t, err)
	assert.Equal(t, []string{"trigger", "deeper-sight", "principle"}, got)

	_, err = memory.ParseArcSectionSelection("trigger,unknown")
	assert.EqualError(t, err, `unknown arc section "unknown" (want all, trigger, push, principle, or deeper-sight)`)
}

func TestParseArcSectionSelection_AllCannotBeCombined(t *testing.T) {
	got, err := memory.ParseArcSectionSelection("all")
	require.NoError(t, err)
	assert.Equal(t, []string{"all"}, got)

	_, err = memory.ParseArcSectionSelection("all,push")
	assert.EqualError(t, err, `arc section "all" cannot be combined with other sections`)
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
	maxShardWeight := 0
	for _, shard := range first {
		weight := 0
		for _, index := range shard {
			weight += weights[index]
		}
		if weight > maxShardWeight {
			maxShardWeight = weight
		}
	}
	assert.LessOrEqual(t, maxShardWeight, 460/3+120,
		"the largest shard should be no more than its byte target plus one largest arc")
}

func TestPlanArcShards_AllArcCountsFitFiveShards(t *testing.T) {
	for arcCount := 0; arcCount <= 4; arcCount++ {
		weights := make([]int, arcCount)
		for i := range weights {
			weights[i] = 40 + i*17
		}
		shards, err := memory.PlanArcShards(weights, 5)
		require.NoError(t, err, "arc count %d", arcCount)
		require.Len(t, shards, 5)

		seen := make([]int, 0, arcCount)
		for _, shard := range shards {
			seen = append(seen, shard...)
		}
		assert.Equal(t, makeRange(arcCount), seen, "arc count %d", arcCount)
	}
}

func makeRange(n int) []int {
	result := make([]int, n)
	for i := range result {
		result[i] = i
	}
	return result
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

func TestRecite_AllDeliversWholeBodyWithoutFallbackOrSections(t *testing.T) {
	db := buildTestDB(t)
	got, err := memory.Recite(db, memory.ReciteConfig{Type: "concept", Sections: []string{"all"}})
	require.NoError(t, err)
	require.NotEmpty(t, got.Items)
	item := got.Items[0]
	note, err := db.QueryFullNote(item.ID)
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(note.Body), item.Excerpt)
	assert.Empty(t, item.Sections)
	assert.False(t, item.Fallback)
}

func TestRecite_AllShardsAreDeterministicAndComplete(t *testing.T) {
	db := buildTestDB(t)
	seen := make(map[string]int)
	for shard := 1; shard <= 3; shard++ {
		cfg := memory.ReciteConfig{Type: "concept", Sections: []string{"all"}, ShardIndex: shard, ShardCount: 3}
		first, err := memory.Recite(db, cfg)
		require.NoError(t, err)
		second, err := memory.Recite(db, cfg)
		require.NoError(t, err)
		require.Equal(t, first.Items, second.Items)
		for _, item := range first.Items {
			seen[item.ID]++
		}
	}
	full, err := memory.Recite(db, memory.ReciteConfig{Type: "concept", Sections: []string{"all"}})
	require.NoError(t, err)
	assert.Len(t, seen, full.Total)
	for _, item := range full.Items {
		assert.Equal(t, 1, seen[item.ID], item.ID)
	}
}

func TestRecite_AllMaxBytesReportsOverflowIDs(t *testing.T) {
	db := buildTestDB(t)
	got, err := memory.Recite(db, memory.ReciteConfig{Type: "concept", Sections: []string{"all"}, MaxBytes: 1})
	require.NoError(t, err)
	assert.Greater(t, got.Overflow, 0)
	assert.IsIncreasing(t, got.OverflowIDs)
}

func TestRenderRecitation_SectionsAndShardHeaderStayWithinMaxBytes(t *testing.T) {
	r := &memory.ReciteResult{
		Type: "arc", Total: 1, Budget: 0, MaxBytes: 300,
		Shard: &memory.ReciteShard{Index: 2, Count: 3},
		Items: []memory.ReciteItem{{
			ID: "arc-one", Title: "One", Excerpt: "## Trigger\n\nA trigger.\n\n## Principle\n\nA rule.",
			Sections: []memory.ReciteSection{{Name: "trigger", Heading: "## Trigger", Text: "A trigger."}, {Name: "principle", Heading: "## Principle", Text: "A rule."}},
		}},
	}
	out := memory.RenderRecitation(r)
	assert.LessOrEqual(t, len(out), 300)
	assert.Contains(t, out, "arcs part 2 of 3")
	assert.Contains(t, out, "## Trigger")
	assert.Contains(t, out, "## Principle")
}

func TestRenderRecitation_EmptyShardIsAValidShortLine(t *testing.T) {
	r := &memory.ReciteResult{
		Type:  "arc",
		Total: 3,
		Shard: &memory.ReciteShard{Index: 4, Count: 5},
	}

	assert.Equal(t, "arcs part 4 of 5 — no arcs in this part\n", memory.RenderRecitation(r))
}

func TestRecite_AllArcCountsAcrossFiveShardsAreComplete(t *testing.T) {
	for arcCount := 0; arcCount <= 4; arcCount++ {
		db := emptyArcDB(t, arcCount)
		seen := make(map[string]int)
		for shard := 1; shard <= 5; shard++ {
			result, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", ShardIndex: shard, ShardCount: 5})
			require.NoError(t, err, "arc count %d shard %d", arcCount, shard)
			if arcCount < 5 && len(result.Items) == 0 {
				assert.Equal(t, "arcs part "+itoa(shard)+" of 5 — no arcs in this part\n", memory.RenderRecitation(result))
			}
			for _, item := range result.Items {
				seen[item.ID]++
			}
		}
		assert.Len(t, seen, arcCount)
		for id, count := range seen {
			assert.Equal(t, 1, count, id)
		}
	}
}

func emptyArcDB(t *testing.T, count int) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	for i := 0; i < count; i++ {
		id := "arc-" + itoa(i)
		_, err := db.Exec(`INSERT INTO notes (id, path, title, type, status, hash, mtime, body_text) VALUES (?, ?, ?, 'arc', 'active', ?, ?, ?)`, id, id+".md", id, id, i, "A body for "+id+".")
		require.NoError(t, err)
	}
	return db
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
