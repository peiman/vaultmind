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
	for _, test := range []struct {
		name    string
		weights []int
		count   int
		want    [][]int
	}{
		{name: "balanced", weights: []int{40, 80, 45, 120, 55, 60}, count: 3, want: [][]int{{0, 1}, {2, 3}, {4, 5}}},
		{name: "alternating", weights: []int{9, 1, 9, 1, 9}, count: 3, want: [][]int{{0, 1}, {2}, {3, 4}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			first, err := memory.PlanArcShards(test.weights, test.count)
			require.NoError(t, err)
			second, err := memory.PlanArcShards(test.weights, test.count)
			require.NoError(t, err)
			assert.Equal(t, first, second)
			assert.Equal(t, test.want, first)

			seen := make([]int, 0, len(test.weights))
			for _, shard := range first {
				seen = append(seen, shard...)
			}
			assert.Equal(t, makeRange(len(test.weights)), seen)
			maxShardWeight := 0
			for _, shard := range first {
				weight := 0
				for _, index := range shard {
					weight += test.weights[index]
				}
				if weight > maxShardWeight {
					maxShardWeight = weight
				}
			}
			assert.Equal(t, optimalContiguousMaxWeight(test.weights, test.count), maxShardWeight)
		})
	}
}

func optimalContiguousMaxWeight(weights []int, count int) int {
	best := int(^uint(0) >> 1)
	var visit func(start, shardsLeft, currentMax int)
	visit = func(start, shardsLeft, currentMax int) {
		if shardsLeft == 1 {
			weight := 0
			for _, value := range weights[start:] {
				weight += value
			}
			if candidate := max(currentMax, weight); candidate < best {
				best = candidate
			}
			return
		}
		weight := 0
		for cut := start; cut <= len(weights)-shardsLeft; cut++ {
			weight += weights[cut]
			visit(cut+1, shardsLeft-1, max(currentMax, weight))
		}
	}
	visit(0, count, 0)
	return best
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

func TestParseReciteConfig_FullNormalizesAndRejectsSections(t *testing.T) {
	cfg, err := memory.ParseReciteConfig("arc", 0, 120, false, "", " arc-z, arc-a, arc-z, , ", "", 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"arc-z", "arc-a"}, cfg.Full)

	_, err = memory.ParseReciteConfig("arc", 0, 120, false, "principle", "arc-a", "", 0)
	assert.EqualError(t, err, "--full cannot be combined with --sections")
}

func TestRecite_FullMixesWholeBodiesWithExcerptsAndKeepsIDOrder(t *testing.T) {
	db := emptyArcDB(t, 0)
	insertArcNote(t, db, "arc-z", "Zed", "---\nid: arc-z\ntype: arc\n---\n<!-- hidden -->\n## Trigger\n\nZ trigger.\n\n## Principle\n\nZ principle.\n\n## Source\n\nZ source.")
	insertArcNote(t, db, "arc-a", "Alpha", "---\nid: arc-a\ntype: arc\n---\n## Trigger\n\nA trigger.\n\n## Principle\n\nA principle.\n\n## Source\n\nA source.")
	insertArcNote(t, db, "arc-m", "Middle", "---\nid: arc-m\ntype: arc\n---\n## Trigger\n\nM trigger.\n\n## Principle\n\nM principle.\n\n## Source\n\nM source.")

	got, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", ExcerptTokens: 4, Full: []string{"arc-z", "arc-a", "arc-z"}})
	require.NoError(t, err)
	require.Len(t, got.Items, 3)
	assert.Equal(t, []string{"arc-a", "arc-m", "arc-z"}, []string{got.Items[0].ID, got.Items[1].ID, got.Items[2].ID})
	assert.True(t, got.Items[0].Full)
	assert.False(t, got.Items[1].Full)
	assert.True(t, got.Items[2].Full)
	assert.Contains(t, got.Items[0].Excerpt, "A source.")
	assert.Contains(t, got.Items[2].Excerpt, "Z source.")
	assert.NotContains(t, got.Items[1].Excerpt, "M source.")
	assert.NotContains(t, got.Items[1].Excerpt, "no trigger/push/principle section")
	assert.NotContains(t, got.Items[0].Excerpt, "hidden")
}

func TestRecite_FullMixedShardPlanIsDeterministicAndComplete(t *testing.T) {
	db := emptyArcDB(t, 0)
	for _, id := range []string{"arc-a", "arc-b", "arc-c", "arc-d"} {
		insertArcNote(t, db, id, id, "## Principle\n\n"+strings.Repeat("decision-"+id+" ", 12))
	}

	seen := make(map[string]int)
	for shard := 1; shard <= 2; shard++ {
		cfg := memory.ReciteConfig{Type: "arc", Full: []string{"arc-b"}, ExcerptTokens: 5, ShardIndex: shard, ShardCount: 2}
		first, err := memory.Recite(db, cfg)
		require.NoError(t, err)
		second, err := memory.Recite(db, cfg)
		require.NoError(t, err)
		assert.Equal(t, first.Items, second.Items, "mixed shard %d must be deterministic", shard)
		for _, item := range first.Items {
			seen[item.ID]++
		}
	}

	assert.Equal(t, map[string]int{"arc-a": 1, "arc-b": 1, "arc-c": 1, "arc-d": 1}, seen)
}

func TestRecite_FullUnknownIDIsRejectedAfterDraftFiltering(t *testing.T) {
	db := emptyArcDB(t, 0)
	insertArcNote(t, db, "arc-known", "Known", "Known body.")
	_, err := db.Exec(`INSERT INTO notes (id, path, title, type, status, hash, mtime, body_text) VALUES (?, ?, ?, 'arc', 'draft', ?, 0, ?)`, "arc-draft", "arc-draft.md", "Draft", "arc-draft", "Draft body.")
	require.NoError(t, err)

	_, err = memory.Recite(db, memory.ReciteConfig{Type: "arc", Full: []string{"arc-draft"}})
	assert.EqualError(t, err, "unknown arc id in --full: arc-draft")

	got, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", Full: []string{"arc-draft"}, IncludeDrafts: true})
	require.NoError(t, err)
	var draft *memory.ReciteItem
	for i := range got.Items {
		if got.Items[i].ID == "arc-draft" {
			draft = &got.Items[i]
		}
	}
	require.NotNil(t, draft)
	assert.True(t, draft.Full)

	_, err = memory.Recite(db, memory.ReciteConfig{Type: "arc", Full: []string{"arc-missing"}})
	assert.EqualError(t, err, "unknown arc id in --full: arc-missing")
}

func TestRecite_SplitShardPartsAreDeliveredOrNamedOnce(t *testing.T) {
	db := emptyArcDB(t, 0)
	body := "## Trigger\n\n" + strings.Repeat("trigger words. ", 12) +
		"\n## Push\n\n" + strings.Repeat("push words. ", 12) +
		"\n## Principle\n\n" + strings.Repeat("principle words. ", 18) + "λ rule."
	insertArcNote(t, db, "arc-split", "Split", body)

	whole, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", Sections: []string{"all"}})
	require.NoError(t, err)
	require.Len(t, whole.Items, 1)
	referenceText := make(map[int]string)
	for shard := 1; shard <= 5; shard++ {
		reference, referenceErr := memory.Recite(db, memory.ReciteConfig{
			Type: "arc", Sections: []string{"all"}, MaxBytes: 420,
			ShardIndex: shard, ShardCount: 5,
		})
		require.NoError(t, referenceErr)
		for _, item := range reference.Items {
			referenceText[item.Part] = item.Excerpt
		}
	}
	var referenceParts []string
	for part := 1; part <= 5; part++ {
		referenceParts = append(referenceParts, referenceText[part])
	}
	assert.Equal(t, whole.Items[0].Excerpt, strings.Join(referenceParts, ""), "the complete part set must preserve the whole body")

	seen := make(map[string]int)
	partText := make(map[int]string)
	parts := 0
	for shard := 1; shard <= 4; shard++ {
		got, reciteErr := memory.Recite(db, memory.ReciteConfig{
			Type: "arc", Sections: []string{"all"}, MaxBytes: 420,
			ShardIndex: shard, ShardCount: 4,
		})
		require.NoError(t, reciteErr)
		assert.LessOrEqual(t, len(memory.RenderRecitation(got)), 420, "shard %d", shard)
		for _, item := range got.Items {
			require.Equal(t, "arc-split", item.ID)
			require.Greater(t, item.Parts, 1)
			parts = item.Parts
			key := fmt.Sprintf("%s part %d of %d", item.ID, item.Part, item.Parts)
			seen[key]++
			partText[item.Part] = item.Excerpt
			assert.Equal(t, referenceText[item.Part], item.Excerpt, "part %d must not be truncated", item.Part)
		}
		for _, id := range got.OverflowIDs {
			seen[id]++
		}
	}
	require.Equal(t, 5, parts)
	for part := 1; part <= parts; part++ {
		key := fmt.Sprintf("arc-split part %d of %d", part, parts)
		assert.Equal(t, 1, seen[key], key)
	}
	assert.Len(t, seen, parts)
	var delivered, corresponding []string
	for part := 1; part <= parts; part++ {
		if text, ok := partText[part]; ok {
			delivered = append(delivered, text)
			corresponding = append(corresponding, referenceText[part])
		}
	}
	assert.Equal(t, strings.Join(corresponding, ""), strings.Join(delivered, ""))
	assert.NotEqual(t, whole.Items[0].Excerpt, strings.Join(delivered, ""), "the overflow report must account for the missing pieces")
}

func TestRecite_FullListedArcSplitsLikeAllAndHardCapsMixedShards(t *testing.T) {
	db := emptyArcDB(t, 0)
	body := "## Trigger\n\n" + strings.Repeat("trigger words. ", 8) +
		"\n## Push\n\n" + strings.Repeat("push words. ", 8) +
		"\n## Principle\n\n" + strings.Repeat("principle words. ", 8) +
		"\n## Source\n\n" + strings.Repeat("source words. ", 8)
	insertArcNote(t, db, "arc-full", "Full", body)
	insertArcNote(t, db, "arc-excerpt", "Excerpt", "## Principle\n\nA short decision rule.\n\n## Source\n\nDo not deliver this.")

	whole, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", Full: []string{"arc-full"}})
	require.NoError(t, err)
	var wholeText string
	for _, item := range whole.Items {
		if item.ID == "arc-full" {
			wholeText = item.Excerpt
			break
		}
	}
	require.NotEmpty(t, wholeText)
	var parts []memory.ReciteItem
	var overflow []string
	for shard := 1; shard <= 3; shard++ {
		got, reciteErr := memory.Recite(db, memory.ReciteConfig{
			Type: "arc", Full: []string{"arc-full"}, ExcerptTokens: 3,
			MaxBytes: 500, ShardIndex: shard, ShardCount: 3,
		})
		require.NoError(t, reciteErr)
		assert.LessOrEqual(t, len(memory.RenderRecitation(got)), 500)
		for _, item := range got.Items {
			if item.ID == "arc-full" {
				assert.True(t, item.Full)
				parts = append(parts, item)
			}
		}
		overflow = append(overflow, got.OverflowIDs...)
	}
	require.Len(t, parts, 2)
	partText := make([]string, len(parts))
	for i, part := range parts {
		partText[i] = part.Excerpt
	}
	sourceStart := strings.Index(wholeText, "## Source")
	require.NotEqual(t, -1, sourceStart)
	assert.Equal(t, wholeText[:sourceStart], strings.Join(partText, ""))
	assert.Equal(t, []string{"arc-full part 3 of 3"}, overflow)
	assert.Contains(t, wholeText, "## Source")

	emptyDB := emptyArcDB(t, 0)
	empty, err := memory.Recite(emptyDB, memory.ReciteConfig{Type: "arc", ShardIndex: 5, ShardCount: 5})
	require.NoError(t, err)
	assert.Empty(t, empty.Items)
	assert.Equal(t, "arcs part 5 of 5 — no arcs in this part\n", memory.RenderRecitation(empty))
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

func TestRecite_BudgetAppliesWithinEachShardAfterPlanning(t *testing.T) {
	db := emptyArcDB(t, 0)
	const budget = 1200
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("arc-budget-%d", i)
		body := "## Trigger\n\n" + strings.Repeat("payload-"+id+" ", 100)
		require.GreaterOrEqual(t, len(body), 1400)
		insertArcNote(t, db, id, "Budget Arc", body)
	}

	seen := make(map[string]int)
	for shard := 1; shard <= 3; shard++ {
		got, err := memory.Recite(db, memory.ReciteConfig{
			Type: "arc", Sections: []string{"all"}, Budget: budget,
			ShardIndex: shard, ShardCount: 3,
		})
		require.NoError(t, err)
		assert.Zero(t, got.Omitted, "shard %d must not inherit omissions from other shards", shard)
		assert.Zero(t, got.Overflow, "shard %d must not inherit overflow from other shards", shard)
		assert.Len(t, got.Items, 2, "shard %d should receive its planned arcs", shard)
		assert.LessOrEqual(t, got.Tokens, budget)
		for _, item := range got.Items {
			seen[item.ID]++
		}
	}

	assert.Len(t, seen, 6)
	for id, count := range seen {
		assert.Equal(t, 1, count, id)
	}

	whole, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", Sections: []string{"all"}, Budget: budget})
	require.NoError(t, err)
	assert.Greater(t, whole.Omitted, 0, "without a shard, the budget still applies to the whole list")
	assert.Equal(t, whole.Total, len(whole.Items)+whole.Omitted)
}

func TestRecite_AllMaxBytesReportsOverflowIDs(t *testing.T) {
	db := buildTestDB(t)
	got, err := memory.Recite(db, memory.ReciteConfig{Type: "concept", Sections: []string{"all"}, MaxBytes: 1})
	require.NoError(t, err)
	assert.Greater(t, got.Overflow, 0)
	assert.IsIncreasing(t, got.OverflowIDs)
}

func TestRecite_MaxBytesSplitsArcAtGreedySectionBoundaries(t *testing.T) {
	db := emptyArcDB(t, 0)
	body := "## Trigger\n\n" + strings.Repeat("trigger words. ", 8) +
		"\n## Push\n\n" + strings.Repeat("push words. ", 8) +
		"\n## Principle\n\n" + strings.Repeat("principle words. ", 8)
	insertArcNote(t, db, "arc-large", "Large", body)

	full, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", Sections: []string{"all"}})
	require.NoError(t, err)
	maxBytes := 500
	var parts []memory.ReciteItem
	for shard := 1; shard <= 2; shard++ {
		cfg := memory.ReciteConfig{
			Type: "arc", Sections: []string{"all"}, MaxBytes: maxBytes,
			ShardIndex: shard, ShardCount: 2,
		}
		got, reciteErr := memory.Recite(db, cfg)
		require.NoError(t, reciteErr)
		again, reciteErr := memory.Recite(db, cfg)
		require.NoError(t, reciteErr)
		assert.Equal(t, got.Items, again.Items, "part sharding must be deterministic")
		assert.LessOrEqual(t, len(memory.RenderRecitation(got)), maxBytes)
		parts = append(parts, got.Items...)
	}

	require.Len(t, parts, 2)
	assert.Equal(t, 1, parts[0].Part)
	assert.Equal(t, 2, parts[0].Parts)
	assert.Equal(t, 2, parts[1].Part)
	assert.Equal(t, 2, parts[1].Parts)
	assert.Contains(t, parts[0].Excerpt, "## Push")
	assert.NotContains(t, parts[0].Excerpt, "## Principle")
	assert.Contains(t, parts[1].Excerpt, "## Principle")
	assert.Equal(t, full.Items[0].Excerpt, strings.Join([]string{parts[0].Excerpt, parts[1].Excerpt}, ""))
}

func TestRecite_MaxBytesDoesNotSplitFencedHeading(t *testing.T) {
	db := emptyArcDB(t, 0)
	body := "## Trigger\n\nBefore.\n\n```markdown\n## Fake\n\nFenced text.\n```\n\nAfter.\n" +
		"## Push\n\n" + strings.Repeat("push words. ", 8) +
		"\n## Principle\n\n" + strings.Repeat("principle words. ", 8)
	insertArcNote(t, db, "arc-fenced", "Fenced", body)

	full, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", Sections: []string{"all"}})
	require.NoError(t, err)
	got, err := memory.Recite(db, memory.ReciteConfig{
		Type: "arc", Sections: []string{"all"}, MaxBytes: 500, ShardIndex: 1, ShardCount: 2,
	})
	require.NoError(t, err)
	other, err := memory.Recite(db, memory.ReciteConfig{
		Type: "arc", Sections: []string{"all"}, MaxBytes: 500, ShardIndex: 2, ShardCount: 2,
	})
	require.NoError(t, err)
	parts := append(got.Items, other.Items...)
	require.Greater(t, len(parts), 1)
	texts := make([]string, len(parts))
	for i, part := range parts {
		texts[i] = part.Excerpt
	}
	assert.Equal(t, full.Items[0].Excerpt, strings.Join(texts, ""))
	for i, part := range parts {
		if i > 0 {
			assert.False(t, strings.HasPrefix(part.Excerpt, "## Fake"))
		}
	}
}

func TestRecite_MaxBytesSplitsOversizedSectionAtParagraphsAndLines(t *testing.T) {
	db := emptyArcDB(t, 0)
	body := "## Principle\n\n" + strings.Repeat("paragraph one. ", 9) + "\n\n" +
		strings.Repeat("paragraph two. ", 9) + "\n\n" +
		"line one " + strings.Repeat("x", 70) + "\n" + "line two " + strings.Repeat("y", 70)
	insertArcNote(t, db, "arc-oversized-section", "Oversized", body)

	full, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", Sections: []string{"all"}})
	require.NoError(t, err)
	parts := make([]memory.ReciteItem, 0, 4)
	for shard := 1; shard <= 4; shard++ {
		got, reciteErr := memory.Recite(db, memory.ReciteConfig{
			Type: "arc", Sections: []string{"all"}, MaxBytes: 430,
			ShardIndex: shard, ShardCount: 4,
		})
		require.NoError(t, reciteErr)
		assert.LessOrEqual(t, len(memory.RenderRecitation(got)), 430)
		parts = append(parts, got.Items...)
	}
	require.Greater(t, len(parts), 2)
	assert.Equal(t, full.Items[0].Excerpt, strings.Join(func() []string {
		texts := make([]string, len(parts))
		for i, part := range parts {
			texts[i] = part.Excerpt
		}
		return texts
	}(), ""))
}

func TestRecite_SelectedSectionsSplitAndKeepPartMetadata(t *testing.T) {
	db := emptyArcDB(t, 0)
	body := "## Trigger\n\n" + strings.Repeat("trigger words. ", 8) +
		"\n## Push\n\n" + strings.Repeat("push words. ", 8) +
		"\n## Principle\n\n" + strings.Repeat("principle words. ", 8)
	insertArcNote(t, db, "arc-selected", "Selected", body)
	full, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", Sections: []string{"trigger", "push", "principle"}})
	require.NoError(t, err)

	parts := make([]memory.ReciteItem, 0, 2)
	for shard := 1; shard <= 2; shard++ {
		got, err := memory.Recite(db, memory.ReciteConfig{
			Type: "arc", Sections: []string{"trigger", "push", "principle"}, MaxBytes: 500,
			ShardIndex: shard, ShardCount: 2,
		})
		require.NoError(t, err)
		parts = append(parts, got.Items...)
	}
	require.Len(t, parts, 2)
	assert.Contains(t, parts[0].Excerpt, "## Trigger")
	assert.Contains(t, parts[0].Excerpt, "## Push")
	assert.NotContains(t, parts[0].Excerpt, "## Principle")
	assert.Contains(t, parts[1].Excerpt, "## Principle")
	assert.Equal(t, full.Items[0].Excerpt, parts[0].Excerpt+parts[1].Excerpt)
}

func TestRecite_MaxBytesZeroPreservesUnsplitOutput(t *testing.T) {
	db := emptyArcDB(t, 0)
	body := "## Trigger\n\nTrigger.\n\n## Principle\n\nPrinciple."
	insertArcNote(t, db, "arc-stable", "Stable", body)

	withoutCap, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", Sections: []string{"all"}})
	require.NoError(t, err)
	withZero, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", Sections: []string{"all"}, MaxBytes: 0})
	require.NoError(t, err)
	assert.Equal(t, withoutCap.Items, withZero.Items)
	assert.Equal(t, memory.RenderRecitation(withoutCap), memory.RenderRecitation(withZero))
	assert.NotContains(t, memory.RenderRecitation(withZero), "(part ")

	bounded, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", Sections: []string{"all"}, MaxBytes: 500})
	require.NoError(t, err)
	assert.Zero(t, bounded.Items[0].Part)
	assert.Zero(t, bounded.Items[0].Parts)
	assert.NotContains(t, memory.RenderRecitation(bounded), "(part ")
}

func insertArcNote(t *testing.T, db *index.DB, id, title, body string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO notes (id, path, title, type, status, hash, mtime, body_text) VALUES (?, ?, ?, 'arc', 'active', ?, 0, ?)`, id, id+".md", title, id, body)
	require.NoError(t, err)
}

func TestRenderRecitation_SectionsAndShardHeaderStayWithinMaxBytes(t *testing.T) {
	header := &memory.ReciteResult{
		Type: "arc", Total: 1, MaxBytes: 300,
		Shard: &memory.ReciteShard{Index: 2, Count: 3},
		Items: []memory.ReciteItem{{
			ID: "arc-header", Title: "Header", Excerpt: "## Trigger\n\nA trigger.\n\n## Principle\n\nA rule.",
		}},
	}
	headerText := memory.RenderRecitation(header)
	assert.LessOrEqual(t, len(headerText), 300)
	assert.Contains(t, headerText, "arcs part 2 of 3")
	assert.Contains(t, headerText, "## Trigger")
	assert.Contains(t, headerText, "## Principle")

	db := emptyArcDB(t, 0)
	insertArcNote(t, db, "arc-one", "One", "## Trigger\n\n"+strings.Repeat("A trigger. ", 30)+"\n## Principle\n\nA rule.")
	unbounded, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", Sections: []string{"trigger", "principle"}})
	require.NoError(t, err)
	maxBytes := 420
	require.Greater(t, len(memory.RenderRecitation(unbounded)), maxBytes, "fixture must require trimming")
	bounded, err := memory.Recite(db, memory.ReciteConfig{Type: "arc", Sections: []string{"trigger", "principle"}, MaxBytes: maxBytes})
	require.NoError(t, err)
	out := memory.RenderRecitation(bounded)
	assert.LessOrEqual(t, len(out), maxBytes)
	assert.Contains(t, out, "YOUR ARCS")
	require.Len(t, bounded.Items, 1)
	assert.Equal(t, "arc-one", bounded.Items[0].ID)
	assert.Equal(t, 1, bounded.Items[0].Part)
	assert.Equal(t, 3, bounded.Items[0].Parts)
	assert.Equal(t, "## Trigger\n\n"+strings.Repeat("A trigger. ", 30)[:len(bounded.Items[0].Excerpt)-len("## Trigger\n\n")], bounded.Items[0].Excerpt)
	assert.Equal(t, []string{"arc-one part 2 of 3", "arc-one part 3 of 3"}, bounded.OverflowIDs)
	assert.Contains(t, out, "arc-one part 2 of 3")
	assert.Contains(t, out, "arc-one part 3 of 3")
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
