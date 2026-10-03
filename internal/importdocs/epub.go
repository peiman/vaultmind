package importdocs

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	xhtml "golang.org/x/net/html"
)

// maxChapterSlug caps the title part of a chapter's note name: a file name
// holds at most 255 bytes, and a book's chapter titles can be sentences.
const maxChapterSlug = 60

// epubBook is what a book's package document says.
type epubBook struct {
	title, creator, language string
	opfDir                   string
	spine                    []epubItem
	tocHref                  string // the NCX, when the spine names one
	navHref                  string // the EPUB 3 nav document, when there is one
}

// epubItem is one manifest entry the spine reads, its href resolved from
// the zip root.
type epubItem struct {
	href, mediaType string
}

// readEPUBDocs reads a book as one doc per spine document that holds text,
// plus a book doc listing them. Each chapter's note sits in <book>-epub/,
// named by its place and title; its source is the book and the chapter's
// href, its `paths:` the book. A book whose chapters are encrypted (DRM) is
// refused; encrypted fonts are not DRM.
func readEPUBDocs(src Source, rel, p string) ([]doc, error) {
	parts, closeFn, err := openEPUB(p)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	b, err := readPackage(parts)
	if err != nil {
		return nil, err
	}
	if err := refuseDRM(parts, b); err != nil {
		return nil, err
	}
	toc := tocEntries(parts, b)
	docPath := path.Join(src.Prefix, rel)
	folder := strings.TrimSuffix(rel, path.Ext(rel)) + "-epub"
	folderPath := strings.TrimSuffix(docPath, path.Ext(docPath)) + "-epub"
	book := src.Repo + ":" + docPath
	var pieces []chapterPiece
	for _, item := range b.spine {
		pieces = append(pieces, chapterPieces(parts, item, toc[item.href], strings.TrimPrefix(item.href, b.opfDir))...)
	}
	if len(pieces) == 0 {
		return nil, errors.New("the epub file holds no text")
	}
	docs := make([]doc, 0, len(pieces)+1)
	for i, pc := range pieces {
		title := chapterTitle(pc.title, pc.body, i+1)
		name := chapterName(i+1, len(pieces), title, pc.part)
		docs = append(docs, doc{
			Rel: path.Join(folder, name), Path: path.Join(folderPath, name), Repo: src.Repo,
			Source: book + "#" + pc.part, PathsEntry: book,
			Title: title, Body: pc.body, Hash: hashOf(pc.body),
		})
	}
	return append(docs, bookDoc(src, b, folder, folderPath, book, docs, rel)), nil
}

// openEPUB opens the book's zip, reading parts straight from the file under
// the Office caps.
func openEPUB(p string) (officeParts, func(), error) {
	// nosemgrep: go-path-traversal -- a file found by walking the folder the operator named; symlinks were skipped
	f, err := os.Open(p) //nolint:gosec // same
	if err != nil {
		return officeParts{}, nil, err
	}
	closeFn := func() { _ = f.Close() }
	info, err := f.Stat()
	if err != nil {
		closeFn()
		return officeParts{}, nil, err
	}
	if info.Size() > maxOfficeBytes {
		closeFn()
		return officeParts{}, nil, fmt.Errorf("epub file is larger than %d MiB", maxOfficeBytes>>20)
	}
	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		closeFn()
		return officeParts{}, nil, fmt.Errorf("not a readable epub file: %w", err)
	}
	if len(zr.File) > maxOfficeParts {
		closeFn()
		return officeParts{}, nil, fmt.Errorf("epub holds more than %d parts", maxOfficeParts)
	}
	parts := officeParts{files: map[string]*zip.File{}}
	for _, zf := range zr.File {
		parts.files[zf.Name] = zf
	}
	return parts, closeFn, nil
}

// readPackage reads container.xml, then the package document it names: the
// metadata, the manifest and the spine.
func readPackage(parts officeParts) (epubBook, error) {
	raw, err := parts.read("META-INF/container.xml")
	if err != nil {
		return epubBook{}, errors.New("not an epub file: no META-INF/container.xml")
	}
	var container struct {
		Rootfiles []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if xml.Unmarshal(raw, &container) != nil || len(container.Rootfiles) == 0 {
		return epubBook{}, errors.New("not an epub file: container.xml names no package")
	}
	opfPath := container.Rootfiles[0].FullPath
	raw, err = parts.read(opfPath)
	if err != nil {
		return epubBook{}, fmt.Errorf("reading the package document: %w", err)
	}
	return parsePackage(raw, path.Dir(opfPath))
}

// opfPackage is the package document's XML.
type opfPackage struct {
	Title    []string `xml:"metadata>title"`
	Creator  []string `xml:"metadata>creator"`
	Language []string `xml:"metadata>language"`
	Items    []struct {
		ID         string `xml:"id,attr"`
		Href       string `xml:"href,attr"`
		MediaType  string `xml:"media-type,attr"`
		Properties string `xml:"properties,attr"`
	} `xml:"manifest>item"`
	Spine struct {
		TOC  string `xml:"toc,attr"`
		Refs []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

func parsePackage(raw []byte, opfDir string) (epubBook, error) {
	var pkg opfPackage
	if err := xml.Unmarshal(raw, &pkg); err != nil {
		return epubBook{}, fmt.Errorf("reading the package document: %w", err)
	}
	if opfDir == "." {
		opfDir = ""
	} else {
		opfDir += "/"
	}
	b := epubBook{title: first(pkg.Title), creator: first(pkg.Creator), language: first(pkg.Language), opfDir: opfDir}
	byID := map[string]epubItem{}
	for _, it := range pkg.Items {
		item := epubItem{href: resolveHref(opfDir, it.Href), mediaType: it.MediaType}
		byID[it.ID] = item
		if strings.Contains(" "+it.Properties+" ", " nav ") {
			b.navHref = item.href
		}
	}
	if it, ok := byID[pkg.Spine.TOC]; ok {
		b.tocHref = it.href
	}
	for _, ref := range pkg.Spine.Refs {
		if it, ok := byID[ref.IDRef]; ok {
			b.spine = append(b.spine, it)
		}
	}
	return b, nil
}

// resolveHref is the zip entry an href names from dir: its fragment
// dropped, percent-escapes decoded (ch%201.xhtml is the entry "ch 1.xhtml"),
// and ../ resolved. An href that does not decode is taken as it is.
func resolveHref(dir, href string) string {
	file, _, _ := strings.Cut(href, "#")
	if dec, err := url.PathUnescape(file); err == nil {
		file = dec
	}
	return strings.TrimPrefix(path.Join(dir, file), "/")
}

func first(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return strings.TrimSpace(s[0])
}

// refuseDRM refuses a book whose spine documents are encrypted. Fonts
// listed in encryption.xml are obfuscated, not locked, and do not count.
//
// An encryption.xml that is there but cannot be read refuses the book too:
// encrypted chapters would import as noise.
func refuseDRM(parts officeParts, b epubBook) error {
	const name = "META-INF/encryption.xml"
	if _, ok := parts.files[name]; !ok {
		return nil
	}
	raw, err := parts.read(name)
	if err != nil {
		return fmt.Errorf("cannot tell whether it is DRM-protected: %w", err)
	}
	var enc struct {
		Refs []struct {
			URI string `xml:"URI,attr"`
		} `xml:"EncryptedData>CipherData>CipherReference"`
	}
	if err := xml.Unmarshal(raw, &enc); err != nil {
		return fmt.Errorf("cannot tell whether it is DRM-protected: reading %s: %w", name, err)
	}
	spine := map[string]bool{}
	for _, it := range b.spine {
		spine[it.href] = true
	}
	for _, r := range enc.Refs {
		if spine[resolveHref("", r.URI)] {
			return errors.New("DRM-protected: its chapters are encrypted, so it cannot be read")
		}
	}
	return nil
}

// tocEntry is one table of contents entry inside a content document: the
// fragment it points at ("" for the document's start) and its title.
type tocEntry struct {
	frag, title string
}

// tocEntries maps each content document to the table of contents entries
// that point into it, in order: from the EPUB 3 nav document's toc, else
// the EPUB 2 NCX.
func tocEntries(parts officeParts, b epubBook) map[string][]tocEntry {
	if b.navHref != "" {
		if raw, err := parts.read(b.navHref); err == nil {
			if out := navEntries(raw, path.Dir(b.navHref)); len(out) > 0 {
				return out
			}
		}
	}
	if b.tocHref != "" {
		if raw, err := parts.read(b.tocHref); err == nil {
			return ncxEntries(raw, path.Dir(b.tocHref))
		}
	}
	return map[string][]tocEntry{}
}

// navEntries reads the links of a nav document's table of contents. Its
// other lists (landmarks, page lists) are not chapters: one book's page
// list held 453 links. A nav document with no toc-typed list is read whole.
func navEntries(raw []byte, dir string) map[string][]tocEntry {
	out := map[string][]tocEntry{}
	doc, err := xhtml.Parse(bytes.NewReader(raw))
	if err != nil {
		return out
	}
	root := tocNav(doc)
	if root == nil {
		root = doc
	}
	var walk func(n *xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					addEntry(out, dir, a.Val, nodeText(n))
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

// tocNav is the <nav> whose epub:type is toc, or nil.
func tocNav(n *xhtml.Node) *xhtml.Node {
	if n.Type == xhtml.ElementNode && n.Data == "nav" {
		for _, a := range n.Attr {
			if (a.Key == "epub:type" || (a.Namespace == "epub" && a.Key == "type")) &&
				strings.Contains(" "+a.Val+" ", " toc ") {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if m := tocNav(c); m != nil {
			return m
		}
	}
	return nil
}

func nodeText(n *xhtml.Node) string {
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// ncxEntries reads the navPoints of an NCX, in document order.
func ncxEntries(raw []byte, dir string) map[string][]tocEntry {
	out := map[string][]tocEntry{}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var label string
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "text":
			var s string
			if dec.DecodeElement(&s, &se) == nil {
				label = strings.Join(strings.Fields(s), " ")
			}
		case "content":
			addEntry(out, dir, attr(se, "src"), label)
		}
	}
}

// addEntry records a table of contents entry for the document href points
// into. A fragment already recorded is not recorded again.
func addEntry(out map[string][]tocEntry, dir, href, title string) {
	file, frag, _ := strings.Cut(href, "#")
	if file == "" || title == "" {
		return
	}
	key := resolveHref(dir, file)
	for _, e := range out[key] {
		if e.frag == frag {
			return
		}
	}
	out[key] = append(out[key], tocEntry{frag: frag, title: title})
}

// markdownImage is an image in converted markdown; its alt text stays.
var markdownImage = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)

// markdownLinkFull is a link in converted markdown, for dropping the ones
// that point inside the book.
var markdownLinkFull = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]*)[^)]*\)`)

// maxChapterChars is the longest a chapter note runs before its document is
// split at its table of contents entries: about 6,000 tokens, inside the
// 8,192 an embedding covers. Gutenberg packs ten chapters into one file.
const maxChapterChars = 24000

// splitMark is put before each entry's element, so the converted markdown
// can be cut where each chapter begins.
const splitMark = "VAULTMINDSPLIT7Q"

// chapterPiece is one note's worth of a spine document: its markdown, the
// title of the entry it starts at, and the part of the book it is
// (file.xhtml, or file.xhtml#frag when the file was split).
type chapterPiece struct {
	body, title, part string
}

// chapterPieces converts one spine document: whole, titled by its first
// entry, or, when it runs past maxChapterChars and several entries point
// into it, one piece per entry. Text before the first entry joins the
// first piece. A document with no text yields nothing.
func chapterPieces(parts officeParts, item epubItem, entries []tocEntry, part string) []chapterPiece {
	if item.mediaType != "application/xhtml+xml" && item.mediaType != "text/html" {
		return nil
	}
	raw, err := parts.read(item.href)
	if err != nil {
		return nil
	}
	whole := chapterText(string(raw))
	if whole == "" {
		return nil
	}
	first := ""
	if len(entries) > 0 {
		first = entries[0].title
	}
	if len(whole) <= maxChapterChars || len(entries) < 2 {
		return []chapterPiece{{body: whole, title: first, part: part}}
	}
	if split := splitAtEntries(string(raw), entries, part); len(split) > 1 {
		return split
	}
	return []chapterPiece{{body: whole, title: first, part: part}}
}

// splitAtEntries marks each entry's element in the document, converts it
// once, and cuts the markdown at the marks. Entries whose element is not
// found are passed over.
func splitAtEntries(raw string, entries []tocEntry, part string) []chapterPiece {
	doc, err := xhtml.Parse(strings.NewReader(raw))
	if err != nil {
		return nil
	}
	ids := map[string]*xhtml.Node{}
	order := map[*xhtml.Node]int{}
	indexIDs(doc, ids, order)
	// Cut in document order: a table of contents may list sections in
	// another order, and the markdown can only be cut in the order it runs.
	entries = append([]tocEntry(nil), entries...)
	sort.SliceStable(entries, func(i, j int) bool {
		return nodeOrder(ids[entries[i].frag], order) < nodeOrder(ids[entries[j].frag], order)
	})
	var marked []tocEntry
	for _, e := range entries {
		n := ids[e.frag]
		if e.frag == "" || n == nil || n.Parent == nil {
			continue
		}
		mark := &xhtml.Node{Type: xhtml.ElementNode, Data: "p"}
		mark.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: fmt.Sprintf("%s%d", splitMark, len(marked))})
		n.Parent.InsertBefore(mark, n)
		marked = append(marked, e)
	}
	var b strings.Builder
	if len(marked) < 2 || xhtml.Render(&b, doc) != nil {
		return nil
	}
	md := chapterText(b.String())
	var out []chapterPiece
	for i, e := range marked {
		start := strings.Index(md, fmt.Sprintf("%s%d", splitMark, i))
		if start < 0 {
			return nil
		}
		end := len(md)
		if i+1 < len(marked) {
			if next := strings.Index(md, fmt.Sprintf("%s%d", splitMark, i+1)); next > start {
				end = next
			}
		}
		if i == 0 {
			start = 0 // text before the first entry joins it
		}
		body := normalizeMarkdown(markLine.ReplaceAllString(md[start:end], ""))
		if strings.TrimSpace(body) != "" {
			out = append(out, chapterPiece{body: body, title: e.title, part: part + "#" + e.frag})
		}
	}
	return out
}

// markLine is a split mark as html-to-markdown leaves it: a paragraph of
// its own.
var markLine = regexp.MustCompile(splitMark + `\d+\s*`)

// indexIDs maps every id (and every <a name>) in the document to its node,
// and every element to its place in document order.
func indexIDs(n *xhtml.Node, ids map[string]*xhtml.Node, order map[*xhtml.Node]int) {
	if n.Type == xhtml.ElementNode {
		order[n] = len(order)
		for _, a := range n.Attr {
			if a.Key == "id" || (a.Key == "name" && n.Data == "a") {
				if _, seen := ids[a.Val]; !seen {
					ids[a.Val] = n
				}
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		indexIDs(c, ids, order)
	}
}

// nodeOrder is n's place in document order; a missing node sorts last.
func nodeOrder(n *xhtml.Node, order map[*xhtml.Node]int) int {
	if i, ok := order[n]; ok && n != nil {
		return i
	}
	return len(order)
}

// chapterText converts a chapter's HTML to markdown. A chapter is all
// content, so no readability: images become their alt text and links inside
// the book their text, since neither leads anywhere from a note.
func chapterText(html string) string {
	md, err := htmltomarkdown.ConvertString(html)
	if err != nil {
		return ""
	}
	if strings.TrimSpace(innerLinks(markdownImage.ReplaceAllString(md, ""), false)) == "" {
		return "" // pictures and links back into the book: a cover or a plate, not a chapter
	}
	return normalizeMarkdown(innerLinks(markdownImage.ReplaceAllString(md, "$1"), true))
}

// innerLinks drops the links that point inside the book: to their text when
// keepText, else entirely. Links out (http, mailto) stay.
func innerLinks(md string, keepText bool) string {
	return markdownLinkFull.ReplaceAllStringFunc(md, func(m string) string {
		sub := markdownLinkFull.FindStringSubmatch(m)
		if strings.Contains(sub[2], "://") || strings.HasPrefix(sub[2], "mailto:") {
			return m
		}
		if keepText {
			return sub[1]
		}
		return ""
	})
}

// chapterTitle is the table of contents' title, else the chapter's first
// heading, else its place.
func chapterTitle(toc, body string, n int) string {
	if toc != "" {
		return toc
	}
	if h := firstHeading(body); h != "" {
		return h
	}
	return fmt.Sprintf("Chapter %d", n)
}

// atxHeading is a markdown heading of any level.
var atxHeading = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*#*$`)

// firstHeading is the text of a chapter's first heading, whatever its
// level: books open chapters with h1 to h3. Fenced code is passed over.
func firstHeading(body string) string {
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if m := atxHeading.FindStringSubmatch(trimmed); !inFence && m != nil {
			return m[1]
		}
	}
	return ""
}

// chapterName is NN-<title>.md, zero-padded to the count's width so the
// files sort in reading order. The title slug is capped; an empty slug
// falls back to the part's file name.
func chapterName(n, total int, title, part string) string {
	slugged := slug(title)
	if slugged == "" {
		file, _, _ := strings.Cut(part, "#")
		slugged = slug(strings.TrimSuffix(path.Base(file), path.Ext(file)))
	}
	if r := []rune(slugged); len(r) > maxChapterSlug {
		slugged = strings.TrimRight(string(r[:maxChapterSlug]), "-")
	}
	width := len(fmt.Sprint(total))
	if width < 2 {
		width = 2
	}
	return fmt.Sprintf("%0*d-%s.md", width, n, slugged)
}

// bookDoc is the book's own note: its title, author and language, and its
// chapters in reading order, linked.
func bookDoc(src Source, b epubBook, folder, folderPath, book string, chapters []doc, rel string) doc {
	title := b.title
	if title == "" {
		title = strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	}
	var body strings.Builder
	fmt.Fprintf(&body, "# %s\n\n", title)
	if b.creator != "" {
		fmt.Fprintf(&body, "By %s.\n\n", b.creator)
	}
	if b.language != "" {
		fmt.Fprintf(&body, "Language: %s.\n\n", b.language)
	}
	body.WriteString("## Chapters\n\n")
	// Linked by the note's path, which the import fixes, not by its id,
	// which it may suffix when another note holds the plain one.
	for i, c := range chapters {
		target := strings.TrimSuffix(path.Join(ImportedDir, src.Repo, src.Prefix, c.Rel), ".md")
		fmt.Fprintf(&body, "%d. [[%s|%s]]\n", i+1, target, strings.ReplaceAll(c.Title, "|", "-"))
	}
	text := body.String()
	return doc{
		Rel: path.Join(folder, "book.md"), Path: path.Join(folderPath, "book.md"), Repo: src.Repo,
		Source: book, Title: title, Body: text, Hash: hashOf(text),
	}
}
