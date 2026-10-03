package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// docsSite serves /docs/ linking to two pages, each with enough text for
// readability, and records every path asked for.
func docsSite(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		links := map[string][]string{"/docs/": {"/docs/install", "/docs/usage"}, "/docs/install": nil, "/docs/usage": nil}
		l, ok := links[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var b strings.Builder
		fmt.Fprintf(&b, "<html><body><article><h1>Docs %s</h1>", r.URL.Path)
		for i := 0; i < 5; i++ {
			fmt.Fprintf(&b, "<p>The page %s describes a step of the tool in words enough to be an article. %d</p>", r.URL.Path, i)
		}
		for _, href := range l {
			fmt.Fprintf(&b, `<p><a href="%s">next</a></p>`, href)
		}
		b.WriteString("</article></body></html>")
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)
	return srv, &asked
}

func TestImport_CrawlImportsTheSiteAndIndexesIt(t *testing.T) {
	vault := indexedBaselineVault(t)
	srv, asked := docsSite(t)

	out, _, err := runRootCmd(t, "import", srv.URL+"/docs/", "--crawl", "--delay", "0s", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "3 added")
	assert.Contains(t, *asked, "/robots.txt", "robots.txt is read for sitemaps")

	out, _, err = runRootCmd(t, "search", "describes a step", "--vault", vault, "--json")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "docs-install", "the crawl indexed what it wrote")
}

func TestImport_CrawlFlagsBoundTheCrawl(t *testing.T) {
	vault := indexedBaselineVault(t)
	srv, _ := docsSite(t)

	out, _, err := runRootCmd(t, "import", srv.URL+"/docs/", "--crawl", "--delay", "0s", "--max-pages", "2",
		"--exclude", "docs/usage", "--vault", vault, "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "2 added")
	assert.NotContains(t, out.String(), "docs-usage")
	assert.NoDirExists(t, filepath.Join(vault, "imported"))
}

func TestImport_CrawlRefusesBadArguments(t *testing.T) {
	vault := indexedBaselineVault(t)
	cases := map[string][]string{
		"--crawl needs an http or https URL": {"import", t.TempDir(), "--crawl"},
		"--max-pages must be at least 1":     {"import", "https://example.com/", "--crawl", "--max-pages", "0"},
		"--depth must not be negative":       {"import", "https://example.com/", "--crawl", "--depth", "-1"},
		"--delay":                            {"import", "https://example.com/", "--crawl", "--delay", "soon"},
	}
	for want, args := range cases {
		_, _, err := runRootCmd(t, append(args, "--vault", vault)...)
		if assert.Error(t, err, want) {
			assert.Contains(t, err.Error(), want)
		}
	}
}
