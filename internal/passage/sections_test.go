package passage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A section runs from its heading to the next heading at its level or above,
// so it holds its subsections and its code. A heading inside a fence is code,
// and an H1 that repeats the note title is the note, not a section.
func TestSections(t *testing.T) {
	body := "# Title\n\nIntro.\n\n## A\n\nText.\n\n```sh\n# not a heading\n```\n\n### A.1\n\nMore.\n\nB\n-\n\nLast."
	got := Sections(body, "Title")
	require.Len(t, got, 3)

	text := func(s Section) string { return body[s.Start:s.End] }
	assert.Equal(t, "## A\n\nText.\n\n```sh\n# not a heading\n```\n\n### A.1\n\nMore.\n\n", text(got[0]))
	assert.Equal(t, 2, got[0].Level)
	assert.Equal(t, "### A.1\n\nMore.\n\n", text(got[1]))
	assert.Equal(t, 3, got[1].Level)
	assert.Equal(t, "B\n-\n\nLast.", text(got[2]))
	assert.Equal(t, "B\n-", got[2].Heading)
	assert.Equal(t, "## A", got[0].Heading)
}

// A different H1 is a section like any other.
func TestSections_OtherH1(t *testing.T) {
	got := Sections("# Other\n\nText.", "Title")
	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].Level)
}
