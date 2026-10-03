package importdocs

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// robotsSite answers robots.txt with body and every other URL with a short
// article; it links nowhere.
func robotsSite(body string) Fetcher {
	article := "<html><body><article><h1>T</h1><p>" + strings.Repeat("Words for the article body. ", 30) + "</p></article></body></html>"
	return func(_ context.Context, u string) (Page, error) {
		switch {
		case strings.HasSuffix(u, "/robots.txt"):
			return Page{FinalURL: u, ContentType: "text/plain", Body: []byte(body)}, nil
		case strings.HasSuffix(u, "/llms.txt"), strings.HasSuffix(u, ".xml"):
			return Page{}, &StatusError{Code: 404, Status: "404 Not Found"}
		}
		return Page{FinalURL: u, ContentType: "text/html", Body: []byte(article)}, nil
	}
}

func recordSleeps(t *testing.T) *[]time.Duration {
	t.Helper()
	var got []time.Duration
	prev := sleep
	t.Cleanup(func() { sleep = prev })
	sleep = func(_ context.Context, d time.Duration) error {
		got = append(got, d)
		return nil
	}
	return &got
}

func TestCrawl_CrawlDelayIsObeyedOnlyWhenRespectingRobots(t *testing.T) {
	const robotsTxt = "User-agent: *\nCrawl-delay: 7\n"
	run := func(t *testing.T, co CrawlOptions) []time.Duration {
		t.Helper()
		sleeps := recordSleeps(t)
		co.MaxPages, co.Depth = 10, 5
		_, err := Crawl(t.Context(), "https://docs.example.com/guide/", t.TempDir(), Options{}, co, robotsSite(robotsTxt))
		require.NoError(t, err)
		return *sleeps
	}
	// robots.txt, llms.txt, sitemap.xml, the start page: three pauses.
	assert.Equal(t, []time.Duration{2 * time.Second, 2 * time.Second, 2 * time.Second},
		run(t, CrawlOptions{Delay: 2 * time.Second}))
	assert.Equal(t, []time.Duration{7 * time.Second, 7 * time.Second, 7 * time.Second},
		run(t, CrawlOptions{Delay: 2 * time.Second, RespectRobots: true}))
}

func TestCrawl_ACrawlDelayIsCapped(t *testing.T) {
	sleeps := recordSleeps(t)
	_, err := Crawl(t.Context(), "https://docs.example.com/", t.TempDir(), Options{},
		CrawlOptions{MaxPages: 1, Depth: 1, Delay: time.Second, RespectRobots: true},
		robotsSite("User-agent: *\nCrawl-delay: 3600\n"))
	require.NoError(t, err)
	require.NotEmpty(t, *sleeps)
	assert.Equal(t, maxCrawlDelay, (*sleeps)[len(*sleeps)-1])
}

func TestNormalizeURL_OneSpellingPerPage(t *testing.T) {
	got := map[string]string{}
	for _, raw := range []string{
		"HTTPS://Docs.Example.COM:443/a#frag", "http://docs.example.com:80/a?utm_source=x",
		"https://docs.example.com", "ftp://docs.example.com/a", "mailto:a@b.c", "/relative",
	} {
		if u, ok := normalizeURL(raw, nil, false); ok {
			got[raw] = u.String() + " " + urlKey(u)
		} else {
			got[raw] = "refused"
		}
	}
	assert.Equal(t, map[string]string{
		"HTTPS://Docs.Example.COM:443/a#frag":       "https://docs.example.com/a docs.example.com/a?",
		"http://docs.example.com:80/a?utm_source=x": "http://docs.example.com/a docs.example.com/a?",
		"https://docs.example.com":                  "https://docs.example.com/ docs.example.com/?",
		"ftp://docs.example.com/a":                  "refused",
		"mailto:a@b.c":                              "refused",
		"/relative":                                 "refused",
	}, got)
}

// A folder and its index.html are one page: Sphinx links both.
func TestURLKey_AFoldersIndexPageIsTheFolder(t *testing.T) {
	keys := map[string]bool{}
	for _, raw := range []string{"https://d.org/usage/", "https://d.org/usage", "https://d.org/usage/index.html", "https://d.org/usage/INDEX.HTM"} {
		u, ok := normalizeURL(raw, nil, false)
		require.True(t, ok)
		keys[urlKey(u)] = true
	}
	assert.Len(t, keys, 1, "%v", keys)
	u, _ := normalizeURL("https://d.org/usage/indexes.html", nil, false)
	assert.NotEqual(t, "d.org/usage?", urlKey(u))
}

// The queue holds at most queueFactor × MaxPages URLs however many a
// sitemap lists; the rest are only counted.
func TestDiscover_AHugeSitemapIsCountedNotHeld(t *testing.T) {
	var b strings.Builder
	b.WriteString("<urlset>")
	for i := 0; i < 500; i++ {
		b.WriteString("<url><loc>https://docs.example.com/guide/p" + strconv.Itoa(i) + "</loc></url>")
	}
	b.WriteString("</urlset>")
	fetch := func(_ context.Context, u string) (Page, error) {
		if strings.HasSuffix(u, "/sitemap.xml") {
			return Page{FinalURL: u, ContentType: "application/xml", Body: []byte(b.String())}, nil
		}
		return Page{}, &StatusError{Code: 404, Status: "404 Not Found"}
	}
	c, err := newCrawler(t.Context(), "https://docs.example.com/guide/", CrawlOptions{MaxPages: 3, Depth: 5}, fetch)
	require.NoError(t, err)
	require.NoError(t, c.discover())
	assert.Len(t, c.queue, 30, "the start and 29 listed pages")
	assert.Equal(t, 471, c.dropped)
}
