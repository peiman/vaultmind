package section

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// filler returns roughly n tokens of prose (4 chars per token) ending in a
// blank line, so a section built from it is a whole paragraph.
func filler(n int) string {
	word := "lorem "
	var b strings.Builder
	for b.Len() < n*4 {
		b.WriteString(word)
	}
	return strings.TrimSpace(b.String()) + "\n\n"
}

// long pads a body past the split threshold with a final section.
func long(body string) string {
	return body + "## Padding\n\n" + filler(splitThreshold)
}

func joined(ss []Section) string {
	var b strings.Builder
	for _, s := range ss {
		b.WriteString(s.Body)
	}
	return b.String()
}

func paths(ss []Section) [][]string {
	out := make([][]string, len(ss))
	for i, s := range ss {
		out[i] = s.HeadingPath
	}
	return out
}

// A note that fits the model's window stays one unit: no sections at all.
func TestSplit_ShortNoteIsNotSplit(t *testing.T) {
	assert.Nil(t, Split("# Title\n\nA short note.\n"))
	atLimit := strings.Repeat("a", splitThreshold*4)
	assert.Nil(t, Split(atLimit), "exactly at the threshold stays whole")
	require.NotNil(t, Split(atLimit+"a"), "one rune over is split")
}

func TestSplit_HeadingLevelsOneToThree(t *testing.T) {
	body := long("# A\n\n" + filler(300) + "## B\n\n" + filler(300) + "### C\n\n" + filler(300) +
		"#### D is not a boundary\n\n" + filler(300) + "#tag is not a heading\n\n" + filler(300))
	ss := Split(body)
	require.NotEmpty(t, ss)
	assert.Equal(t, [][]string{{"A"}, {"A", "B"}, {"A", "B", "C"}, {"A", "Padding"}}, paths(ss))
	assert.Equal(t, body, joined(ss))
}

// A `# comment` inside a code fence is code, not a heading — real imported
// pages (Python's sqlite3 docs) are full of them.
func TestSplit_HashLinesInsideFencesDoNotSplit(t *testing.T) {
	body := long("## Loading\n\n" + filler(300) +
		"```python\n# Load the fulltext search extension\ncon.enable_load_extension(True)\n```\n\n" +
		"~~~\n## not a heading either\n~~~\n\n" + filler(300))
	ss := Split(body)
	require.Len(t, ss, 2)
	assert.Equal(t, []string{"Loading"}, ss[0].HeadingPath)
	assert.Contains(t, ss[0].Body, "# Load the fulltext search extension")
	assert.Equal(t, body, joined(ss))
}

func TestSplit_BreadcrumbDropsDeeperHeadings(t *testing.T) {
	body := long("# Top\n\n" + filler(300) + "## One\n\n" + filler(300) + "### Deep\n\n" + filler(300) +
		"## Two\n\n" + filler(300))
	assert.Equal(t, [][]string{{"Top"}, {"Top", "One"}, {"Top", "One", "Deep"}, {"Top", "Two"}, {"Top", "Padding"}},
		paths(Split(body)))
}

func TestSplit_HeadingNoiseIsStripped(t *testing.T) {
	body := long("### Blob objects[¶](#blob-objects \"Link to this heading\")\n\n" + filler(300) +
		"## 5.1.4. The fts5\\_get\\_locale() function ##\n\n" + filler(300) +
		"## See [the docs](https://example.test/x)\n\n" + filler(300))
	ss := Split(body)
	assert.Equal(t, []string{"Blob objects"}, ss[0].HeadingPath)
	assert.Equal(t, []string{"5.1.4. The fts5_get_locale() function"}, ss[1].HeadingPath)
	assert.Equal(t, []string{"See the docs"}, ss[2].HeadingPath)
	assert.Equal(t, "blob-objects", ss[0].Anchor)
	assert.Equal(t, "5-1-4-the-fts5-get-locale-function", ss[1].Anchor)
}

// A two-line "See also" is not worth a retrieval unit of its own.
func TestSplit_TinySectionsMergeBackward(t *testing.T) {
	body := long("## Big\n\n" + filler(300) + "## See also\n\nOne line.\n\n" + "## Next\n\n" + filler(300))
	ss := Split(body)
	assert.Equal(t, [][]string{{"Big"}, {"Next"}, {"Padding"}}, paths(ss))
	assert.Contains(t, ss[0].Body, "## See also")
	assert.Equal(t, body, joined(ss))
}

func TestSplit_TinyFirstSectionMergesForward(t *testing.T) {
	body := long("Intro line.\n\n## Real\n\n" + filler(300))
	ss := Split(body)
	assert.Equal(t, []string{"Real"}, ss[0].HeadingPath)
	assert.True(t, strings.HasPrefix(ss[0].Body, "Intro line."))
	assert.Equal(t, body, joined(ss))
}

func TestSplit_PreambleBecomesItsOwnSection(t *testing.T) {
	body := long(filler(300) + "## After\n\n" + filler(300))
	ss := Split(body)
	assert.Empty(t, ss[0].HeadingPath)
	assert.Equal(t, "preamble", ss[0].Anchor)
	assert.Equal(t, body, joined(ss))
}

// A section past the size cap is cut at blank lines — never inside a
// paragraph, never inside a fence.
func TestSplit_OversizedSectionSplitsAtParagraphs(t *testing.T) {
	var big strings.Builder
	big.WriteString("## Huge\n\n")
	for range 6 {
		big.WriteString(filler(800))
	}
	big.WriteString("```\nline one\n\nline after a blank inside the fence\n```\n\n")
	body := long(big.String())
	ss := Split(body)
	var parts []Section
	for _, s := range ss {
		if len(s.HeadingPath) == 1 && s.HeadingPath[0] == "Huge" {
			parts = append(parts, s)
		}
	}
	require.Greater(t, len(parts), 1)
	assert.Equal(t, "huge", parts[0].Anchor)
	assert.Equal(t, "huge-2", parts[1].Anchor)
	for _, p := range parts[:len(parts)-1] {
		assert.LessOrEqual(t, estimateTokens(p.Body), maxSectionTokens)
	}
	for _, p := range parts {
		assert.Equal(t, strings.Count(p.Body, "```")%2, 0, "a fence is never cut in two")
	}
	assert.Equal(t, body, joined(ss))
}

// Cutting an oversized section can leave a tail of a few lines; it joins the
// part before it rather than becoming a unit that carries nothing. Found by
// splitting real imported pages: parts of 23 and 40 tokens.
func TestSplit_ATinyTailOfACutSectionJoinsThePartBefore(t *testing.T) {
	// Each paragraph nearly fills a part, so the closing line cannot fit in
	// the part before it and is left on its own unless merged back.
	body := long("## Long\n\n" + filler(maxSectionTokens-2) + filler(maxSectionTokens-2) + "Last line.\n\n" +
		"## Next\n\n" + filler(300))
	for _, s := range Split(body) {
		if s.HeadingPath[len(s.HeadingPath)-1] == "Long" {
			assert.GreaterOrEqual(t, estimateTokens(s.Body), minSectionTokens, "part %s", s.Anchor)
		}
	}
	assert.Equal(t, body, joined(Split(body)))
}

func TestSplit_AHugeParagraphStaysWhole(t *testing.T) {
	one := strings.TrimSpace(filler(maxSectionTokens*2)) + "\n\n"
	body := long("## Wall\n\n" + one)
	ss := Split(body)
	assert.Equal(t, "## Wall\n\n"+one, ss[0].Body)
}

// A table or list with no blank lines is one paragraph; a generated config
// reference had one of 9,606 tokens — past the window again. Such a block is
// cut at line breaks; a single line longer than the cap still stays whole.
func TestSplit_AnOversizedMultiLineParagraphIsCutAtLines(t *testing.T) {
	var table strings.Builder
	table.WriteString("## Reference\n\n| key | description |\n|---|---|\n")
	for table.Len() < maxSectionTokens*3*4 {
		table.WriteString("| app.some.key | " + strings.Repeat("words ", 30) + "|\n")
	}
	table.WriteString("\n")
	body := long(table.String())
	var parts []Section
	for _, s := range Split(body) {
		if s.HeadingPath[len(s.HeadingPath)-1] == "Reference" {
			parts = append(parts, s)
		}
	}
	require.Greater(t, len(parts), 2)
	for _, p := range parts {
		assert.LessOrEqual(t, estimateTokens(p.Body), maxSectionTokens+minSectionTokens, "part %s", p.Anchor)
	}
	assert.Equal(t, body, joined(Split(body)))
}

// A page at the import cap with no headings: appending to strings made the
// cut quadratic (1 MiB took 4.9 s, 2 MiB 6.2 s); it is linear now (5 MiB in
// ~0.1 s). The bound is generous so a loaded machine cannot fail it, and still
// an order of magnitude under what the quadratic version took.
func TestCut_LargeHeadinglessPageIsFast(t *testing.T) {
	line := strings.Repeat("word ", 15) + "\n"
	body := strings.Repeat(line, 5<<20/len(line))
	start := time.Now()
	ss := Cut(body)
	assert.Less(t, time.Since(start), 3*time.Second)
	assert.Equal(t, body, joined(ss))
}

func TestSplit_DuplicateHeadingsGetUniqueAnchors(t *testing.T) {
	body := long("## Example\n\n" + filler(300) + "## Example\n\n" + filler(300))
	ss := Split(body)
	assert.Equal(t, "example", ss[0].Anchor)
	assert.Equal(t, "example-2", ss[1].Anchor)
	assert.Equal(t, []int{0, 1, 2}, []int{ss[0].Ordinal, ss[1].Ordinal, ss[2].Ordinal})
}

func TestSplit_CRLFAndNoTrailingNewlineRoundTrip(t *testing.T) {
	body := long("## A\r\n\r\n"+strings.ReplaceAll(filler(300), "\n", "\r\n")+"## B\r\n\r\n"+filler(300)) + "tail without newline"
	assert.Equal(t, body, joined(Split(body)))
}

// The index stores a note's text stripped of markdown, and that stripped text
// is what the model is given — so whether to split is decided on it, while the
// cut is made in the markdown, where the headings are. Found by indexing real
// pages: the stripped text has no headings, so splitting it cut only by length.
func TestNeededAndCut(t *testing.T) {
	assert.False(t, Needed(strings.Repeat("a", splitThreshold*4)))
	assert.True(t, Needed(strings.Repeat("a", splitThreshold*4+1)))

	md := "## One\n\n" + filler(300) + "## Two\n\n" + filler(300)
	ss := Cut(md)
	require.Len(t, ss, 2, "Cut always cuts, whatever the length")
	assert.Equal(t, []string{"Two"}, ss[1].HeadingPath)
	assert.Equal(t, md, joined(ss))
	assert.Nil(t, Cut(""))
}

func TestPrefix(t *testing.T) {
	assert.Equal(t, "SQLite FTS5 › 6. Special INSERT Commands › 6.9. The 'optimize' Command",
		Prefix("SQLite FTS5", Section{HeadingPath: []string{"6. Special INSERT Commands", "6.9. The 'optimize' Command"}}))
	assert.Equal(t, "SQLite FTS5", Prefix("SQLite FTS5", Section{}))
}

// A section's stored text opens with its own heading, which delivery names
// separately. Left in, an excerpt took a numbered heading ("7.1.1. Synonym
// Support" has a full stop) for the opening sentence and delivered only that.
func TestWithoutHeading(t *testing.T) {
	path := "7.1. Custom Tokenizers" + PathSeparator + "7.1.1. Synonym Support"
	assert.Equal(t, "There are several ways.", WithoutHeading("7.1.1. Synonym Support\n\nThere are several ways.", path))
	assert.Equal(t, "There are several ways.", WithoutHeading("  7.1.1. Synonym Support  \r\n\r\nThere are several ways.", path))
	assert.Equal(t, "Text before any heading.", WithoutHeading("Text before any heading.", ""), "a preamble has no heading")
	assert.Equal(t, "Other line\n\nbody", WithoutHeading("Other line\n\nbody", path), "only the section's own heading goes")
	assert.Equal(t, "7.1.1. Synonym Support", WithoutHeading("7.1.1. Synonym Support", path), "a heading with nothing under it stays")
}
