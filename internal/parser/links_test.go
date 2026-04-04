package parser_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractLinks_SimpleWikilink(t *testing.T) {
	body := "See [[Context Pack]] for details."
	links := parser.ExtractLinks(body)

	require.Len(t, links, 1)
	assert.Equal(t, "Context Pack", links[0].Target)
	assert.Equal(t, "Context Pack", links[0].Display)
	assert.Equal(t, parser.LinkTypeWikilink, links[0].LinkType)
	assert.Equal(t, parser.TargetKindNote, links[0].TargetKind)
	assert.Empty(t, links[0].Heading)
	assert.Empty(t, links[0].BlockID)
}

func TestExtractLinks_AliasedWikilink(t *testing.T) {
	body := "Read [[Memory Model|the memory model]] for background."
	links := parser.ExtractLinks(body)

	require.Len(t, links, 1)
	assert.Equal(t, "Memory Model", links[0].Target)
	assert.Equal(t, "the memory model", links[0].Display)
	assert.Equal(t, parser.LinkTypeWikilink, links[0].LinkType)
}

func TestExtractLinks_HeadingLink(t *testing.T) {
	body := "See [[Memory Model#Recall Algorithm]] for traversal."
	links := parser.ExtractLinks(body)

	require.Len(t, links, 1)
	assert.Equal(t, "Memory Model", links[0].Target)
	assert.Equal(t, "Recall Algorithm", links[0].Heading)
	assert.Equal(t, parser.TargetKindHeading, links[0].TargetKind)
}

func TestExtractLinks_BlockLink(t *testing.T) {
	body := "Referenced at [[Memory Model#^block-abc123]]."
	links := parser.ExtractLinks(body)

	require.Len(t, links, 1)
	assert.Equal(t, "Memory Model", links[0].Target)
	assert.Equal(t, "block-abc123", links[0].BlockID)
	assert.Equal(t, parser.TargetKindBlock, links[0].TargetKind)
}

func TestExtractLinks_AliasedHeadingLink(t *testing.T) {
	body := "See [[Memory Model#Recall Algorithm|Recall Algorithm]] for details."
	links := parser.ExtractLinks(body)

	require.Len(t, links, 1)
	assert.Equal(t, "Memory Model", links[0].Target)
	assert.Equal(t, "Recall Algorithm", links[0].Heading)
	assert.Equal(t, "Recall Algorithm", links[0].Display)
}

func TestExtractLinks_EmbedLink(t *testing.T) {
	body := "![[diagram.png]] appears here.\n![[Architecture Overview#Key Points]] is embedded."
	links := parser.ExtractLinks(body)

	require.Len(t, links, 2)
	assert.Equal(t, "diagram.png", links[0].Target)
	assert.Equal(t, parser.LinkTypeEmbed, links[0].LinkType)

	assert.Equal(t, "Architecture Overview", links[1].Target)
	assert.Equal(t, "Key Points", links[1].Heading)
	assert.Equal(t, parser.LinkTypeEmbed, links[1].LinkType)
}

func TestExtractLinks_MarkdownLink(t *testing.T) {
	body := "See [the Obsidian docs](https://obsidian.md) and [local note](concepts/act-r.md)."
	links := parser.ExtractLinks(body)

	require.Len(t, links, 2)
	assert.Equal(t, "https://obsidian.md", links[0].Target)
	assert.Equal(t, "the Obsidian docs", links[0].Display)
	assert.Equal(t, parser.LinkTypeMarkdown, links[0].LinkType)
	assert.Equal(t, parser.TargetKindExternal, links[0].TargetKind)

	assert.Equal(t, "concepts/act-r.md", links[1].Target)
	assert.Equal(t, "local note", links[1].Display)
	assert.Equal(t, parser.TargetKindPath, links[1].TargetKind)
}

func TestExtractLinks_MultipleLinksOnOneLine(t *testing.T) {
	body := "Both [[ACT-R]] and [[Spreading Activation]] are related."
	links := parser.ExtractLinks(body)

	require.Len(t, links, 2)
	assert.Equal(t, "ACT-R", links[0].Target)
	assert.Equal(t, "Spreading Activation", links[1].Target)
}

func TestExtractLinks_ConsecutiveWikilinks(t *testing.T) {
	// B2 regression: [[A]][[B]] must extract both links
	body := "[[A]][[B]][[C]]"
	links := parser.ExtractLinks(body)

	require.Len(t, links, 3)
	assert.Equal(t, "A", links[0].Target)
	assert.Equal(t, "B", links[1].Target)
	assert.Equal(t, "C", links[2].Target)
}

func TestExtractLinks_LinksInCodeBlocksSkipped(t *testing.T) {
	body := "Normal [[Linked Note]] is extracted.\n" +
		"```\n[[Code Block Link]] is NOT extracted.\n```\n" +
		"Another [[After Code]] is extracted."
	links := parser.ExtractLinks(body)

	targets := make([]string, len(links))
	for i, l := range links {
		targets[i] = l.Target
	}
	assert.Contains(t, targets, "Linked Note")
	assert.Contains(t, targets, "After Code")
	assert.NotContains(t, targets, "Code Block Link")
}

func TestExtractLinks_LinksInInlineCodeSkipped(t *testing.T) {
	body := "Use `[[inline code link]]` as an example but [[Real Link]] is a link."
	links := parser.ExtractLinks(body)

	targets := make([]string, len(links))
	for i, l := range links {
		targets[i] = l.Target
	}
	assert.Contains(t, targets, "Real Link")
	assert.NotContains(t, targets, "inline code link")
}

func TestExtractLinks_LineNumbers(t *testing.T) {
	body := "Line 1\nLine 2 has [[Note A]]\nLine 3\nLine 4 has [[Note B]]"
	links := parser.ExtractLinks(body)

	require.Len(t, links, 2)
	assert.Equal(t, 2, links[0].Line)
	assert.Equal(t, 4, links[1].Line)
}

func TestExtractLinks_EmptyBody(t *testing.T) {
	links := parser.ExtractLinks("")
	assert.Empty(t, links)
}

func TestExtractLinks_NoLinks(t *testing.T) {
	links := parser.ExtractLinks("This is plain text with no links at all.")
	assert.Empty(t, links)
}

func TestExtractLinks_RealVaultNoteBody(t *testing.T) {
	body := "# Memory Research Knowledge Base\n\n" +
		"- **Cognitive science** — [[Forgetting Curve]], [[Spacing Effect]], [[ACT-R]].\n" +
		"- **AI memory** — [[MemGPT]] and [[Generative Agents]].\n" +
		"- **Retrieval** — [[RAG]] and [[Embedding-Based Retrieval]].\n\n" +
		"See [[VaultMind]] for how the research is applied."
	links := parser.ExtractLinks(body)

	targets := make([]string, len(links))
	for i, l := range links {
		targets[i] = l.Target
	}
	assert.Contains(t, targets, "Forgetting Curve")
	assert.Contains(t, targets, "ACT-R")
	assert.Contains(t, targets, "MemGPT")
	assert.Contains(t, targets, "VaultMind")
	assert.Len(t, links, 8)
}
