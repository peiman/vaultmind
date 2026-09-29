package importdocs_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fts5HTML is long enough for readability to keep the article and drop the
// nav and footer. The title and the h1 differ so the note's title shows
// which one was used.
func fts5HTML() string {
	para := "The FTS5 extension stores an inverted index for fast word lookup. " +
		"A query finds every row that mentions a word without scanning the whole table. " +
		"Prefix terms combine with boolean filters, and the highlight function wraps each match. " +
		"Tokenizers decide how words are split, including unicode words and separators. "
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><title>SQLite FTS5 Extension</title></head><body>\n")
	b.WriteString("<nav>NAV-MENU-UNIQUE Home Docs Download</nav>\n")
	b.WriteString("<article>\n")
	// A breadcrumb readability keeps. The note must open on the article.
	b.WriteString("<ol>\n")
	b.WriteString("<li><a href=\"https://go.dev/doc/\">Documentation</a></li>\n")
	b.WriteString("<li><a href=\"https://go.dev/doc/effective_go\">Effective Go</a></li>\n")
	b.WriteString("</ol>\n")
	b.WriteString("<h1>Full-Text Search in SQLite</h1>\n")
	for i := 1; i <= 8; i++ {
		fmt.Fprintf(&b, "<p>%sParagraph %d stands on its own.</p>\n", para, i)
	}
	b.WriteString("</article>\n<footer>FOOTER-UNIQUE Copyright held by the example foundation.</footer>\n")
	b.WriteString("</body></html>\n")
	return b.String()
}

func pageFetcher(page importdocs.Page) importdocs.Fetcher {
	return func(context.Context, string) (importdocs.Page, error) {
		return page, nil
	}
}

func importPage(t *testing.T, rawURL, vault string, opts importdocs.Options, fetch importdocs.Fetcher) *importdocs.Result {
	t.Helper()
	res, err := importdocs.ImportURL(t.Context(), rawURL, vault, opts, fetch)
	require.NoError(t, err)
	return res
}

func noteFront(t *testing.T, vault, rel string) (map[string]interface{}, string, string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(rel))) //nolint:gosec // vault path from the test
	require.NoError(t, err)
	fm, body, err := parser.ExtractFrontmatter(raw)
	require.NoError(t, err)
	return fm, body, string(raw)
}

func TestImportURL_HTMLPageBecomesOneNoteWithoutNavOrFooter(t *testing.T) {
	const given = "https://sqlite.org/fts5.html"
	vault := t.TempDir()
	res := importPage(t, given, vault, importdocs.Options{}, pageFetcher(importdocs.Page{
		// A redirect's final URL must not move the note. The path comes from
		// the URL the operator named.
		FinalURL:    "https://cdn.sqlite.org/fts5.html",
		ContentType: "text/html; charset=utf-8",
		Body:        []byte(fts5HTML()),
	}))

	const rel = "imported/web/sqlite.org/fts5.md"
	require.Equal(t, 1, res.Count(importdocs.Added))
	require.Len(t, res.Entries, 1)
	assert.Equal(t, rel, res.Entries[0].Note)
	assert.Equal(t, given, res.Entries[0].Source)

	fm, body, raw := noteFront(t, vault, rel)
	assert.Equal(t, "imported-web-sqlite-org-fts5", fm["id"])
	assert.Equal(t, "reference", fm["type"])
	assert.Equal(t, "SQLite FTS5 Extension", fm["title"])
	assert.Equal(t, given, fm["url"])
	assert.Equal(t, given, fm["source"])
	_, hasPaths := fm["paths"]
	assert.False(t, hasPaths, "a page note has no paths")
	sum := sha256.Sum256([]byte(body))
	assert.Equal(t, hex.EncodeToString(sum[:]), fm["source_hash"], "source_hash is the note body")
	assert.Contains(t, raw, "# Imported by `vaultmind import` from https://sqlite.org/fts5.html.\n")
	assert.Contains(t, raw, "# The page is the source: re-run the import to refresh it.\n")
	assert.Contains(t, body, "The FTS5 extension stores an inverted index for fast word lookup.")
	assert.NotContains(t, body, "NAV-MENU-UNIQUE")
	assert.NotContains(t, body, "FOOTER-UNIQUE")
	assert.False(t, strings.HasPrefix(body, "1. [Documentation]"), "a breadcrumb readability kept must not open the note")
	assert.True(t, strings.HasPrefix(body, "## Full-Text Search in SQLite\n"), "the note opens on the article, got %q", body[:min(80, len(body))])
	assert.True(t, strings.HasSuffix(body, "\n"))
	assert.False(t, strings.HasSuffix(strings.TrimSuffix(body, "\n"), "\n"), "exactly one trailing newline")
}

// A text page may arrive with its own frontmatter. That block is not the
// note's: the title is kept, the rest is dropped, and the note's id is the
// import's. The same split a folder import uses for a doc.
func TestImportURL_ServedFrontmatterIsStrippedAndItsTitleIsKept(t *testing.T) {
	const served = "---\ntitle: Real Title\nid: evil\n---\n# H\nBody"
	cases := []struct {
		ctype, url, rel, id string
	}{
		{"text/markdown", "https://example.com/served.md", "imported/web/example.com/served.md", "imported-web-example-com-served"},
		{"text/x-markdown", "https://example.com/x.md", "imported/web/example.com/x.md", "imported-web-example-com-x"},
		{"text/plain", "https://example.com/plain.txt", "imported/web/example.com/plain-txt.md", "imported-web-example-com-plain-txt"},
	}
	for _, tc := range cases {
		t.Run(tc.ctype, func(t *testing.T) {
			vault := t.TempDir()
			importPage(t, tc.url, vault, importdocs.Options{}, pageFetcher(importdocs.Page{
				FinalURL: tc.url, ContentType: tc.ctype, Body: []byte(served),
			}))
			fm, body, _ := noteFront(t, vault, tc.rel)
			assert.Equal(t, "Real Title", fm["title"])
			assert.Equal(t, tc.id, fm["id"])
			assert.NotEqual(t, "evil", fm["id"])
			assert.True(t, strings.HasPrefix(body, "# H\n"), "body %q", body)
			assert.NotContains(t, body, "evil")
		})
	}
}

func TestImportURL_MarkdownBodyIsKeptAndTitleComesFromTheHeading(t *testing.T) {
	const (
		given = "https://example.com/docs/guide.md#section"
		rel   = "imported/web/example.com/docs-guide.md"
		// Leading and trailing whitespace is trimmed; the note ends with one newline.
		served = "\n\n# Guide Title\n\nThe body text.\n\n"
		want   = "# Guide Title\n\nThe body text.\n"
	)
	vault := t.TempDir()
	importPage(t, given, vault, importdocs.Options{}, pageFetcher(importdocs.Page{
		FinalURL:    given,
		ContentType: "text/markdown",
		Body:        []byte(served),
	}))

	fm, body, _ := noteFront(t, vault, rel)
	assert.Equal(t, want, body)
	assert.Equal(t, "Guide Title", fm["title"])
	assert.Equal(t, given, fm["url"])
	assert.Equal(t, "imported-web-example-com-docs-guide", fm["id"])
	sum := sha256.Sum256([]byte(want))
	assert.Equal(t, hex.EncodeToString(sum[:]), fm["source_hash"])
}

func TestImportURL_NotePathAndTitleFollowTheURL(t *testing.T) {
	cases := []struct {
		url, ctype, body, rel, id, title string
	}{
		{
			url: "https://example.com/", ctype: "text/plain", body: "Hello.\n",
			rel: "imported/web/example.com/index.md", id: "imported-web-example-com-index",
			title: "example.com/",
		},
		{
			url: "https://example.com/docs/guide.md?mode=full#x", ctype: "text/x-markdown",
			body: "# Guide\n\nBody.\n",
			rel:  "imported/web/example.com/docs-guide-mode-full.md",
			id:   "imported-web-example-com-docs-guide-mode-full", title: "Guide",
		},
		{
			url: "http://Example.COM:8080/A.HTML", ctype: "text/plain", body: "Port.\n",
			rel: "imported/web/example.com-8080/a.md", id: "imported-web-example-com-8080-a",
			title: "example.com/A.HTML",
		},
		{
			url: "https://example.com:443/a.htm", ctype: "text/plain", body: "Secure.\n",
			rel: "imported/web/example.com/a.md", id: "imported-web-example-com-a",
			title: "example.com/a.htm",
		},
		{
			url: "http://example.com:80/a.md", ctype: "text/plain", body: "Default port.\n",
			rel: "imported/web/example.com/a.md", id: "imported-web-example-com-a",
			title: "example.com/a.md",
		},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			vault := t.TempDir()
			importPage(t, tc.url, vault, importdocs.Options{}, pageFetcher(importdocs.Page{
				FinalURL: tc.url, ContentType: tc.ctype, Body: []byte(tc.body),
			}))
			fm, body, _ := noteFront(t, vault, tc.rel)
			assert.Equal(t, tc.id, fm["id"])
			assert.Equal(t, tc.title, fm["title"])
			assert.Equal(t, strings.TrimSpace(tc.body)+"\n", body)
		})
	}
}

func TestImportURL_RerunIsUnchangedUntilThePageOrAHandEditChanges(t *testing.T) {
	const (
		given = "https://example.com/notes/sync.md"
		rel   = "imported/web/example.com/notes-sync.md"
	)
	served := "# Sync Title\n\nOriginal paragraph.\n"
	vault := t.TempDir()
	fetch := func(context.Context, string) (importdocs.Page, error) {
		return importdocs.Page{FinalURL: given, ContentType: "text/markdown", Body: []byte(served)}, nil
	}
	opts := importdocs.Options{}

	res := importPage(t, given, vault, opts, fetch)
	assert.Equal(t, 1, res.Count(importdocs.Added))

	res = importPage(t, given, vault, opts, fetch)
	assert.Equal(t, 1, res.Count(importdocs.Unchanged))
	assert.Zero(t, res.Count(importdocs.Updated))

	served = "# Sync Title\n\nUpdated paragraph.\n"
	res = importPage(t, given, vault, opts, fetch)
	assert.Equal(t, 1, res.Count(importdocs.Updated))
	_, body, _ := noteFront(t, vault, rel)
	assert.Contains(t, body, "Updated paragraph.")

	note := filepath.Join(vault, filepath.FromSlash(rel))
	raw, err := os.ReadFile(note) //nolint:gosec // vault path from the test
	require.NoError(t, err)
	write(t, note, strings.Replace(string(raw), "Updated paragraph.", "Updated paragraph. A hand-written aside.", 1))
	served = "# Sync Title\n\nThird paragraph.\n"

	res = importPage(t, given, vault, opts, fetch)
	assert.Equal(t, 1, res.Count(importdocs.Conflict))
	kept, err := os.ReadFile(note) //nolint:gosec // vault path from the test
	require.NoError(t, err)
	assert.Contains(t, string(kept), "hand-written aside")
	assert.NotContains(t, string(kept), "Third paragraph.")

	res = importPage(t, given, vault, importdocs.Options{Force: true}, fetch)
	assert.Equal(t, 1, res.Count(importdocs.Updated))
	forced, err := os.ReadFile(note) //nolint:gosec // vault path from the test
	require.NoError(t, err)
	assert.Contains(t, string(forced), "Third paragraph.")
	assert.NotContains(t, string(forced), "hand-written aside")
}

func TestImportURL_LeavesANoteItDidNotWriteUntouched(t *testing.T) {
	const (
		given = "https://example.com/notes/sync.md"
		rel   = "imported/web/example.com/notes-sync.md"
	)
	vault := t.TempDir()
	mine := filepath.Join(vault, filepath.FromSlash(rel))
	write(t, mine, "---\nid: my-own\ntype: concept\n---\nMine.\n")

	res := importPage(t, given, vault, importdocs.Options{Force: true}, pageFetcher(importdocs.Page{
		FinalURL: given, ContentType: "text/markdown", Body: []byte("# Sync Title\n\nOriginal paragraph.\n"),
	}))
	assert.Equal(t, 1, res.Count(importdocs.Skipped))
	assert.Contains(t, res.Entries[0].Reason, "did not write")
	kept, err := os.ReadFile(mine) //nolint:gosec // vault path from the test
	require.NoError(t, err)
	assert.Contains(t, string(kept), "Mine.")
}

func TestImportURL_DryRunFetchesButWritesNothing(t *testing.T) {
	const given = "https://example.com/docs/guide.md"
	called := 0
	vault := t.TempDir()
	res := importPage(t, given, vault, importdocs.Options{DryRun: true}, func(context.Context, string) (importdocs.Page, error) {
		called++
		return importdocs.Page{FinalURL: given, ContentType: "text/plain", Body: []byte("Hello.\n")}, nil
	})
	assert.Equal(t, 1, called)
	assert.True(t, res.DryRun)
	assert.Equal(t, 1, res.Count(importdocs.Added))
	assert.NoDirExists(t, filepath.Join(vault, "imported"))
}

func TestImportURL_FetchErrorsWriteNothing(t *testing.T) {
	fetch := importdocs.HTTPFetcher()
	assertNothing := func(t *testing.T, rawURL string, want string) {
		t.Helper()
		vault := t.TempDir()
		_, err := importdocs.ImportURL(t.Context(), rawURL, vault, importdocs.Options{}, fetch)
		require.Error(t, err)
		assert.Contains(t, err.Error(), want)
		assert.NoDirExists(t, filepath.Join(vault, "imported"))
	}

	t.Run("pdf", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.4"))
		}))
		t.Cleanup(srv.Close)
		assertNothing(t, srv.URL+"/file.pdf", "not a text page (application/pdf)")
	})

	t.Run("oversized", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write(bytesOf(5<<20 + 1))
		}))
		t.Cleanup(srv.Close)
		assertNothing(t, srv.URL+"/big.txt", "page is larger than 5 MiB")
	})

	// U+0800 is 2 bytes in UTF-16LE and 3 in UTF-8. A raw body just under
	// 5 MiB therefore decodes to more than 5 MiB, and must be refused
	// after charset decoding, with nothing written.
	t.Run("utf16 grows past the limit", func(t *testing.T) {
		const maxPageBytes = 5 << 20
		raw := bytes.Repeat([]byte{0x00, 0x08}, (maxPageBytes/2)-1)
		require.Less(t, len(raw), maxPageBytes)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-16le")
			_, _ = w.Write(raw)
		}))
		t.Cleanup(srv.Close)
		assertNothing(t, srv.URL+"/wide.txt", "page is larger than 5 MiB")
	})

	t.Run("status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "missing", http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)
		assertNothing(t, srv.URL+"/missing", "404")
	})

	t.Run("redirect scheme", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "ftp://example.com/secret", http.StatusFound)
		}))
		t.Cleanup(srv.Close)
		assertNothing(t, srv.URL+"/go", "not http or https")
	})
}

func TestHTTPFetcher_StopsAfterFiveRedirects(t *testing.T) {
	t.Run("five land", func(t *testing.T) {
		var srv *httptest.Server
		n := 0
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n++
			if n <= 5 {
				http.Redirect(w, r, srv.URL+"/next", http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, "landed\n")
		}))
		t.Cleanup(srv.Close)
		page, err := importdocs.HTTPFetcher()(t.Context(), srv.URL)
		require.NoError(t, err)
		assert.Equal(t, "landed\n", string(page.Body))
		assert.Equal(t, 6, n, "the original request plus five redirects")
	})

	t.Run("a sixth is refused", func(t *testing.T) {
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, srv.URL+"/loop", http.StatusFound)
		}))
		t.Cleanup(srv.Close)
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		_, err := importdocs.HTTPFetcher()(ctx, srv.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stopped after 5 redirects")
		assert.NotContains(t, err.Error(), "deadline")
	})
}

func TestHTTPFetcher_NamesItselfAndKeepsNoCookies(t *testing.T) {
	prev := importdocs.UserAgentVersion
	importdocs.UserAgentVersion = "9.9.9-test"
	t.Cleanup(func() { importdocs.UserAgentVersion = prev })

	var srv *httptest.Server
	var sawCookie, sawUA bool
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUA = r.Header.Get("User-Agent") == "vaultmind/9.9.9-test (+https://github.com/peiman/vaultmind)"
		if r.URL.Path == "/" {
			if r.Header.Get("Cookie") != "" {
				sawCookie = true
			}
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "secret"})
			http.Redirect(w, r, srv.URL+"/next", http.StatusFound)
			return
		}
		if r.Header.Get("Cookie") != "" {
			sawCookie = true
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "ok\n")
	}))
	t.Cleanup(srv.Close)

	page, err := importdocs.HTTPFetcher()(t.Context(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "ok\n", string(page.Body))
	assert.True(t, sawUA)
	assert.False(t, sawCookie)
}

func TestHTTPFetcher_DecodesTheContentTypeCharset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=iso-8859-1")
		_, _ = w.Write([]byte{'c', 'a', 'f', 0xe9})
	}))
	t.Cleanup(srv.Close)

	vault := t.TempDir()
	res := importPage(t, srv.URL+"/cafe", vault, importdocs.Options{}, importdocs.HTTPFetcher())
	require.Equal(t, 1, res.Count(importdocs.Added))
	_, body, _ := noteFront(t, vault, res.Entries[0].Note)
	assert.Equal(t, "café\n", body)
}

// A folder import of a repository named https must not treat a URL note as
// an orphan. docGone refuses any source containing ://, including a note that
// sits inside that repository's own imported folder.
func TestImport_ARepoNamedHTTPSDoesNotPruneURLNotes(t *testing.T) {
	const given = "https://sqlite.org/fts5.html"
	vault := t.TempDir()
	importPage(t, given, vault, importdocs.Options{}, pageFetcher(importdocs.Page{
		FinalURL: given, ContentType: "text/html; charset=utf-8", Body: []byte(fts5HTML()),
	}))
	webNote := filepath.Join(vault, "imported", "web", "sqlite.org", "fts5.md")
	require.FileExists(t, webNote)

	body := "Planted page.\n"
	sum := sha256.Sum256([]byte(body))
	planted := filepath.Join(vault, "imported", "https", "planted.md")
	write(t, planted, fmt.Sprintf("---\nid: planted-url\ntype: reference\ntitle: Planted\nsource: https://example.com/planted\nsource_hash: %s\n---\n%s", hex.EncodeToString(sum[:]), body))

	repo := t.TempDir()
	write(t, filepath.Join(repo, "alpha.md"), "# Alpha\n\nAlpha text.\n")
	res, err := importdocs.Import(importdocs.Source{Dir: repo, Repo: "https"}, vault, importdocs.Options{Prune: true})
	require.NoError(t, err)

	assert.Zero(t, res.Count(importdocs.Orphaned))
	assert.Zero(t, res.Count(importdocs.Pruned))
	assert.FileExists(t, webNote)
	assert.FileExists(t, planted)
	for _, e := range res.Entries {
		assert.NotContains(t, e.Note, "imported/web/")
		assert.NotContains(t, e.Source, "://")
	}
}

// https://x.test/a/b and https://x.test/a-b slug to one note. The second
// import must not replace the first page, and --force must not either.
func TestImportURL_ADifferentPageAtTheSamePathIsSkipped(t *testing.T) {
	const (
		pageA = "https://x.test/a/b"
		pageB = "https://x.test/a-b"
		rel   = "imported/web/x.test/a-b.md"
	)
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%v", force), func(t *testing.T) {
			vault := t.TempDir()
			res := importPage(t, pageA, vault, importdocs.Options{}, pageFetcher(importdocs.Page{
				FinalURL: pageA, ContentType: "text/plain", Body: []byte("Page A.\n"),
			}))
			require.Equal(t, 1, res.Count(importdocs.Added))
			note := filepath.Join(vault, filepath.FromSlash(rel))
			before, err := os.ReadFile(note) //nolint:gosec // vault path from the test
			require.NoError(t, err)

			res = importPage(t, pageB, vault, importdocs.Options{Force: force}, pageFetcher(importdocs.Page{
				FinalURL: pageB, ContentType: "text/plain", Body: []byte("Page B.\n"),
			}))
			require.Len(t, res.Entries, 1)
			assert.Equal(t, importdocs.Skipped, res.Entries[0].Action)
			assert.Equal(t, "another page maps to this note ("+pageA+")", res.Entries[0].Reason)
			after, err := os.ReadFile(note) //nolint:gosec // vault path from the test
			require.NoError(t, err)
			assert.Equal(t, before, after)
			assert.NotContains(t, string(after), "Page B.")
		})
	}
}

// A folder import's note (source repo:path) sitting where a URL lands is
// another page's note. The URL import skips it through the same check.
func TestImportURL_AFolderNoteAtThePagePathIsSkipped(t *testing.T) {
	const (
		given  = "https://x.test/a/b"
		rel    = "imported/web/x.test/a-b.md"
		source = "demo-repo:docs/a-b.md"
	)
	vault := t.TempDir()
	body := "Folder note.\n"
	sum := sha256.Sum256([]byte(body))
	note := filepath.Join(vault, filepath.FromSlash(rel))
	before := fmt.Sprintf("---\nid: imported-demo-repo-docs-a-b\ntype: reference\ntitle: A B\nsource: %s\nsource_hash: %s\n---\n%s", source, hex.EncodeToString(sum[:]), body)
	write(t, note, before)

	res := importPage(t, given, vault, importdocs.Options{Force: true}, pageFetcher(importdocs.Page{
		FinalURL: given, ContentType: "text/plain", Body: []byte("Page from the URL.\n"),
	}))
	require.Len(t, res.Entries, 1)
	assert.Equal(t, importdocs.Skipped, res.Entries[0].Action)
	assert.Equal(t, "another page maps to this note ("+source+")", res.Entries[0].Reason)
	after, err := os.ReadFile(note) //nolint:gosec // vault path from the test
	require.NoError(t, err)
	assert.Equal(t, before, string(after))
}

func bytesOf(n int) []byte {
	return []byte(strings.Repeat("a", n))
}
