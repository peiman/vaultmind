package parser_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/parser"
	"github.com/stretchr/testify/assert"
)

func TestStripForFTS_RemovesWikilinks(t *testing.T) {
	result := parser.StripForFTS("See [[Context Pack]] and [[Spreading Activation|spreading activation]] for details.")
	assert.Contains(t, result, "Context Pack")
	assert.Contains(t, result, "spreading activation")
	assert.NotContains(t, result, "[[")
	assert.NotContains(t, result, "]]")
}

func TestStripForFTS_RemovesEmbeds(t *testing.T) {
	result := parser.StripForFTS("Here is an embed: ![[diagram.png]] followed by more text.")
	assert.NotContains(t, result, "![[")
	assert.NotContains(t, result, "diagram.png")
	assert.Contains(t, result, "followed by more text")
}

func TestStripForFTS_RemovesMarkdownLinks(t *testing.T) {
	result := parser.StripForFTS("Read [the docs](https://obsidian.md) and [local page](concepts/act-r.md).")
	assert.Contains(t, result, "the docs")
	assert.Contains(t, result, "local page")
	assert.NotContains(t, result, "https://")
	assert.NotContains(t, result, "concepts/act-r.md")
}

func TestStripForFTS_RemovesHeadingMarkers(t *testing.T) {
	result := parser.StripForFTS("# Main Title\n\n## Section One\n\nSome text.\n\n### Subsection")
	assert.Contains(t, result, "Main Title")
	assert.Contains(t, result, "Section One")
	assert.NotContains(t, result, "#")
}

func TestStripForFTS_RemovesInlineFormatting(t *testing.T) {
	result := parser.StripForFTS("This is **bold**, _italic_, `code`, and ~~strikethrough~~.")
	assert.Contains(t, result, "bold")
	assert.Contains(t, result, "italic")
	assert.Contains(t, result, "code")
	assert.Contains(t, result, "strikethrough")
	assert.NotContains(t, result, "**")
	assert.NotContains(t, result, "_italic_")
	assert.NotContains(t, result, "`code`")
	assert.NotContains(t, result, "~~")
}

func TestStripForFTS_RemovesFencedCodeBlocks(t *testing.T) {
	result := parser.StripForFTS("Before code.\n\n```go\nfunc main() { fmt.Println(\"hello\") }\n```\n\nAfter code.")
	assert.Contains(t, result, "Before code")
	assert.Contains(t, result, "After code")
	assert.NotContains(t, result, "func main")
}

func TestStripForFTS_RemovesBlockquotes(t *testing.T) {
	result := parser.StripForFTS("Normal text.\n\n> This is a blockquote.\n> It continues.\n\nBack to normal.")
	assert.Contains(t, result, "This is a blockquote")
	assert.Contains(t, result, "Back to normal")
	assert.NotContains(t, result, "> This")
}

func TestStripForFTS_RemovesHTMLTags(t *testing.T) {
	result := parser.StripForFTS("Text with <em>emphasis</em> and <br/> breaks.")
	assert.Contains(t, result, "emphasis")
	assert.NotContains(t, result, "<em>")
}

func TestStripForFTS_RemovesHTMLComments(t *testing.T) {
	body := "Before <!-- vaultmind:accepted proposal-quartz --> after.\n<!-- keep > this whole comment -->\nDone."
	result := parser.StripForFTS(body)
	assert.Contains(t, result, "Before")
	assert.Contains(t, result, "after.")
	assert.Contains(t, result, "Done.")
	assert.NotContains(t, result, "vaultmind:accepted")
	assert.NotContains(t, result, "proposal-quartz")
	assert.NotContains(t, result, "this whole comment")
}

func TestStripVaultMindMarkers(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain text", in: "Quartz keeps one timeline.", want: "Quartz keeps one timeline."},
		{name: "marker", in: "<!-- vaultmind:accepted proposal-1 -->\nQuartz keeps one timeline.", want: "\nQuartz keeps one timeline."},
		{name: "author comment", in: "<!-- todo -->\nQuartz", want: "<!-- todo -->\nQuartz"},
		{name: "marker and author comment", in: "<!-- todo -->\n<!-- vaultmind:accepted proposal-1 -->\nQuartz", want: "<!-- todo -->\n\nQuartz"},
		{name: "vaultmind word inside an author comment", in: "<!-- note vaultmind:accepted -->\nQuartz", want: "<!-- note vaultmind:accepted -->\nQuartz"},
		{name: "two markers", in: "<!-- vaultmind:accepted a -->\n<!-- vaultmind:accepted b -->\nDone", want: "\n\nDone"},
		{name: "no space after the comment open", in: "<!--vaultmind:accepted a-->\nDone", want: "\nDone"},
		{name: "unclosed comment stays", in: "<!-- vaultmind:accepted proposal-1\nDone", want: "<!-- vaultmind:accepted proposal-1\nDone"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parser.StripVaultMindMarkers(tc.in))
		})
	}
}

func TestStripHTMLComments_LeavesTextWithoutAComment(t *testing.T) {
	plain := "Quartz keeps one timeline."
	assert.Equal(t, plain, parser.StripHTMLComments(plain))

	marked := "<!-- vaultmind:accepted proposal-1 -->\nQuartz keeps one timeline."
	got := parser.StripHTMLComments(marked)
	assert.NotContains(t, got, "vaultmind:accepted")
	assert.NotContains(t, got, "proposal-1")
	assert.Contains(t, got, "Quartz keeps one timeline.")
}

func TestStripForFTS_RemovesBlockIDs(t *testing.T) {
	result := parser.StripForFTS("An important fact. ^key-fact\n\nAnother paragraph.")
	assert.NotContains(t, result, "^key-fact")
	assert.Contains(t, result, "An important fact")
}

func TestStripForFTS_RemovesHorizontalRules(t *testing.T) {
	result := parser.StripForFTS("Section one.\n\n---\n\nSection two.\n\n***\n\nSection three.")
	assert.Contains(t, result, "Section one")
	assert.Contains(t, result, "Section two")
	assert.NotContains(t, result, "---")
}

func TestStripForFTS_PreservesPlainText(t *testing.T) {
	result := parser.StripForFTS("This is ordinary plain text with no special markdown.")
	assert.Contains(t, result, "This is ordinary plain text with no special markdown")
}

func TestStripForFTS_EmptyBody(t *testing.T) {
	assert.Empty(t, parser.StripForFTS(""))
}

func TestStripForFTS_CollapseWhitespace(t *testing.T) {
	result := parser.StripForFTS("Word1   word2\t\tword3\n\n\n\nword4")
	assert.NotContains(t, result, "   ")
	assert.NotContains(t, result, "\t\t")
}

func TestStripForFTS_RealVaultNoteBody(t *testing.T) {
	body := "# Anderson — The Architecture of Cognition (1983)\n\nAnderson introduced ACT* (predecessor to [[ACT-R]]), a unified theory.\n\nThe book established that memory is a **dynamic system** shaped by use.\n\nVaultMind's model is directly inspired by ACT-R activation equations."
	result := parser.StripForFTS(body)
	assert.Contains(t, result, "Anderson")
	assert.Contains(t, result, "ACT-R")
	assert.Contains(t, result, "dynamic system")
	assert.NotContains(t, result, "[[")
	assert.NotContains(t, result, "**")
	assert.NotContains(t, result, "#")
}

func TestStripForFTSOffsets(t *testing.T) {
	for _, body := range []string{"", "ordinary", " \n\t", "Before\n```go\nignored\n```\nAfter", "~~~\nignored\n~~~", "# Heading\n\n![[embed]] [[destination|alias]] [[page]] [label](url) <em>text</em> `code` **bold** _italic_ *star* ~~strike~~\n> quote ^block\n---\n\n\n tail\t space"} {
		text, offsets := parser.StripForFTSOffsets(body)
		assert.Equal(t, parser.StripForFTS(body), text)
		assert.Len(t, offsets, len(text))
		for i, at := range offsets {
			if text[i] != ' ' {
				assert.Equal(t, body[at], text[i], "source mapping at %d", i)
			}
		}
	}
}
