package importdocs

import (
	"context"
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
