package importdocs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

// CrawlOptions bound a crawl. See Crawl.
type CrawlOptions struct {
	// MaxPages caps the page requests. Discovery reads (robots.txt,
	// llms.txt, sitemaps) are not pages.
	MaxPages int
	// Depth caps link hops from the start: 0 is the start page alone.
	// Pages listed in llms.txt or a sitemap are one hop away.
	Depth int
	// Include and Exclude are `paths:` globs on the URL path. A link must
	// match an Include (when there are any) and no Exclude. The start page
	// is always fetched.
	Include, Exclude []string
	// RespectRobots obeys robots.txt: its Disallow rules and a Crawl-delay
	// above Delay. Off by default; robots.txt is still read for sitemaps.
	RespectRobots bool
	// Delay is the pause between requests.
	Delay time.Duration
}

// maxCrawlDelay caps a robots.txt Crawl-delay.
const maxCrawlDelay = 30 * time.Second

// Crawl imports startURL and the pages it leads to as notes, one per page,
// each the note a single-page import of that URL would write. Pages come
// from llms.txt, the sitemaps, then links, breadth first, within the start
// URL's host and folder. A page that fails is skipped and the crawl goes
// on; the start page failing is an error and nothing is written.
func Crawl(ctx context.Context, startURL, vaultRoot string, opts Options, co CrawlOptions, fetch Fetcher) (*Result, error) {
	if fetch == nil {
		return nil, errors.New("no page fetcher")
	}
	c, err := newCrawler(ctx, startURL, co, fetch)
	if err != nil {
		return nil, err
	}
	if err := c.discover(); err != nil {
		return nil, err
	}
	if err := c.run(); err != nil {
		return nil, err
	}
	return c.write(vaultRoot, opts)
}

// target is a page waiting to be fetched.
type target struct {
	url   string
	depth int
}

// crawler is one crawl's state.
type crawler struct {
	ctx   context.Context
	fetch Fetcher
	co    CrawlOptions
	start *url.URL
	host  string
	scope string // the start's folder, no trailing slash: "" is the whole host
	robot *robots
	delay time.Duration

	queue    []target
	seen     map[string]bool   // keys of every URL queued
	finals   map[string]string // key of each fetched page's final URL → the URL asked for
	requests int
	pages    int
	docs     []doc
	entries  []Entry

	overCap, disallowed []string
	failed, depthCut    bool
}

func newCrawler(ctx context.Context, startURL string, co CrawlOptions, fetch Fetcher) (*crawler, error) {
	host, _, _, err := pagePlace(startURL)
	if err != nil {
		return nil, err
	}
	start, ok := normalizeURL(startURL, nil, true)
	if !ok {
		return nil, fmt.Errorf("%s is not an http or https URL", startURL)
	}
	c := &crawler{ctx: ctx, fetch: fetch, co: co, start: start, host: host,
		scope: startScope(start.Path), delay: co.Delay,
		seen: map[string]bool{}, finals: map[string]string{}}
	c.enqueue(start.String(), 0)
	return c, nil
}

// startScope is the folder a crawl stays in. A path ending in / is that
// folder; a file (intro.html) is its folder; an extensionless name (/docs)
// is a folder too, as doc sites name their roots.
func startScope(p string) string {
	switch {
	case strings.HasSuffix(p, "/"):
		return strings.TrimSuffix(p, "/")
	case path.Ext(path.Base(p)) != "":
		return strings.TrimSuffix(path.Dir(p), "/")
	default:
		return p
	}
}

// inScope reports a URL on the start's host and inside its folder, matched
// by the include and exclude globs.
func (c *crawler) inScope(u *url.URL) bool {
	if u.Host != c.start.Host {
		return false
	}
	if u.Path != c.scope && !strings.HasPrefix(u.Path, c.scope+"/") {
		return false
	}
	return globsAllow(strings.TrimPrefix(u.Path, "/"), c.co.Include, c.co.Exclude)
}

func (c *crawler) enqueue(raw string, depth int) {
	u, ok := normalizeURL(raw, nil, depth == 0)
	if !ok || c.seen[urlKey(u)] {
		return
	}
	if depth > 0 && (!c.inScope(u) || isAsset(u.Path)) {
		return
	}
	if depth > c.co.Depth {
		c.depthCut = true
		return
	}
	c.seen[urlKey(u)] = true
	c.queue = append(c.queue, target{url: u.String(), depth: depth})
}

// run fetches the queue in order until it is empty or the page cap is hit.
func (c *crawler) run() error {
	for len(c.queue) > 0 {
		if c.pages >= c.co.MaxPages {
			for _, t := range c.queue {
				c.overCap = append(c.overCap, t.url)
			}
			return nil
		}
		t := c.queue[0]
		c.queue = c.queue[1:]
		if err := c.visit(t); err != nil {
			return err
		}
	}
	return nil
}

// visit fetches one page and keeps it, or records why not. Only the start
// page's failure stops the crawl.
func (c *crawler) visit(t target) error {
	if !c.robot.allows(t.url) {
		if t.depth == 0 {
			return fmt.Errorf("robots.txt disallows %s (crawl without --respect-robots to fetch it)", t.url)
		}
		c.disallowed = append(c.disallowed, t.url)
		return nil
	}
	page, err := c.get(t.url)
	c.pages++
	if err != nil {
		if t.depth == 0 {
			return err
		}
		c.skipFailed(t.url, err)
		return nil
	}
	if reason := c.keep(t, page); reason != "" {
		if t.depth == 0 {
			return fmt.Errorf("%s: %s", t.url, reason)
		}
		c.entries = append(c.entries, Entry{Action: Skipped, Note: t.url, Source: t.url, Reason: reason})
	}
	return nil
}

// skipFailed records a page that could not be fetched. A page that is gone
// (404, 410) is a fact about the site; anything else means the crawl did
// not see the whole site.
func (c *crawler) skipFailed(u string, err error) {
	var se *StatusError
	if errors.As(err, &se) && (se.Code == 404 || se.Code == 410) {
		c.entries = append(c.entries, Entry{Action: Skipped, Note: u, Source: u, Reason: "gone (" + se.Status + ")"})
		return
	}
	c.failed = true
	c.entries = append(c.entries, Entry{Action: Skipped, Note: u, Source: u, Reason: err.Error()})
}

// get waits its turn and fetches u.
func (c *crawler) get(u string) (Page, error) {
	if c.requests > 0 {
		if err := sleep(c.ctx, c.delay); err != nil {
			return Page{}, err
		}
	}
	c.requests++
	return c.fetch(c.ctx, u)
}

// sleep pauses for d, or until ctx is done. A test replaces it.
var sleep = func(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// keep turns a fetched page into a doc and queues its links. It returns why
// the page was not kept, or "".
func (c *crawler) keep(t target, page Page) string {
	final, ok := normalizeURL(page.FinalURL, nil, true)
	if !ok || final.Host != c.start.Host {
		return "redirected off the site to " + page.FinalURL
	}
	if first, dup := c.finals[urlKey(final)]; dup {
		return "the same page as " + first
	}
	c.finals[urlKey(final)] = t.url
	media, err := acceptedMedia(page.ContentType)
	if err != nil {
		return err.Error()
	}
	d, err := c.pageDoc(t.url, media, page)
	if err != nil {
		return err.Error()
	}
	if strings.TrimSpace(d.Body) == "" {
		return "no text in the HTML (a page that needs JavaScript?)"
	}
	c.docs = append(c.docs, d)
	if media == "text/html" || media == "application/xhtml+xml" {
		for _, l := range linksIn(page.Body, final) {
			c.enqueue(l, t.depth+1)
		}
	}
	return ""
}

func (c *crawler) pageDoc(rawURL, media string, page Page) (doc, error) {
	host, name, u, err := pagePlace(rawURL)
	if err != nil {
		return doc{}, err
	}
	return docFromPage(c.ctx, rawURL, u, host, name, media, page)
}

// incomplete says why the crawl did not see the whole site, or "".
func (c *crawler) incomplete() string {
	switch {
	case len(c.overCap) > 0:
		return "the page cap stopped the crawl"
	case c.failed:
		return "a page failed, so the crawl did not see the whole site"
	case len(c.disallowed) > 0:
		return "robots.txt kept pages out"
	case c.depthCut:
		return "the depth cap stopped the crawl"
	}
	return ""
}

// write runs every kept page through one syncer, then judges orphans when
// the crawl saw the whole site.
func (c *crawler) write(vaultRoot string, opts Options) (*Result, error) {
	vaultAbs, _, err := absVault(vaultRoot)
	if err != nil {
		return nil, err
	}
	base := path.Join(ImportedDir, "web", c.host)
	all, noteSkips, err := readNotes(vaultAbs, ImportedDir, base)
	if err != nil {
		return nil, err
	}
	s := newSyncer(Source{Repo: "web"}, "", base, all, opts, writer{vaultRoot: vaultAbs, dryRun: opts.DryRun})
	res := &Result{DryRun: opts.DryRun, Entries: append(under(noteSkips, base), c.summaryEntries()...)}
	res.Entries = append(res.Entries, s.syncDocs(c.docs)...)
	if why := c.incomplete(); why != "" {
		res.Entries = append(res.Entries, Entry{Action: Skipped, Note: c.start.String(), Reason: "orphans were not checked: " + why})
	} else {
		res.Entries = append(res.Entries, s.crawlOrphans(c.sourceInScope)...)
	}
	sort.SliceStable(res.Entries, func(i, j int) bool { return res.Entries[i].Note < res.Entries[j].Note })
	return res, nil
}

// summaryEntries are the per-page skips plus one line each for the pages
// over the cap and the pages robots.txt kept out.
func (c *crawler) summaryEntries() []Entry {
	out := append([]Entry(nil), c.entries...)
	if len(c.overCap) > 0 {
		out = append(out, countEntry(c.overCap, fmt.Sprintf("over --max-pages %d; raise it to fetch them", c.co.MaxPages)))
	}
	if len(c.disallowed) > 0 {
		out = append(out, countEntry(c.disallowed, "robots.txt disallows them, so they were not fetched"))
	}
	return out
}

// countEntry reports many URLs as one line, naming the first few.
func countEntry(urls []string, why string) Entry {
	const shown = 3
	names := strings.Join(urls[:min(len(urls), shown)], ", ")
	if len(urls) > shown {
		names += ", …"
	}
	return Entry{Action: Skipped, Note: fmt.Sprintf("%d page(s)", len(urls)), Reason: why + " (" + names + ")"}
}

// sourceInScope reports a note's source URL inside this crawl's scope.
func (c *crawler) sourceInScope(source string) bool {
	u, ok := normalizeURL(source, nil, true)
	return ok && c.inScope(u)
}

// crawlOrphans reports this host's notes that the crawl's scope covers and
// the crawl did not produce, and removes them under --prune.
func (s *syncer) crawlOrphans(inScope func(string) bool) []Entry {
	var out []Entry
	for rel, n := range s.notes {
		if s.claimed[rel] || !n.managed || !inScope(n.source) {
			continue
		}
		e := Entry{Action: Orphaned, Note: rel, Source: n.source, Reason: "the crawl no longer finds its page; --prune removes it"}
		if s.opts.Prune {
			e = s.prune(rel, n, e)
		}
		out = append(out, e)
	}
	return out
}
