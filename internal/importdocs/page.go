package importdocs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"codeberg.org/readeck/go-readability/v2"
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/peiman/vaultmind/internal/vault"
	xhtml "golang.org/x/net/html"
)

// ImportURL imports the page at rawURL as one note. fetch retrieves it;
// HTTPFetcher is what the command passes, and a test passes its own.
// The note is planned and written by the same syncer as a folder import,
// so hash re-sync, conflicts, excludes, and unowned files behave the same.
// Orphans are not considered: this import has one page, and notes from
// other pages are not its to remove.
func ImportURL(ctx context.Context, rawURL, vaultRoot string, opts Options, fetch Fetcher) (*Result, error) {
	host, name, u, err := pagePlace(rawURL)
	if err != nil {
		return nil, err
	}
	if fetch == nil {
		return nil, errors.New("no page fetcher")
	}
	page, err := fetch(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	media, err := acceptedMedia(page.ContentType)
	if err != nil {
		return nil, err
	}
	d, err := docFromPage(ctx, rawURL, u, host, name, media, page)
	if err != nil {
		return nil, err
	}
	return writePage(vaultRoot, host, opts, d)
}

// pagePlace names the vault folder and file for rawURL. The host and the
// file name are single path segments: a separator or a control character
// would write outside imported/web or break the frontmatter.
func pagePlace(rawURL string) (host, name string, u *url.URL, err error) {
	u, err = url.Parse(rawURL)
	if err != nil {
		return "", "", nil, fmt.Errorf("parsing %s: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", nil, errors.New("only http and https URLs can be imported")
	}
	if host, err = pageHost(u); err != nil {
		return "", "", nil, err
	}
	if name, err = pageName(u); err != nil {
		return "", "", nil, err
	}
	return host, name, u, nil
}

func pageHost(u *url.URL) (string, error) {
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", errors.New("page URL has no host")
	}
	if port := u.Port(); port != "" && port != defaultPort(u.Scheme) {
		host += "-" + port
	}
	if !plainSegment(host) || slug(host) == "" {
		return "", fmt.Errorf("page host %q is not a single folder name", host)
	}
	return host, nil
}

func defaultPort(scheme string) string {
	switch scheme {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func pageName(u *url.URL) (string, error) {
	name := slugPath(u.Path)
	if q := slug(u.RawQuery); q != "" {
		name += "-" + q
	}
	if !plainSegment(name) {
		return "", fmt.Errorf("page name %q is not a single file name", name)
	}
	return name, nil
}

// slugPath is the URL path as one slug. "/" is index. A trailing
// .html, .htm, or .md is dropped before slugging, so fts5.html is fts5.
func slugPath(p string) string {
	if p == "" || p == "/" {
		return "index"
	}
	p = strings.Trim(p, "/")
	if ext := strings.ToLower(path.Ext(p)); ext == ".html" || ext == ".htm" || ext == ".md" {
		p = strings.TrimSuffix(p, path.Ext(p))
	}
	if name := slug(p); name != "" {
		return name
	}
	return "index"
}

func plainSegment(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\`) && !hasControl(s)
}

func docFromPage(ctx context.Context, rawURL string, u *url.URL, host, name, media string, page Page) (doc, error) {
	base := page.FinalURL
	if base == "" {
		base = rawURL
	}
	body, title, err := pageMarkdown(ctx, media, page.Body, base)
	if err != nil {
		return doc{}, err
	}
	if title == "" {
		title = markdownTitle(body, u)
	}
	return doc{
		Rel:    name + vault.NoteExtension,
		Path:   slug(host) + "-" + name,
		Repo:   "web",
		Source: rawURL,
		URL:    rawURL,
		Title:  title,
		Body:   body,
		Hash:   hashOf(body),
	}, nil
}

func markdownTitle(body string, u *url.URL) string {
	if h, ok := headingTitle(body); ok && h != "" {
		return h
	}
	return fallbackTitle(u)
}

// fallbackTitle is the page's host and path when the page has no heading.
// The host is lowercased and the port is left off; an empty path is "/".
func fallbackTitle(u *url.URL) string {
	p := u.Path
	if p == "" {
		p = "/"
	}
	return strings.ToLower(u.Hostname()) + p
}

func pageMarkdown(ctx context.Context, media string, body []byte, finalURL string) (string, string, error) {
	switch media {
	case pdfMediaType:
		return pdfText(ctx, body)
	case "text/html", "application/xhtml+xml":
		return safeHTMLMarkdown(string(body), finalURL)
	default:
		return servedMarkdown(body)
	}
}

// servedMarkdown is a text page. A leading frontmatter block is not the
// note's body: the same split a folder import uses for a doc, and its title
// when it has one. A heading is the fallback, applied by the caller.
func servedMarkdown(raw []byte) (string, string, error) {
	fm, body := splitFrontmatter(raw)
	return normalizeMarkdown(body), frontmatterTitle(fm), nil
}

// htmlConverter turns HTML into markdown and a title. A test replaces it
// to force a panic in the conversion step.
var htmlConverter = htmlMarkdown

// safeHTMLMarkdown turns a panic in readability or html-to-markdown into an
// error. Either library can panic on a page; the import should refuse it.
func safeHTMLMarkdown(html, finalURL string) (md, title string, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			md, title, err = "", "", fmt.Errorf("converting the page: %v", rec)
		}
	}()
	return htmlConverter(html, finalURL)
}

// htmlMarkdown keeps the article readability extracts, or the whole page
// when that extraction fails or is empty. The title is readability's when
// it has one. The markdown is what the note stores, so its hash is the
// source hash: hashing the raw HTML would make every page look hand-edited.
func htmlMarkdown(html, finalURL string) (string, string, error) {
	fragment := html
	title := ""
	article, err := readability.FromReader(strings.NewReader(html), pageURL(finalURL))
	// A nil node is an empty extraction. Keep the whole page, as when parsing fails.
	if err == nil && article.Node != nil {
		var b strings.Builder
		if renderErr := article.RenderHTML(&b); renderErr == nil && strings.TrimSpace(b.String()) != "" {
			fragment = chooseFragment(b.String(), html)
			title = strings.TrimSpace(article.Title())
		}
	}
	md, err := htmltomarkdown.ConvertString(fragment)
	if err != nil {
		return "", "", err
	}
	return normalizeMarkdown(dropLeadingLinkList(md)), title, nil
}

// chooseFragment keeps readability's pick unless the page's <main> holds at
// least twice its text: then readability chose a navbar dropdown or a footer
// (doc sites' category and short pages), and <main> is the content.
//
// It also yields to <main> when its pick is mostly link text and <main> is
// not: on a short page the sidebar outweighs the article, and readability
// takes the sidebar (Sphinx's one-paragraph extension pages).
func chooseFragment(picked, page string) string {
	main := mainElement(page)
	if main == "" {
		return picked
	}
	p, m := textStats(picked), textStats(main)
	if 2*p.total < m.total || (p.mostlyLinks() && !m.mostlyLinks()) {
		return main
	}
	return picked
}

// mainElement is the HTML of the page's first <main> (or role="main")
// element, or "" when it has none.
func mainElement(page string) string {
	doc, err := xhtml.Parse(strings.NewReader(page))
	if err != nil {
		return ""
	}
	n := findMain(doc)
	if n == nil {
		return ""
	}
	var b strings.Builder
	if xhtml.Render(&b, n) != nil {
		return ""
	}
	return b.String()
}

func findMain(n *xhtml.Node) *xhtml.Node {
	if n.Type == xhtml.ElementNode && (n.Data == "main" || hasAttr(n, "role", "main")) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if m := findMain(c); m != nil {
			return m
		}
	}
	return nil
}

func hasAttr(n *xhtml.Node, key, val string) bool {
	for _, a := range n.Attr {
		if a.Key == key && a.Val == val {
			return true
		}
	}
	return false
}

// fragmentText is how much text an HTML fragment holds, and how much of it
// is inside links, in non-space characters.
type fragmentText struct {
	total, inLinks int
}

func (f fragmentText) mostlyLinks() bool {
	return f.total > 0 && 2*f.inLinks > f.total
}

// textStats measures a fragment's text, leaving out scripts and styles.
func textStats(fragment string) fragmentText {
	z := xhtml.NewTokenizer(strings.NewReader(fragment))
	var f fragmentText
	skip, links := false, 0
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			return f
		case xhtml.StartTagToken:
			name, _ := z.TagName()
			skip = string(name) == "script" || string(name) == "style"
			if string(name) == "a" {
				links++
			}
		case xhtml.EndTagToken:
			skip = false
			if name, _ := z.TagName(); string(name) == "a" && links > 0 {
				links--
			}
		case xhtml.TextToken:
			if skip {
				continue
			}
			n := len(strings.Join(strings.Fields(string(z.Text())), ""))
			f.total += n
			if links > 0 {
				f.inLinks += n
			}
		}
	}
}

// pageURL is the base readability uses for relative links. It is never nil:
// a nil base panics inside the parser.
func pageURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Scheme == "" || u.Host == "" {
		return &url.URL{Scheme: "https", Host: "localhost", Path: "/"}
	}
	return u
}

// A breadcrumb item is one list marker and one inline link, nothing else.
// Readability keeps that list at the top of pages such as go.dev's docs.
// A bare link line counts too: Docusaurus's version dropdown opens with the
// current version as one, and MkDocs Material puts its edit and view-source
// icon links side by side on one.
var linkOnlyListItem = regexp.MustCompile(`^\s*(?:(?:\d+\.|[-*+])\s+)?(?:\[[^]]*\]\([^)]*\)\s*)+$`)

// navFiller are the lines a navigation list holds besides its links: a
// separator item, a bold label item, and html-to-markdown's marker between
// two lists. They are dropped only among link items, never on their own.
var navFiller = regexp.MustCompile(`^\s*(?:[-*+]\s+(?:\* \* \*|\*\*[^*]+\*\*)|<!--THE END-->)\s*$`)

// dropLeadingLinkList removes a leading breadcrumb or version list
// readability kept. Excerpts and the recall hook show a note's opening
// lines, so those lines have to be content. Blank lines among the items go
// with them. A page that is only such a list keeps it, and a list after
// other text stays.
func dropLeadingLinkList(s string) string {
	lines := strings.Split(s, "\n")
	end, saw := leadingNavEnd(lines)
	if !saw {
		return s
	}
	rest := strings.Join(lines[end:], "\n")
	if strings.TrimSpace(rest) == "" {
		return s
	}
	return rest
}

// leadingNavEnd is the index of the first line after the leading navigation
// block, and whether that block held a link at all.
func leadingNavEnd(lines []string) (int, bool) {
	saw := false
	for i, line := range lines {
		switch {
		case strings.TrimSpace(line) == "":
		case linkOnlyListItem.MatchString(line):
			saw = true
		case saw && navFiller.MatchString(line):
		default:
			return i, saw
		}
	}
	return len(lines), saw
}

func normalizeMarkdown(s string) string {
	return strings.TrimSpace(s) + "\n"
}

// writePage runs one page through the folder import's syncer and stops
// before the orphan pass.
func writePage(vaultRoot, host string, opts Options, d doc) (*Result, error) {
	vaultAbs, _, err := absVault(vaultRoot)
	if err != nil {
		return nil, err
	}
	base := path.Join(ImportedDir, "web", host)
	all, noteSkips, err := readNotes(vaultAbs, ImportedDir, base)
	if err != nil {
		return nil, err
	}
	s := newSyncer(Source{Repo: "web"}, "", base, all, opts, writer{vaultRoot: vaultAbs, dryRun: opts.DryRun})
	res := &Result{DryRun: opts.DryRun, Entries: under(noteSkips, base)}
	res.Entries = append(res.Entries, s.syncDocs([]doc{d})...)
	sort.SliceStable(res.Entries, func(i, j int) bool { return res.Entries[i].Note < res.Entries[j].Note })
	return res, nil
}
