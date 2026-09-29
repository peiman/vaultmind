package importdocs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

// maxPageBytes is the most of a page that is read. One extra byte is read
// so a page of exactly this size is kept and anything larger is refused.
const maxPageBytes = 5 << 20

// pageTimeout bounds the whole fetch, redirects included.
const pageTimeout = 30 * time.Second

// maxRedirects is how many redirects are followed. The next one is refused.
const maxRedirects = 5

// UserAgentVersion is the version in the User-Agent. The command sets it
// from the binary's version string. Empty means "dev".
var UserAgentVersion = "dev"

// Page is one fetched page. Body is UTF-8. FinalURL is where redirects
// landed; the note's address is the URL the operator named, not this one.
type Page struct {
	FinalURL    string
	ContentType string
	Body        []byte
}

// Fetcher retrieves one page. ImportURL takes one so a test can answer
// without the network; the command passes HTTPFetcher.
type Fetcher func(ctx context.Context, rawURL string) (Page, error)

// HTTPFetcher fetches a page with net/http. No cookies are stored. Redirects
// stop at five and must stay on http or https. A body over 5 MiB, a non-2xx
// status, or a media type that is not text is an error and nothing is written.
func HTTPFetcher() Fetcher {
	client := &http.Client{
		Timeout:       pageTimeout,
		CheckRedirect: redirectPolicy,
		Jar:           nil,
	}
	return func(ctx context.Context, rawURL string) (Page, error) {
		return fetchPage(ctx, client, rawURL)
	}
}

func userAgent() string {
	v := UserAgentVersion
	if v == "" {
		v = "dev"
	}
	return "vaultmind/" + v + " (+https://github.com/peiman/vaultmind)"
}

// redirectPolicy refuses a hop off http/https, then a sixth hop.
// via is the requests already made, not the one about to be sent, so five
// followed redirects leave len(via) == 5 when the landing request is allowed
// and len(via) == 6 when a sixth redirect is refused.
func redirectPolicy(req *http.Request, via []*http.Request) error {
	scheme := ""
	if req != nil && req.URL != nil {
		scheme = req.URL.Scheme
	}
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("redirect to %q is not http or https", scheme)
	}
	if len(via) >= maxRedirects+1 {
		return errors.New("stopped after 5 redirects")
	}
	return nil
}

func fetchPage(ctx context.Context, client *http.Client, rawURL string) (Page, error) {
	resp, err := doGet(ctx, client, rawURL)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		return Page{}, err
	}
	if err := pageStatus(resp); err != nil {
		return Page{}, err
	}
	ctype := resp.Header.Get("Content-Type")
	if _, err := acceptedMedia(ctype); err != nil {
		return Page{}, err
	}
	body, err := readPage(resp.Body, ctype)
	if err != nil {
		return Page{}, err
	}
	return Page{FinalURL: finalURL(resp, rawURL), ContentType: ctype, Body: body}, nil
}

func doGet(ctx context.Context, client *http.Client, rawURL string) (*http.Response, error) {
	// The operator named this URL. The scheme was checked, and every redirect
	// has to stay on http or https.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil) //nolint:gosec // G107: the page the operator asked to import
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent())
	resp, err := client.Do(req)
	if err != nil {
		// CheckRedirect's failure returns the previous response with its body
		// already closed. Close again when it is still open; a nil body is a
		// transport error and has nothing to close.
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, err
	}
	return resp, nil
}

func pageStatus(resp *http.Response) error {
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("page returned %s", resp.Status)
	}
	return nil
}

func finalURL(resp *http.Response, rawURL string) string {
	if resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL.String()
	}
	return rawURL
}

func readPage(r io.Reader, contentType string) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxPageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxPageBytes {
		return nil, errors.New("page is larger than 5 MiB")
	}
	return decodeCharset(body, contentType)
}

func decodeCharset(body []byte, contentType string) ([]byte, error) {
	decoded, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(decoded)
}

// textPageTypes are the media types a page import accepts.
var textPageTypes = map[string]bool{
	"text/html":             true,
	"application/xhtml+xml": true,
	"text/markdown":         true,
	"text/x-markdown":       true,
	"text/plain":            true,
}

// acceptedMedia parses a Content-Type and refuses anything that is not text.
// The error names the type, so a PDF is refused as a type rather than read.
func acceptedMedia(header string) (string, error) {
	mt, _, err := mime.ParseMediaType(header)
	if err != nil || !textPageTypes[mt] {
		shown := mt
		if shown == "" {
			shown = strings.TrimSpace(header)
		}
		if shown == "" {
			shown = "missing"
		}
		return "", fmt.Errorf("not a text page (%s)", shown)
	}
	return mt, nil
}
