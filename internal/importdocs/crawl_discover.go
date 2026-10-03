package importdocs

import (
	"bytes"
	"encoding/xml"
	"errors"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/peiman/vaultmind/internal/pathglob"
	"github.com/temoto/robotstxt"
	"golang.org/x/net/html"
)

// robotsAgent is the name robots.txt groups are matched against.
const robotsAgent = "vaultmind"

// maxSubSitemaps caps the sitemaps one sitemap index leads to.
const maxSubSitemaps = 20

// robots is robots.txt when the crawl obeys it; nil allows everything.
type robots struct {
	data *robotstxt.RobotsData
}

func (r *robots) allows(raw string) bool {
	if r == nil {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	p := u.EscapedPath()
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	return r.data.TestAgent(p, robotsAgent)
}

// discover reads robots.txt, llms.txt and the sitemaps, and queues the
// pages they list after the start page.
func (c *crawler) discover() error {
	sitemaps, err := c.readRobots()
	if err != nil {
		return err
	}
	for _, l := range c.llmsLinks() {
		c.enqueue(l, 1)
	}
	if len(sitemaps) == 0 {
		sitemaps = []string{c.origin() + "/sitemap.xml"}
	}
	for _, sm := range sitemaps {
		c.readSitemap(sm, true)
	}
	return nil
}

func (c *crawler) origin() string {
	return c.start.Scheme + "://" + c.start.Host
}

// readRobots returns robots.txt's sitemaps. Under --respect-robots it also
// keeps its rules: per RFC 9309 a 4xx allows everything, and a 5xx or an
// unreachable robots.txt disallows everything. A Crawl-delay above the
// configured delay is used, up to maxCrawlDelay.
func (c *crawler) readRobots() ([]string, error) {
	page, err := c.get(c.origin() + "/robots.txt")
	code, body := 200, page.Body
	if err != nil {
		code, body = 500, nil
		var se *StatusError
		if errors.As(err, &se) {
			code = se.Code
		}
	}
	data, perr := robotstxt.FromStatusAndBytes(code, body)
	if perr != nil {
		data, _ = robotstxt.FromStatusAndBytes(500, nil)
	}
	if c.co.RespectRobots {
		c.robot = &robots{data: data}
		if d := data.FindGroup(robotsAgent).CrawlDelay; d > c.delay {
			c.delay = min(d, maxCrawlDelay)
		}
	}
	return data.Sitemaps, nil
}

// markdownLink is the target of a markdown link.
var markdownLink = regexp.MustCompile(`\]\(\s*<?([^)\s>]+)`)

// llmsLinks are the links in the site's llms.txt, resolved against it.
func (c *crawler) llmsLinks() []string {
	at := c.origin() + "/llms.txt"
	page, err := c.get(at)
	if err != nil {
		return nil
	}
	base, _ := url.Parse(at)
	var out []string
	for _, m := range markdownLink.FindAllStringSubmatch(string(page.Body), -1) {
		if u, ok := normalizeURL(m[1], base, false); ok {
			out = append(out, u.String())
		}
	}
	return out
}

// sitemap is a urlset or a sitemap index.
type sitemap struct {
	URLs     []sitemapLoc `xml:"url"`
	Sitemaps []sitemapLoc `xml:"sitemap"`
}

type sitemapLoc struct {
	Loc string `xml:"loc"`
}

// readSitemap queues the page URLs a sitemap lists, following one level of
// sitemap index when follow is set. Each sitemap is queued as it is read,
// so memory holds one sitemap at a time. Only sitemaps on the start's host
// are read; one that cannot be read lists nothing.
func (c *crawler) readSitemap(at string, follow bool) {
	if u, ok := normalizeURL(at, nil, true); !ok || u.Host != c.start.Host {
		return
	}
	page, err := c.get(at)
	if err != nil {
		return
	}
	var sm sitemap
	if xml.Unmarshal(page.Body, &sm) != nil {
		return
	}
	for _, l := range sm.URLs {
		c.enqueue(strings.TrimSpace(l.Loc), 1)
	}
	if follow {
		for _, child := range sm.Sitemaps[:min(len(sm.Sitemaps), maxSubSitemaps)] {
			c.readSitemap(strings.TrimSpace(child.Loc), false)
		}
	}
}

// normalizeURL resolves raw against base and returns one spelling per page:
// http or https only, the scheme and host lowercased, the default port and
// the fragment dropped, an empty path made /. The query is dropped unless
// keepQuery: doc sites use queries for tabs and tracking, not for pages.
func normalizeURL(raw string, base *url.URL, keepQuery bool) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, false
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, false
	}
	u.Host = strings.ToLower(u.Host)
	if port := u.Port(); port != "" && port == defaultPort(u.Scheme) {
		u.Host = u.Hostname()
	}
	u.Fragment, u.RawFragment, u.User = "", "", nil
	if !keepQuery {
		u.RawQuery = ""
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u, true
}

// urlKey is what makes two URLs one page: the host, the path without a
// trailing slash or index.html (Sphinx links a folder both ways), and the
// query. The scheme is left out: a site served on both http and https is
// one site.
func urlKey(u *url.URL) string {
	p := u.Path
	if base := strings.ToLower(path.Base(p)); base == "index.html" || base == "index.htm" {
		p = path.Dir(p)
	}
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	return u.Host + p + "?" + u.RawQuery
}

// assetExts are links a crawl never follows: they are not pages.
var assetExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".webp": true, ".ico": true,
	".bmp": true, ".avif": true, ".tif": true, ".tiff": true,
	".css": true, ".js": true, ".mjs": true, ".map": true, ".json": true, ".xml": true, ".rss": true, ".atom": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".zip": true, ".tar": true, ".gz": true, ".tgz": true, ".bz2": true, ".xz": true, ".7z": true, ".rar": true,
	".mp4": true, ".webm": true, ".mov": true, ".avi": true, ".mp3": true, ".wav": true, ".ogg": true, ".flac": true,
	".exe": true, ".dmg": true, ".pkg": true, ".deb": true, ".rpm": true, ".msi": true, ".apk": true,
	".iso": true, ".bin": true, ".wasm": true,
}

func isAsset(p string) bool {
	return assetExts[strings.ToLower(path.Ext(p))]
}

// linksIn are the href targets of a page's <a> elements, resolved against
// the page's final URL or its <base href>.
func linksIn(body []byte, page *url.URL) []string {
	base := page
	var out []string
	z := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return out
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			if !hasAttr || (string(name) != "a" && string(name) != "base") {
				continue
			}
			href := htmlAttr(z, "href")
			if href == "" {
				continue
			}
			if string(name) == "base" {
				if b, err := page.Parse(href); err == nil {
					base = b
				}
				continue
			}
			if u, ok := normalizeURL(href, base, false); ok {
				out = append(out, u.String())
			}
		}
	}
}

func htmlAttr(z *html.Tokenizer, key string) string {
	for {
		k, v, more := z.TagAttr()
		if string(k) == key {
			return string(v)
		}
		if !more {
			return ""
		}
	}
}

// globsAllow reports a path that matches an include (when there are any)
// and no exclude. Globs are `paths:` globs; a leading / is optional.
func globsAllow(rel string, include, exclude []string) bool {
	for _, g := range exclude {
		if pathglob.Match(strings.TrimPrefix(g, "/"), rel) {
			return false
		}
	}
	if len(include) == 0 {
		return true
	}
	for _, g := range include {
		if pathglob.Match(strings.TrimPrefix(g, "/"), rel) {
			return true
		}
	}
	return false
}
