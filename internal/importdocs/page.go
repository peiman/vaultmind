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
	d, err := docFromPage(rawURL, u, host, name, media, page)
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

func docFromPage(rawURL string, u *url.URL, host, name, media string, page Page) (doc, error) {
	base := page.FinalURL
	if base == "" {
		base = rawURL
	}
	body, title, err := pageMarkdown(media, page.Body, base)
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

func pageMarkdown(media string, body []byte, finalURL string) (string, string, error) {
	if media != "text/html" && media != "application/xhtml+xml" {
		return servedMarkdown(body)
	}
	return safeHTMLMarkdown(string(body), finalURL)
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
			fragment = b.String()
			title = strings.TrimSpace(article.Title())
		}
	}
	md, err := htmltomarkdown.ConvertString(fragment)
	if err != nil {
		return "", "", err
	}
	return normalizeMarkdown(dropLeadingLinkList(md)), title, nil
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
var linkOnlyListItem = regexp.MustCompile(`^\s*(?:\d+\.|[-*+])\s+\[[^]]*\]\([^)]*\)\s*$`)

// dropLeadingLinkList removes a leading breadcrumb readability kept.
// Excerpts and the recall hook show a note's opening lines, so those lines
// have to be content. Blank lines among the items go with them. A page that
// is only such a list keeps it, and a list after other text stays.
func dropLeadingLinkList(s string) string {
	lines := strings.Split(s, "\n")
	end := 0
	saw := false
	for end < len(lines) {
		if strings.TrimSpace(lines[end]) == "" {
			end++
			continue
		}
		if !linkOnlyListItem.MatchString(lines[end]) {
			break
		}
		saw = true
		end++
	}
	if !saw {
		return s
	}
	rest := strings.Join(lines[end:], "\n")
	if strings.TrimSpace(rest) == "" {
		return s
	}
	return rest
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
