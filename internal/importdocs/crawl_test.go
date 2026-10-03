package importdocs_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const docsHost = "https://docs.example.com"

// site is a fake web server for the crawler: each URL answers with a page
// or an error, and every request is recorded in order.
type site struct {
	pages    map[string]importdocs.Page
	errs     map[string]error
	requests []string
}

func newSite() *site {
	return &site{pages: map[string]importdocs.Page{}, errs: map[string]error{}}
}

// html adds an article page at u whose text names it, linking to links.
func (s *site) html(u string, links ...string) {
	var b strings.Builder
	fmt.Fprintf(&b, "<html><head><title>Page %s</title></head><body><nav>NAV-MENU</nav><article><h1>Page %s</h1>\n", u, u)
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&b, "<p>This page at %s explains one part of the guide in enough words for readability to keep it as the article body. Paragraph %d.</p>\n", u, i)
	}
	for _, l := range links {
		fmt.Fprintf(&b, "<p>See <a href=\"%s\">this related page</a> for more on the topic of the guide.</p>\n", l)
	}
	b.WriteString("</article></body></html>")
	s.pages[u] = importdocs.Page{FinalURL: u, ContentType: "text/html; charset=utf-8", Body: []byte(b.String())}
}

func (s *site) text(u, ctype, body string) {
	s.pages[u] = importdocs.Page{FinalURL: u, ContentType: ctype, Body: []byte(body)}
}

func (s *site) status(u string, code int) {
	s.errs[u] = &importdocs.StatusError{Code: code, Status: fmt.Sprintf("%d %s", code, http.StatusText(code))}
}

func (s *site) fetch(_ context.Context, u string) (importdocs.Page, error) {
	s.requests = append(s.requests, u)
	if err, ok := s.errs[u]; ok {
		return importdocs.Page{}, err
	}
	if p, ok := s.pages[u]; ok {
		return p, nil
	}
	return importdocs.Page{}, &importdocs.StatusError{Code: 404, Status: "404 Not Found"}
}

// pageRequests are the requests that were not for robots.txt, llms.txt or a
// sitemap.
func (s *site) pageRequests() []string {
	var out []string
	for _, u := range s.requests {
		if strings.HasSuffix(u, "/robots.txt") || strings.HasSuffix(u, "/llms.txt") || strings.HasSuffix(u, ".xml") {
			continue
		}
		out = append(out, u)
	}
	return out
}

func crawl(t *testing.T, s *site, start, vault string, opts importdocs.Options, co importdocs.CrawlOptions) *importdocs.Result {
	t.Helper()
	if co.MaxPages == 0 {
		co.MaxPages = 100
	}
	if co.Depth == 0 {
		co.Depth = 5
	}
	res, err := importdocs.Crawl(t.Context(), start, vault, opts, co, s.fetch)
	require.NoError(t, err)
	return res
}

func notesBy(res *importdocs.Result, a importdocs.Action) []string {
	var out []string
	for _, e := range res.Entries {
		if e.Action == a {
			out = append(out, e.Note)
		}
	}
	sort.Strings(out)
	return out
}

func webNote(name string) string {
	return "imported/web/docs.example.com/" + name + ".md"
}

func TestCrawl_FollowsLinksInScopeOnlyAndWritesOneNotePerPage(t *testing.T) {
	s := newSite()
	s.html(docsHost+"/guide/",
		"a", "/guide/b#install", "/guide/b?tab=linux", "/guide/a/", // one page each, however written
		"/blog/news", "https://other.example.org/guide/c", // outside the start's folder or host
		"/guide/logo.png", "mailto:docs@example.com", "javascript:void(0)") // never pages
	s.html(docsHost+"/guide/a", "/guide/", "c")
	s.html(docsHost + "/guide/b")
	s.html(docsHost + "/guide/c")
	vault := t.TempDir()

	res := crawl(t, s, docsHost+"/guide/", vault, importdocs.Options{}, importdocs.CrawlOptions{})

	assert.ElementsMatch(t, []string{webNote("guide"), webNote("guide-a"), webNote("guide-b"), webNote("guide-c")},
		notesBy(res, importdocs.Added))
	assert.Equal(t, []string{docsHost + "/guide/", docsHost + "/guide/a", docsHost + "/guide/b", docsHost + "/guide/c"},
		s.pageRequests(), "breadth first, each page once, nothing out of scope")
	fm, body, _ := noteFront(t, vault, webNote("guide-a"))
	assert.Equal(t, docsHost+"/guide/a", fm["url"])
	assert.NotContains(t, body, "NAV-MENU")

	again := crawl(t, newSiteLike(s), docsHost+"/guide/", vault, importdocs.Options{}, importdocs.CrawlOptions{})
	assert.Equal(t, 4, again.Count(importdocs.Unchanged))
	assert.Zero(t, again.Count(importdocs.Added)+again.Count(importdocs.Updated)+again.Count(importdocs.Orphaned))
}

// newSiteLike is s with a fresh request log.
func newSiteLike(s *site) *site {
	return &site{pages: s.pages, errs: s.errs}
}

func TestCrawl_SeedsFromLLMsTxtThenSitemapsBeforeFollowingLinks(t *testing.T) {
	s := newSite()
	s.text(docsHost+"/robots.txt", "text/plain", "User-agent: *\nDisallow: /guide/b\nSitemap: "+docsHost+"/index.xml\n")
	s.text(docsHost+"/llms.txt", "text/markdown", "# Docs\n\n- [A](https://docs.example.com/guide/a): the A page\n- [Blog](https://docs.example.com/blog/x)\n")
	s.text(docsHost+"/index.xml", "application/xml",
		`<?xml version="1.0"?><sitemapindex><sitemap><loc>`+docsHost+`/pages.xml</loc></sitemap></sitemapindex>`)
	s.text(docsHost+"/pages.xml", "text/xml",
		`<?xml version="1.0"?><urlset><url><loc>`+docsHost+`/guide/b</loc></url><url><loc>`+docsHost+`/guide/a</loc></url><url><loc>`+docsHost+`/blog/y</loc></url></urlset>`)
	s.html(docsHost+"/guide/", "c")
	s.html(docsHost + "/guide/a")
	s.html(docsHost + "/guide/b")
	s.html(docsHost + "/guide/c")

	res := crawl(t, s, docsHost+"/guide/", t.TempDir(), importdocs.Options{}, importdocs.CrawlOptions{})

	assert.Equal(t, []string{docsHost + "/guide/", docsHost + "/guide/a", docsHost + "/guide/b", docsHost + "/guide/c"},
		s.pageRequests(), "start, llms.txt, sitemap, then links; robots.txt is not obeyed by default")
	assert.Equal(t, 4, res.Count(importdocs.Added))
}

func TestCrawl_RespectRobotsSkipsDisallowedPages(t *testing.T) {
	s := newSite()
	s.text(docsHost+"/robots.txt", "text/plain", "User-agent: *\nDisallow: /guide/b\n")
	s.html(docsHost+"/guide/", "a", "b")
	s.html(docsHost + "/guide/a")
	s.html(docsHost + "/guide/b")

	res := crawl(t, s, docsHost+"/guide/", t.TempDir(), importdocs.Options{}, importdocs.CrawlOptions{RespectRobots: true})

	assert.Equal(t, []string{docsHost + "/guide/", docsHost + "/guide/a"}, s.pageRequests())
	assert.Equal(t, 2, res.Count(importdocs.Added))
	assert.Contains(t, skipReasons(res), "robots.txt disallows them, so they were not fetched ("+docsHost+"/guide/b)")
}

func TestCrawl_RobotsStatusFollowsRFC9309WhenRespected(t *testing.T) {
	t.Run("4xx allows everything", func(t *testing.T) {
		s := newSite()
		s.status(docsHost+"/robots.txt", 403)
		s.html(docsHost+"/guide/", "a")
		s.html(docsHost + "/guide/a")
		res := crawl(t, s, docsHost+"/guide/", t.TempDir(), importdocs.Options{}, importdocs.CrawlOptions{RespectRobots: true})
		assert.Equal(t, 2, res.Count(importdocs.Added))
	})
	t.Run("5xx disallows everything", func(t *testing.T) {
		s := newSite()
		s.status(docsHost+"/robots.txt", 503)
		s.html(docsHost + "/guide/")
		_, err := importdocs.Crawl(t.Context(), docsHost+"/guide/", t.TempDir(), importdocs.Options{},
			importdocs.CrawlOptions{MaxPages: 10, Depth: 5, RespectRobots: true}, s.fetch)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "robots.txt disallows")
		assert.Empty(t, s.pageRequests())
	})
}

func skipReasons(res *importdocs.Result) string {
	var b strings.Builder
	for _, e := range res.Entries {
		if e.Action == importdocs.Skipped {
			b.WriteString(e.Note + " — " + e.Reason + "\n")
		}
	}
	return b.String()
}

func TestCrawl_PageAndDepthCapsStopTheCrawlAndSaySo(t *testing.T) {
	s := newSite()
	s.html(docsHost+"/guide/", "a", "b", "c")
	s.html(docsHost+"/guide/a", "deep")
	s.html(docsHost + "/guide/b")
	s.html(docsHost + "/guide/c")
	s.html(docsHost + "/guide/deep")

	res := crawl(t, s, docsHost+"/guide/", t.TempDir(), importdocs.Options{}, importdocs.CrawlOptions{MaxPages: 2})
	assert.Equal(t, []string{docsHost + "/guide/", docsHost + "/guide/a"}, s.pageRequests())
	assert.Contains(t, skipReasons(res), "3 page(s) — over --max-pages 2; raise it to fetch them")

	s2 := newSiteLike(s)
	res = crawl(t, s2, docsHost+"/guide/", t.TempDir(), importdocs.Options{}, importdocs.CrawlOptions{Depth: 1})
	assert.NotContains(t, s2.pageRequests(), docsHost+"/guide/deep", "two links from the start")
	assert.Equal(t, 4, res.Count(importdocs.Added))
}

func TestCrawl_IncludeAndExcludeNarrowTheScope(t *testing.T) {
	s := newSite()
	s.html(docsHost+"/", "/guide/a", "/api/x", "/guide/old/b")
	s.html(docsHost + "/guide/a")
	s.html(docsHost + "/api/x")
	s.html(docsHost + "/guide/old/b")

	crawl(t, s, docsHost+"/", t.TempDir(), importdocs.Options{},
		importdocs.CrawlOptions{Include: []string{"guide/**"}, Exclude: []string{"/guide/old/"}})
	assert.Equal(t, []string{docsHost + "/", docsHost + "/guide/a"}, s.pageRequests())
}

func TestCrawl_ABadPageIsSkippedAndTheCrawlGoesOn(t *testing.T) {
	s := newSite()
	s.html(docsHost+"/guide/", "a", "broken", "zip", "moved", "dup", "shell")
	s.html(docsHost + "/guide/a")
	s.status(docsHost+"/guide/broken", 500)
	s.text(docsHost+"/guide/zip", "application/zip", "PK")
	s.pages[docsHost+"/guide/moved"] = importdocs.Page{FinalURL: "https://elsewhere.example.net/x", ContentType: "text/html",
		Body: s.pages[docsHost+"/guide/a"].Body}
	s.pages[docsHost+"/guide/dup"] = importdocs.Page{FinalURL: docsHost + "/guide/a", ContentType: "text/html",
		Body: s.pages[docsHost+"/guide/a"].Body}
	s.text(docsHost+"/guide/shell", "text/html", `<html><body><div id="root"></div><script src="app.js"></script></body></html>`)

	res := crawl(t, s, docsHost+"/guide/", t.TempDir(), importdocs.Options{}, importdocs.CrawlOptions{})

	assert.ElementsMatch(t, []string{webNote("guide"), webNote("guide-a")}, notesBy(res, importdocs.Added))
	reasons := skipReasons(res)
	for _, want := range []string{
		docsHost + "/guide/broken — page returned 500 Internal Server Error",
		docsHost + "/guide/zip — not a text page (application/zip)",
		docsHost + "/guide/moved — redirected off the site to https://elsewhere.example.net/x",
		docsHost + "/guide/dup — the same page as " + docsHost + "/guide/a",
		docsHost + "/guide/shell — no text in the HTML (a page that needs JavaScript?)",
		"orphans were not checked: a page failed, so the crawl did not see the whole site",
	} {
		assert.Contains(t, reasons, want)
	}
}

func TestCrawl_AFailedStartPageIsAnErrorAndWritesNothing(t *testing.T) {
	s := newSite()
	s.status(docsHost+"/guide/", 503)
	vault := t.TempDir()
	_, err := importdocs.Crawl(t.Context(), docsHost+"/guide/", vault, importdocs.Options{},
		importdocs.CrawlOptions{MaxPages: 10, Depth: 5}, s.fetch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
	assert.NoDirExists(t, filepath.Join(vault, "imported"))
}

func TestCrawl_APageGoneFromACompleteCrawlIsAnOrphanPrunedOnlyWhenAsked(t *testing.T) {
	s := newSite()
	s.html(docsHost+"/guide/", "a", "b")
	s.html(docsHost + "/guide/a")
	s.html(docsHost + "/guide/b")
	vault := t.TempDir()
	crawl(t, s, docsHost+"/guide/", vault, importdocs.Options{}, importdocs.CrawlOptions{})
	// A page imported on its own, outside the crawl's folder, is not the crawl's to judge.
	importPage(t, docsHost+"/blog/news", vault, importdocs.Options{}, pageFetcher(s.pages[docsHost+"/guide/a"]))

	s.html(docsHost+"/guide/", "a")
	delete(s.pages, docsHost+"/guide/b")
	res := crawl(t, newSiteLike(s), docsHost+"/guide/", vault, importdocs.Options{}, importdocs.CrawlOptions{})
	assert.Equal(t, []string{webNote("guide-b")}, notesBy(res, importdocs.Orphaned))
	assert.FileExists(t, filepath.Join(vault, filepath.FromSlash(webNote("guide-b"))))

	// An incomplete crawl judges nothing.
	res = crawl(t, newSiteLike(s), docsHost+"/guide/", vault, importdocs.Options{Prune: true}, importdocs.CrawlOptions{MaxPages: 1})
	assert.Empty(t, notesBy(res, importdocs.Orphaned))
	assert.Empty(t, notesBy(res, importdocs.Pruned))

	res = crawl(t, newSiteLike(s), docsHost+"/guide/", vault, importdocs.Options{Prune: true}, importdocs.CrawlOptions{})
	assert.Equal(t, []string{webNote("guide-b")}, notesBy(res, importdocs.Pruned))
	assert.NoFileExists(t, filepath.Join(vault, filepath.FromSlash(webNote("guide-b"))))
	assert.FileExists(t, filepath.Join(vault, filepath.FromSlash(webNote("blog-news"))))
}

func TestCrawl_A404FromTheSitemapIsGoneNotAFailure(t *testing.T) {
	s := newSite()
	s.text(docsHost+"/sitemap.xml", "application/xml",
		`<urlset><url><loc>`+docsHost+`/guide/a</loc></url><url><loc>`+docsHost+`/guide/removed</loc></url></urlset>`)
	s.html(docsHost + "/guide/")
	s.html(docsHost + "/guide/a")

	res := crawl(t, s, docsHost+"/guide/", t.TempDir(), importdocs.Options{}, importdocs.CrawlOptions{})
	assert.Equal(t, 2, res.Count(importdocs.Added))
	assert.Contains(t, skipReasons(res), docsHost+"/guide/removed — gone (404 Not Found)")
	assert.NotContains(t, skipReasons(res), "orphans were not checked")
}

func TestCrawl_StartScopeIsTheStartsFolder(t *testing.T) {
	cases := map[string][]string{
		docsHost + "/docs":            {docsHost + "/docs", docsHost + "/docs/a"},
		docsHost + "/docs/intro.html": {docsHost + "/docs/intro.html", docsHost + "/docs/a"},
	}
	var errs []error
	for start, want := range cases {
		s := newSite()
		s.html(start, "/docs/a", "/other/b")
		s.html(docsHost + "/docs/a")
		s.html(docsHost + "/other/b")
		crawl(t, s, start, t.TempDir(), importdocs.Options{}, importdocs.CrawlOptions{})
		if got := s.pageRequests(); fmt.Sprint(got) != fmt.Sprint(want) {
			errs = append(errs, fmt.Errorf("start %s fetched %v, want %v", start, got, want))
		}
	}
	assert.NoError(t, errors.Join(errs...))
}

// A sitemap can list 50,000 URLs. The crawl queues at most ten times
// --max-pages of them and reports the rest as over the cap.
func TestCrawl_AHugeSitemapQueuesABoundedNumberOfPages(t *testing.T) {
	s := newSite()
	var b strings.Builder
	b.WriteString("<urlset>")
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&b, "<url><loc>%s/guide/p%d</loc></url>", docsHost, i)
	}
	b.WriteString("</urlset>")
	s.text(docsHost+"/sitemap.xml", "application/xml", b.String())
	s.html(docsHost + "/guide/")

	res := crawl(t, s, docsHost+"/guide/", t.TempDir(), importdocs.Options{}, importdocs.CrawlOptions{MaxPages: 3})
	assert.Len(t, s.pageRequests(), 3)
	// 501 pages (the start and 500 listed): 3 fetched, 27 queued and left,
	// 471 counted but never held.
	assert.Contains(t, skipReasons(res), "498 page(s) — over --max-pages 3")
}
