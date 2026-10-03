package importdocs

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// para is n characters of prose, so section sizes are set by the test.
func prose(word string, n int) string {
	return strings.TrimSpace(strings.Repeat(word+" ", n/(len(word)+1)+1))[:n]
}

// Sections break at headings, but not at a "#" line inside a code fence; a
// small section joins the next; each keeps its heading path.
func TestSplitSections_AtHeadingsOutsideCodeFences(t *testing.T) {
	body := "Intro text " + prose("intro", 2000) + "\n\n" +
		"## Alpha\n\n" + prose("alpha", 2500) + "\n\n```sh\n# not a heading\n```\n\n" +
		"### Tiny\n\nsmall bit\n\n" +
		"## Beta\n\n" + prose("beta", 2500) + "\n"

	secs := splitSections(body)
	require.Len(t, secs, 3, "preamble, Alpha (with Tiny merged in), Beta")
	assert.Empty(t, secs[0].path, "text before the first heading has no heading")
	assert.Equal(t, []string{"Alpha"}, secs[1].path)
	assert.Contains(t, secs[1].text, "# not a heading", "a fenced line is text, not a break")
	assert.Equal(t, []string{"Beta"}, secs[2].path)
	assert.NotContains(t, secs[1].text, "## Alpha", "a section's own heading is in its path, not its text")
}

// A section over the maximum splits at paragraphs; a doc without headings
// (a PDF's text) becomes parts of at most that size.
func TestSplitSections_SplitsLargeSectionsAndHeadinglessText(t *testing.T) {
	var paras []string
	for i := 0; i < 12; i++ {
		paras = append(paras, prose("word", 1000))
	}
	secs := splitSections(strings.Join(paras, "\n\n"))
	require.Greater(t, len(secs), 1)
	for _, s := range secs {
		assert.LessOrEqual(t, len(s.text), maxSectionChars)
	}
}

// A heading whose text is split into parts names each part, so no two
// section notes share a title.
func TestExpandLong_PartsOfOneHeadingAreNumbered(t *testing.T) {
	var big []string
	for i := 0; i < 16; i++ { // 16 × 900 chars: three parts of at most 6,000
		big = append(big, prose("big", 900))
	}
	body := "## Big\n\n" + strings.Join(big, "\n\n") + "\n\n## Next\n\n" + prose("next", 5000) +
		"\n\n## Last\n\n" + prose("last", 5000) + "\n\n## More\n\n" + prose("more", 5000) +
		"\n\n## End\n\n" + prose("end", 5000)
	d := doc{Rel: "m.md", Path: "m.md", Repo: "r", Source: "r:m.md", Title: "M", Body: body}
	var titles []string
	for _, sd := range expandLong(d)[1:] {
		titles = append(titles, sd.Title)
	}
	assert.Contains(t, titles, "M › Big (1/3)")
	assert.Contains(t, titles, "M › Big (3/3)")
	assert.Contains(t, titles, "M › Next", "a heading in one piece keeps its plain name")
}

// Only a doc past the long threshold that yields at least two sections is
// expanded; the index keeps the doc's path, id and source, and lists its
// sections by id.
func TestExpandLong_IndexAndSections(t *testing.T) {
	short := doc{Rel: "guide.md", Path: "docs/guide.md", Repo: "demo", Source: "demo:docs/guide.md", Title: "Guide", Body: "short"}
	assert.Equal(t, []doc{short}, expandLong(short))

	var b strings.Builder
	for _, h := range []string{"Install", "Configure", "Run", "Debug", "Extend", "Release", "Upgrade"} {
		b.WriteString("## " + h + "\n\n" + prose(strings.ToLower(h), 5000) + "\n\n")
	}
	long := doc{Rel: "guide.md", Path: "docs/guide.md", Repo: "demo", Source: "demo:docs/guide.md", Title: "Guide", Body: b.String()}
	long.Hash = hashOf(long.Body)

	docs := expandLong(long)
	require.Len(t, docs, 8)
	idx, first := docs[0], docs[1]
	assert.Equal(t, "guide.md", idx.Rel)
	assert.Equal(t, "demo:docs/guide.md", idx.Source)
	assert.Contains(t, idx.Body, "Guide: 7 sections.")
	assert.Contains(t, idx.Body, "[[imported-demo-docs-guide-01-install|Install]]")
	assert.Equal(t, hashOf(idx.Body), idx.Hash)

	assert.Equal(t, "guide/01-install.md", first.Rel)
	assert.Equal(t, "demo:docs/guide.md#01-install", first.Source)
	assert.Equal(t, "Guide › Install", first.Title)
	assert.Equal(t, "imported-demo-docs-guide", first.PartOf)
	assert.Equal(t, "imported-demo-docs-guide-01-install", noteID(first.Repo, first.Path))
	assert.Contains(t, first.Body, "Part of [[imported-demo-docs-guide]].")
	assert.Equal(t, hashOf(first.Body), first.Hash)
}
