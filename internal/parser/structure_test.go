package parser_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractHeadings_AllLevels(t *testing.T) {
	body := "# Level 1\n\nSome text.\n\n## Level 2\n\n### Level 3\n\n#### Level 4\n\n##### Level 5\n\n###### Level 6"
	headings := parser.ExtractHeadings(body)
	require.Len(t, headings, 6)
	assert.Equal(t, 1, headings[0].Level)
	assert.Equal(t, "Level 1", headings[0].Title)
	assert.Equal(t, "level-1", headings[0].Slug)
	assert.Equal(t, 6, headings[5].Level)
}

func TestExtractHeadings_SlugNormalization(t *testing.T) {
	body := "# Hello World\n\n## ACT-R Architecture\n\n### Some Title With Spaces And Caps"
	headings := parser.ExtractHeadings(body)
	require.Len(t, headings, 3)
	assert.Equal(t, "hello-world", headings[0].Slug)
	assert.Equal(t, "act-r-architecture", headings[1].Slug)
	assert.Equal(t, "some-title-with-spaces-and-caps", headings[2].Slug)
}

func TestExtractHeadings_SlugWithSpecialChars(t *testing.T) {
	body := "## Decision: Use BFS (with visited set)\n\n### Phase 1 — Parser & Link Extractor"
	headings := parser.ExtractHeadings(body)
	require.Len(t, headings, 2)
	assert.Equal(t, "decision-use-bfs-with-visited-set", headings[0].Slug)
	assert.Equal(t, "phase-1--parser--link-extractor", headings[1].Slug)
}

func TestExtractHeadings_HeadingNotInCodeBlock(t *testing.T) {
	body := "# Real Heading\n\n```\n# Not a heading\n```\n\n## Another Real Heading"
	headings := parser.ExtractHeadings(body)
	titles := make([]string, len(headings))
	for i, h := range headings {
		titles[i] = h.Title
	}
	assert.Contains(t, titles, "Real Heading")
	assert.Contains(t, titles, "Another Real Heading")
	assert.NotContains(t, titles, "Not a heading")
}

func TestExtractHeadings_LineNumbers(t *testing.T) {
	body := "# First\n\nSome text.\n\n## Second\n\n### Third"
	headings := parser.ExtractHeadings(body)
	require.Len(t, headings, 3)
	assert.Equal(t, 1, headings[0].Line)
	assert.Equal(t, 5, headings[1].Line)
	assert.Equal(t, 7, headings[2].Line)
}

func TestExtractHeadings_NoHeadings(t *testing.T) {
	assert.Empty(t, parser.ExtractHeadings("This is plain text.\n\nNo headings here."))
}

func TestExtractHeadings_EmptyBody(t *testing.T) {
	assert.Empty(t, parser.ExtractHeadings(""))
}

func TestExtractHeadings_HeadingMustHaveSpace(t *testing.T) {
	body := "#NoSpace\n\n# With Space"
	headings := parser.ExtractHeadings(body)
	require.Len(t, headings, 1)
	assert.Equal(t, "With Space", headings[0].Title)
}

func TestExtractHeadings_RealVaultNote(t *testing.T) {
	body := "# Mandate BFS with Visited Set\n\n## Decision\n\nAll graph traversal must use BFS.\n\n## Rationale\n\nSeveral strong reasons.\n\n## Trade-offs Accepted\n\nBFS has higher peak memory."
	headings := parser.ExtractHeadings(body)
	require.Len(t, headings, 4)
	assert.Equal(t, "Mandate BFS with Visited Set", headings[0].Title)
	assert.Equal(t, 1, headings[0].Level)
	assert.Equal(t, "Decision", headings[1].Title)
}

func TestExtractBlocks_SimpleBlockID(t *testing.T) {
	blocks := parser.ExtractBlocks("This is a paragraph. ^block-abc123")
	require.Len(t, blocks, 1)
	assert.Equal(t, "block-abc123", blocks[0].BlockID)
}

func TestExtractBlocks_MultipleBlocks(t *testing.T) {
	body := "First paragraph. ^first-block\n\nSome more text.\n\nSecond paragraph with ID. ^second-block\n\nThird paragraph — no ID."
	blocks := parser.ExtractBlocks(body)
	require.Len(t, blocks, 2)
	assert.Equal(t, "first-block", blocks[0].BlockID)
	assert.Equal(t, "second-block", blocks[1].BlockID)
}

func TestExtractBlocks_LineNumbers(t *testing.T) {
	body := "Line 1\nLine 2 ^block-one\nLine 3\nLine 4 ^block-two"
	blocks := parser.ExtractBlocks(body)
	require.Len(t, blocks, 2)
	assert.Equal(t, 2, blocks[0].Line)
	assert.Equal(t, 4, blocks[1].Line)
}

func TestExtractBlocks_BlockUnderHeading(t *testing.T) {
	body := "## My Section\n\nFirst para.\n\nImportant fact. ^key-fact"
	blocks := parser.ExtractBlocks(body)
	require.Len(t, blocks, 1)
	assert.Equal(t, "key-fact", blocks[0].BlockID)
	assert.Equal(t, "My Section", blocks[0].Heading)
}

func TestExtractBlocks_BlockNotInCodeBlock(t *testing.T) {
	body := "Real block here. ^real-block\n```\nCode ^not-a-block\n```"
	blocks := parser.ExtractBlocks(body)
	ids := make([]string, len(blocks))
	for i, b := range blocks {
		ids[i] = b.BlockID
	}
	assert.Contains(t, ids, "real-block")
	assert.NotContains(t, ids, "not-a-block")
}

func TestExtractBlocks_EmptyBody(t *testing.T) {
	assert.Empty(t, parser.ExtractBlocks(""))
}

func TestExtractBlocks_NoBlocks(t *testing.T) {
	assert.Empty(t, parser.ExtractBlocks("Just text, no block IDs."))
}
