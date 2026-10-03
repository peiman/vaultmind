package importdocs_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chapter is one spine document of a test book.
type chapter struct {
	href, title, body string
	linearNo          bool
	// entries are table of contents entries pointing inside the document;
	// when set they replace the one entry for title.
	entries [][2]string // {frag, title}
}

// book describes a test EPUB.
type book struct {
	epub3      bool // a nav document; otherwise an NCX
	chapters   []chapter
	encryption string // META-INF/encryption.xml, when set
	noTOC      bool   // neither nav nor NCX
	landmarks  bool   // a second nav list of page links, which are not chapters
}

func xhtml(body string) string {
	return `<?xml version="1.0" encoding="utf-8"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title>HEAD-TITLE-UNIQUE</title></head><body>` + body + `</body></html>`
}

// writeEPUB writes b as an EPUB under OEBPS/.
func writeEPUB(t *testing.T, p string, b book) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	add("mimetype", "application/epub+zip")
	add("META-INF/container.xml", `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)
	if b.encryption != "" {
		add("META-INF/encryption.xml", b.encryption)
	}
	var manifest, spine, nav, ncx strings.Builder
	for i, c := range b.chapters {
		fmt.Fprintf(&manifest, `<item id="c%d" href="%s" media-type="application/xhtml+xml"/>`, i, c.href)
		linear := ""
		if c.linearNo {
			linear = ` linear="no"`
		}
		fmt.Fprintf(&spine, `<itemref idref="c%d"%s/>`, i, linear)
		add("OEBPS/"+c.href, xhtml(c.body))
		if c.title != "" {
			fmt.Fprintf(&nav, `<li><a href="%s#start">%s</a></li>`, c.href, c.title)
			fmt.Fprintf(&ncx, `<navPoint id="n%d"><navLabel><text>%s</text></navLabel><content src="%s"/></navPoint>`, i, c.title, c.href)
		}
		for j, e := range c.entries {
			fmt.Fprintf(&nav, `<li><a href="%s#%s">%s</a></li>`, c.href, e[0], e[1])
			fmt.Fprintf(&ncx, `<navPoint id="n%d-%d"><navLabel><text>%s</text></navLabel><content src="%s#%s"/></navPoint>`, i, j, e[1], c.href, e[0])
		}
	}
	toc := ""
	switch {
	case b.noTOC:
	case b.epub3:
		manifest.WriteString(`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>`)
		marks := ""
		if b.landmarks {
			marks = `<nav epub:type="landmarks"><ol><li><a href="` + b.chapters[0].href + `#p7">{vii}</a></li></ol></nav>`
		}
		add("OEBPS/nav.xhtml", xhtml(marks+`<nav xmlns:epub="http://www.idpf.org/2007/ops" epub:type="toc"><ol>`+nav.String()+`</ol></nav>`))
	default:
		manifest.WriteString(`<item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>`)
		add("OEBPS/toc.ncx", `<?xml version="1.0"?><ncx xmlns="http://www.daisy.org/z3986/2005/ncx/"><navMap>`+ncx.String()+`</navMap></ncx>`)
		toc = ` toc="ncx"`
	}
	add("OEBPS/content.opf", `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>The Test Book</dc:title><dc:creator>Ada Writer</dc:creator><dc:language>en</dc:language></metadata><manifest>`+
		manifest.String()+`</manifest><spine`+toc+`>`+spine.String()+`</spine></package>`)
	require.NoError(t, zw.Close())
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
	require.NoError(t, os.WriteFile(p, buf.Bytes(), 0o600))
}

func epubPara(s string) string {
	return "<p>" + strings.Repeat(s+" ", 5) + "</p>"
}

func twoChapterBook(epub3 bool) book {
	return book{epub3: epub3, chapters: []chapter{
		// An image wrapper is not a chapter, though its alt text and its link
		// back into the book are words (Gutenberg's back cover).
		{href: "cover.xhtml", body: `<div><img src="cover.jpg" alt="Cover"/><br/><a href="ch1.xhtml#x" title="back">back</a></div>`},
		{href: "ch1.xhtml", title: "Chapter One: Arrival", body: `<h1>Arrival</h1>` + epubPara("The traveller reached the harbour at dawn.") +
			`<p><img src="map.png" alt="A map of the harbour"/> See <a href="ch2.xhtml#x">the next chapter</a> or <a href="https://example.org/">the web</a>.</p>`},
		{href: "ch2.xhtml", title: "Chapter Two: Departure", body: `<h1>Departure</h1>` + epubPara("The ship left the harbour at dusk.")},
	}}
}

func noteAt(t *testing.T, vault, rel string) (map[string]interface{}, string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(rel))) //nolint:gosec // test path
	require.NoError(t, err)
	fm, body, err := parser.ExtractFrontmatter(raw)
	require.NoError(t, err)
	return fm, body
}

const bookDir = "imported/demo-repo/docs/voyage-epub/"

func TestImport_AnEPUBBecomesOneNotePerChapterAndABookNote(t *testing.T) {
	for _, epub3 := range []bool{true, false} {
		t.Run(fmt.Sprintf("epub3=%v", epub3), func(t *testing.T) {
			repo, vault := srcRepo(t), t.TempDir()
			writeEPUB(t, filepath.Join(repo, "docs", "voyage.epub"), twoChapterBook(epub3))

			res := run(t, repo, vault, importdocs.Options{})
			assert.Equal(t, 5, res.Count(importdocs.Added), "two markdown docs, two chapters, the book; the cover has no text")

			fm, body := noteAt(t, vault, bookDir+"01-chapter-one-arrival.md")
			assert.Equal(t, "Chapter One: Arrival", fm["title"])
			assert.Equal(t, "demo-repo:docs/voyage.epub#ch1.xhtml", fm["source"])
			assert.Equal(t, []interface{}{"demo-repo:docs/voyage.epub"}, fm["paths"])
			assert.Contains(t, body, "The traveller reached the harbour at dawn.")
			assert.Contains(t, body, "A map of the harbour", "an image becomes its alt text")
			assert.NotContains(t, body, "map.png")
			assert.Contains(t, body, "See the next chapter or [the web](https://example.org/)", "a link inside the book becomes its text")
			assert.NotContains(t, body, "HEAD-TITLE-UNIQUE")

			fm, body = noteAt(t, vault, bookDir+"book.md")
			assert.Equal(t, "The Test Book", fm["title"])
			assert.Contains(t, body, "Ada Writer")
			one := strings.Index(body, "[["+bookDir+"01-chapter-one-arrival|Chapter One: Arrival]]")
			two := strings.Index(body, "[["+bookDir+"02-chapter-two-departure|Chapter Two: Departure]]")
			assert.True(t, one >= 0 && two > one, "chapters in spine order:\n%s", body)

			again := run(t, repo, vault, importdocs.Options{})
			assert.Equal(t, 5, again.Count(importdocs.Unchanged))
		})
	}
}

func TestImport_AChapterWithoutATOCEntryIsNamedByItsHeadingOrPlace(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	writeEPUB(t, filepath.Join(repo, "docs", "voyage.epub"), book{noTOC: true, chapters: []chapter{
		{href: "a.xhtml", body: `<h2>The Harbour</h2>` + epubPara("Words about the harbour.")},
		{href: "b.xhtml", body: epubPara("Words with no heading at all.")},
		{href: "notes.xhtml", linearNo: true, body: epubPara("Endnotes kept because they hold text.")},
	}})
	run(t, repo, vault, importdocs.Options{})
	fm, _ := noteAt(t, vault, bookDir+"01-the-harbour.md")
	assert.Equal(t, "The Harbour", fm["title"])
	fm, _ = noteAt(t, vault, bookDir+"02-chapter-2.md")
	assert.Equal(t, "Chapter 2", fm["title"])
	assert.FileExists(t, filepath.Join(vault, bookDir+"03-chapter-3.md"))
}

func TestImport_ADRMBookIsSkippedButObfuscatedFontsAreNot(t *testing.T) {
	enc := func(uri string) string {
		return `<encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container" xmlns:enc="http://www.w3.org/2001/04/xmlenc#"><enc:EncryptedData><enc:EncryptionMethod Algorithm="http://www.idpf.org/2008/embedding"/><enc:CipherData><enc:CipherReference URI="` + uri + `"/></enc:CipherData></enc:EncryptedData></encryption>`
	}
	repo, vault := srcRepo(t), t.TempDir()
	fonts := twoChapterBook(true)
	fonts.encryption = enc("OEBPS/fonts/serif.otf")
	writeEPUB(t, filepath.Join(repo, "docs", "voyage.epub"), fonts)
	drm := twoChapterBook(true)
	drm.encryption = enc("OEBPS/ch1.xhtml")
	writeEPUB(t, filepath.Join(repo, "docs", "locked.epub"), drm)

	broken := twoChapterBook(true)
	broken.encryption = "<encryption><not closed"
	writeEPUB(t, filepath.Join(repo, "docs", "unsure.epub"), broken)

	res := run(t, repo, vault, importdocs.Options{})
	assert.FileExists(t, filepath.Join(vault, bookDir+"book.md"))
	e, ok := entryFor(res, "unsure.epub")
	require.True(t, ok)
	assert.Contains(t, e.Reason, "cannot tell whether it is DRM-protected")
	e, ok = entryFor(res, "locked.epub")
	require.True(t, ok)
	assert.Equal(t, importdocs.Skipped, e.Action)
	assert.Contains(t, e.Reason, "DRM-protected")
	assert.NoDirExists(t, filepath.Join(vault, "imported/demo-repo/docs/locked-epub"))
}

func TestImport_AChapterGoneFromItsBookIsAnOrphanButAnUnreadableBookKeepsItsNotes(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	p := filepath.Join(repo, "docs", "voyage.epub")
	writeEPUB(t, p, twoChapterBook(true))
	run(t, repo, vault, importdocs.Options{})

	short := twoChapterBook(true)
	short.chapters = short.chapters[:2]
	writeEPUB(t, p, short)
	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, []string{bookDir + "02-chapter-two-departure.md"}, notesBy(res, importdocs.Orphaned))

	write(t, p, "not a zip any more")
	res = run(t, repo, vault, importdocs.Options{})
	assert.Empty(t, notesBy(res, importdocs.Orphaned), "a book that cannot be read is not a book without chapters")
	e, ok := entryFor(res, "voyage.epub")
	require.True(t, ok)
	assert.Equal(t, importdocs.Skipped, e.Action)

	require.NoError(t, os.Remove(p))
	res = run(t, repo, vault, importdocs.Options{Prune: true})
	assert.Len(t, notesBy(res, importdocs.Pruned), 3, "the book is gone: its notes go with --prune")
}

func TestImportFile_AnEPUBImportsAloneWithAllItsNotes(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	writeEPUB(t, filepath.Join(repo, "docs", "voyage.epub"), twoChapterBook(false))
	res, err := importdocs.ImportFile(source(repo), "voyage.epub", vault, importdocs.Options{})
	require.NoError(t, err)
	assert.Equal(t, 3, res.Count(importdocs.Added))
}

// Gutenberg packs ten chapters into one file. A document longer than one
// note should be is split at its table of contents entries, each piece
// titled by its entry; a short one with several entries stays whole.
func TestImport_ALongDocumentIsSplitAtItsTableOfContentsEntries(t *testing.T) {
	long := func(s string) string { return strings.Repeat(epubPara(s), 70) } // ~10,000 characters
	big := chapter{href: "part1.xhtml",
		body: `<p id="p7">Title page words.</p>` +
			`<div><h2 id="c1">CHAPTER I.</h2>` + long("It is a truth universally acknowledged.") + `</div>` +
			`<div><h2 id="c2">CHAPTER II.</h2>` + long("Mr. Bennet was among the earliest.") + `</div>` +
			`<div><h2 id="c3">CHAPTER III.</h2>` + long("Not all that Mrs. Bennet could ask.") + `</div>`,
		entries: [][2]string{{"c1", "Chapter I."}, {"c2", "Chapter II."}, {"c3", "Chapter III."}}}
	small := chapter{href: "front.xhtml", body: `<h1 id="t">The Book</h1><p id="e">Edition 3.0</p>`,
		entries: [][2]string{{"t", "The Book"}, {"e", "Edition"}}}
	for _, epub3 := range []bool{true, false} {
		t.Run(fmt.Sprintf("epub3=%v", epub3), func(t *testing.T) {
			repo, vault := srcRepo(t), t.TempDir()
			p := filepath.Join(repo, "docs", "voyage.epub")
			writeEPUB(t, p, book{epub3: epub3, landmarks: true, chapters: []chapter{small, big}})

			res := run(t, repo, vault, importdocs.Options{})
			assert.Equal(t, 2+1+3+1, res.Count(importdocs.Added), "the front matter whole, three chapters, the book")
			fm, body := noteAt(t, vault, bookDir+"02-chapter-i.md")
			assert.Equal(t, "demo-repo:docs/voyage.epub#part1.xhtml#c1", fm["source"])
			assert.Contains(t, body, "Title page words.", "text before the first entry joins it")
			assert.Contains(t, body, "universally acknowledged")
			assert.NotContains(t, body, "earliest")
			assert.NotContains(t, body, "VAULTMINDSPLIT")
			_, body = noteAt(t, vault, bookDir+"04-chapter-iii.md")
			assert.Contains(t, body, "Not all that Mrs. Bennet could ask.")
			assert.NotContains(t, body, "earliest")
			fm, _ = noteAt(t, vault, bookDir+"01-the-book.md")
			assert.Equal(t, "The Book", fm["title"], "the page list is not the table of contents")

			// A chapter dropped from a split document is gone from the book.
			shorter := big
			shorter.body = strings.Split(big.body, `<div><h2 id="c3">`)[0]
			shorter.entries = big.entries[:2]
			shorter.body += strings.Repeat(epubPara("Padding so the document is still long."), 40)
			writeEPUB(t, p, book{epub3: epub3, chapters: []chapter{small, shorter}})
			res = run(t, repo, vault, importdocs.Options{})
			assert.Equal(t, []string{bookDir + "04-chapter-iii.md"}, notesBy(res, importdocs.Orphaned))
		})
	}
}

// Manifest hrefs are IRIs relative to the package document: percent-escaped,
// and free to climb out of its folder.
func TestImport_AnEPUBsHrefsAreDecodedAndResolved(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	add("META-INF/container.xml", `<container><rootfiles><rootfile full-path="OEBPS/content.opf"/></rootfiles></container>`)
	add("OEBPS/content.opf", `<package><metadata><title>Paths</title></metadata><manifest>`+
		`<item id="a" href="ch%201.xhtml" media-type="application/xhtml+xml"/>`+
		`<item id="b" href="../text/ch2.xhtml" media-type="application/xhtml+xml"/>`+
		`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>`+
		`</manifest><spine><itemref idref="a"/><itemref idref="b"/></spine></package>`)
	add("OEBPS/ch 1.xhtml", xhtml(epubPara("The first chapter has a space in its file name.")))
	add("text/ch2.xhtml", xhtml(epubPara("The second chapter lives outside the package folder.")))
	add("OEBPS/nav.xhtml", xhtml(`<nav epub:type="toc"><ol><li><a href="ch%201.xhtml">Spaced</a></li><li><a href="../text/ch2.xhtml">Outside</a></li></ol></nav>`))
	require.NoError(t, zw.Close())
	write(t, filepath.Join(repo, "docs", "paths.epub"), buf.String())

	run(t, repo, vault, importdocs.Options{})
	dir := "imported/demo-repo/docs/paths-epub/"
	fm, body := noteAt(t, vault, dir+"01-spaced.md")
	assert.Equal(t, "Spaced", fm["title"])
	assert.Contains(t, body, "space in its file name")
	fm, body = noteAt(t, vault, dir+"02-outside.md")
	assert.Equal(t, "Outside", fm["title"])
	assert.Contains(t, body, "outside the package folder")
}

// A table of contents may list a file's sections out of document order.
// The cut follows the document; each piece keeps its own entry's title.
func TestImport_ASplitFollowsTheDocumentNotTheTOCOrder(t *testing.T) {
	long := func(s string) string { return strings.Repeat(epubPara(s), 70) }
	repo, vault := srcRepo(t), t.TempDir()
	writeEPUB(t, filepath.Join(repo, "docs", "voyage.epub"), book{epub3: true, chapters: []chapter{{href: "all.xhtml",
		body:    `<h2 id="a">A</h2>` + long("Alpha words fill this section of the chapter.") + `<h2 id="b">B</h2>` + long("Bravo words fill this section of the chapter.") + `<h2 id="c">C</h2>` + long("Charlie words fill this section of the chapter."),
		entries: [][2]string{{"c", "Section C"}, {"a", "Section A"}, {"b", "Section B"}}}}})
	run(t, repo, vault, importdocs.Options{})
	for name, want := range map[string]string{"01-section-a.md": "Alpha", "02-section-b.md": "Bravo", "03-section-c.md": "Charlie"} {
		_, body := noteAt(t, vault, bookDir+name)
		assert.Contains(t, body, want, name)
	}
	_, body := noteAt(t, vault, bookDir+"01-section-a.md")
	assert.NotContains(t, body, "Bravo")
}

// The book note links each chapter by its path, so a chapter whose id had to
// be suffixed is still reached.
func TestImport_TheBookNoteLinksAChapterWhoseIDWasTaken(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(vault, "taken.md"), "---\nid: imported-demo-repo-docs-voyage-epub-01-chapter-one-arrival\ntype: reference\ntitle: Taken\n---\nAn id the chapter wants.\n")
	writeEPUB(t, filepath.Join(repo, "docs", "voyage.epub"), twoChapterBook(true))
	run(t, repo, vault, importdocs.Options{})
	_, body := noteAt(t, vault, bookDir+"book.md")
	assert.Contains(t, body, "[["+bookDir+"01-chapter-one-arrival|")
}
