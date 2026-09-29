package importdocs

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
