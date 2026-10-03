package importdocs

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDropLeadingLinkList_GoDevBreadcrumbLeavesTheNote(t *testing.T) {
	in := "1. [Documentation](https://go.dev/doc/)\n" +
		"2. [Effective Go](https://go.dev/doc/effective_go)\n" +
		"\n" +
		"**Note:** This document was written for Go's release in 2009.\n"
	got := dropLeadingLinkList(in)
	assert.True(t, strings.HasPrefix(got, "**Note:**"), "got %q", got)
	assert.Contains(t, got, "This document was written for Go's release in 2009.")
}

func TestDropLeadingLinkList_APageThatIsOnlyALinkListStays(t *testing.T) {
	in := "1. [Documentation](https://go.dev/doc/)\n" +
		"2. [Effective Go](https://go.dev/doc/effective_go)\n"
	assert.Equal(t, in, dropLeadingLinkList(in))
}

func TestDropLeadingLinkList_ALinkListAfterAParagraphStays(t *testing.T) {
	in := "Effective Go is a document about the language.\n" +
		"\n" +
		"1. [Documentation](https://go.dev/doc/)\n" +
		"2. [Effective Go](https://go.dev/doc/effective_go)\n" +
		"\n" +
		"The list is part of the page.\n"
	assert.Equal(t, in, dropLeadingLinkList(in))
}

func TestDropLeadingLinkList_AnItemWithTextBesidesTheLinkStays(t *testing.T) {
	in := "1. See [x](y) for more\n" +
		"\n" +
		"**Note:** This document was written for Go's release in 2009.\n"
	assert.Equal(t, in, dropLeadingLinkList(in))
}

func TestDropLeadingLinkList_UnorderedBreadcrumbWithSpaceLeavesTheNote(t *testing.T) {
	in := "\n" +
		"  - [Documentation](https://go.dev/doc/)  \n" +
		"\n" +
		"* [Effective Go](https://go.dev/doc/effective_go)\n" +
		"+ [Tour](https://go.dev/tour/)\n" +
		"\n" +
		"**Note:** This document was written for Go's release in 2009.\n"
	got := dropLeadingLinkList(in)
	assert.True(t, strings.HasPrefix(got, "**Note:**"), "got %q", got)
}

func TestImportURL_APanicWhileConvertingThePageIsAnError(t *testing.T) {
	prev := htmlConverter
	htmlConverter = func(string, string) (string, string, error) {
		panic("boom")
	}
	t.Cleanup(func() { htmlConverter = prev })

	vault := t.TempDir()
	_, err := ImportURL(t.Context(), "https://example.com/panic", vault, Options{}, func(context.Context, string) (Page, error) {
		return Page{
			FinalURL:    "https://example.com/panic",
			ContentType: "text/html",
			Body:        []byte("<p>Hi</p>"),
		}, nil
	})
	require.Error(t, err)
	assert.EqualError(t, err, "converting the page: boom")
	assert.NoDirExists(t, filepath.Join(vault, "imported"))
}

// docPage is a doc-site page: a navbar dropdown, the content in <main>, and
// a footer. mainText is what <main> holds.
func docPage(mainText string) string {
	return `<html><head><title>Sidebar</title></head><body>
<nav><a href="/docs/sidebar">3.10.2</a><ul><li><a href="/docs/next/sidebar">Canary</a></li><li><a href="/docs/3.9.2/sidebar">3.9.2</a></li></ul></nav>
<main><h1>Sidebar</h1>` + mainText + `</main>
<footer>Copyright Example Platforms. Built with a generator.</footer></body></html>`
}

func TestChooseFragment_MainWinsWhenReadabilityKeptTooLittle(t *testing.T) {
	long := strings.Repeat("<p>A sidebar groups the docs into categories and orders them for readers.</p>", 12)
	page := docPage(long)
	// A category page: readability takes the footer, plain text and short.
	picked := `<div><p>Copyright Example Platforms. Built with a generator.</p></div>`

	got := chooseFragment(picked, page)
	assert.Contains(t, got, "A sidebar groups the docs")
	assert.NotContains(t, got, "Copyright")
}

func TestChooseFragment_ReadabilityStaysWhenItKeptTheArticle(t *testing.T) {
	long := strings.Repeat("<p>A sidebar groups the docs into categories and orders them for readers.</p>", 12)
	picked := "<div><h1>Sidebar</h1>" + long + "</div>"
	assert.Equal(t, picked, chooseFragment(picked, docPage(long+"<p>Edit this page</p>")))
	assert.Equal(t, picked, chooseFragment(picked, "<html><body><div>"+long+"</div></body></html>"), "no <main>: readability's pick stands")
}

// Docusaurus keeps its version dropdown at the top of the article: the
// current version as a bare link, then a list of versions with separators
// and a bold label, then html-to-markdown's list separator.
func TestDropLeadingLinkList_AVersionDropdownLeavesTheNote(t *testing.T) {
	in := "[3.10.2](https://docusaurus.io/docs/sidebar)\n\n" +
		"- [Canary 🚧](https://docusaurus.io/docs/next/sidebar)\n" +
		"- [3.10.2](https://docusaurus.io/docs/sidebar)\n" +
		"- * * *\n" +
		"- **Archived versions**\n" +
		"- [1.x.x](https://v1.docusaurus.io)\n\n" +
		"<!--THE END-->\n\n" +
		"- Guides\n\n" +
		"# Sidebar\n\nA sidebar groups the docs.\n"
	got := dropLeadingLinkList(in)
	assert.True(t, strings.HasPrefix(got, "- Guides"), "got %q", got)
}

func TestDropLeadingLinkList_ABareLinkBeforeTextStaysWithoutAList(t *testing.T) {
	in := "**Archived versions**\n\nThe archive holds old releases.\n"
	assert.Equal(t, in, dropLeadingLinkList(in), "a bold line alone is not navigation")
}

// A short Sphinx page: the sidebar has more text than the page, so
// readability takes it. It is nearly all links, and <main> is not.
func TestChooseFragment_MainWinsOverALinkListReadabilityPicked(t *testing.T) {
	var side strings.Builder
	side.WriteString("<div><p>The Basics</p><ul>")
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&side, `<li><a href="/x%d.html">sphinx.ext.module%d – Publish HTML docs somewhere</a></li>`, i, i)
	}
	side.WriteString("</ul></div>")
	page := `<html><body>` + side.String() + `<div role="main"><h1>githubpages</h1><p>This extension creates a .nojekyll file in the HTML output so GitHub Pages publishes it.</p></div></body></html>`

	got := chooseFragment(side.String(), page)
	assert.Contains(t, got, "creates a .nojekyll file")
	assert.NotContains(t, got, "module7")
}

// MkDocs Material opens the article with its icon links, two on one line.
func TestDropLeadingLinkList_IconLinksOnOneLineLeaveTheNote(t *testing.T) {
	in := `[](https://github.com/x/edit/master/docs/a.md "Edit this page")[](https://github.com/x/raw/master/docs/a.md "View source of this page")` + "\n\n" +
		"After you've installed it, you can bootstrap your docs.\n"
	got := dropLeadingLinkList(in)
	assert.True(t, strings.HasPrefix(got, "After you've installed it"), "got %q", got)
}
