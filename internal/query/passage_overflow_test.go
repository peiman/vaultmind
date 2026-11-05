package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Navigation is a block made only of links to headings in the same note.
// One line of prose, or a link that leaves the note, makes it content.
func TestIsNavigation(t *testing.T) {
	for text, want := range map[string]bool{
		"- [Setup](#setup)\n  - [Install](#install)\n1. [Use](#use)": true,
		"[A](#a) | [B](#b)":                     true,
		"- [Setup](#setup)\nRun the installer.": false,
		"- [Docs](https://example.com/docs)":    false,
		"See [Setup](#setup) first.":            false,
		"   \n":                                 false,
	} {
		assert.Equal(t, want, isNavigation(text), text)
	}
}

// A cut keeps whole lines, marks itself, and closes a fence it cut through,
// so the rest of the delivery is not read as code.
func TestCutAtLine(t *testing.T) {
	body := "## Step\n\nRun:\n\n```bash\ntask one\ntask two\n```\n\nDone."
	kept, text := cutAtLine(body, 40)
	assert.Equal(t, "## Step\n\nRun:\n\n```bash\ntask one\n```\n…", text)
	assert.Equal(t, len("## Step\n\nRun:\n\n```bash\ntask one\n"), kept)

	kept, text = cutAtLine(body, 3)
	assert.Zero(t, kept)
	assert.Empty(t, text)
}

// The keyword pick too long for any line to fit is cut at a word, as the
// packer cuts. A dense pick is not: a word or two of a far-off block under
// its headings is a fragment, not an answer.
func TestFitPick(t *testing.T) {
	long := "one long sentence that cannot fit in the room given."
	assert.Equal(t, "short.", fitPick("short.", 10, false))
	got := fitPick(long, 24, false)
	assert.NotEmpty(t, got)
	assert.LessOrEqual(t, len(got), 24+len("…"))
	assert.Empty(t, fitPick(long, 24, true))
	assert.Equal(t, "short.", fitPick("short.", 10, true))
}
