// Package doctorclient is a LOOPBACK-PINNED, read-only HTTP client for probing
// the LOCAL Contract-B chat daemon's well-known/whoami endpoints from `vaultmind
// doctor`. It is deliberately NOT relayclient.FetchRoot: that client is
// remote-permissive (enroll fetches REMOTE relays). doctor only ever talks to a
// daemon on loopback, so this client is hard fail-closed:
//
//   - resolve-then-check: it resolves the daemon host and refuses unless EVERY
//     resolved IP satisfies net.IP.IsLoopback (an allowlist of loopback fails
//     closed on any non-loopback form — no alternate-IP bypass).
//   - DNS-rebind defense: it pins the resolved loopback IP in a custom
//     DialContext, so a TOCTOU rebind between resolve and dial cannot redirect
//     the connection off-loopback.
//   - redirect refusal: CheckRedirect refuses ALL redirects (well-known and
//     whoami endpoints never legitimately redirect).
//   - scheme allowlist: http/https only.
//   - body caps: io.LimitReader bounds every response (64 KiB for the small
//     well-known/whoami payloads, 1 MiB for the registry directory, mirroring the
//     signer's maxRequestBytes); an over-cap body fails LOUD, never silently
//     truncated.
//   - inescapable deadline: every fetch wraps the request in its OWN
//     context.WithTimeout so a caller cannot hang doctor.
//
// It performs NO trust decisions: authenticity is owned by the pinned-root verify
// in the doctor mesh logic. This package is pure transport.
package doctorclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Well-known + whoami paths the local daemon serves (SSOT). They mirror the
// agent-chat daemon routes: /.well-known/vaultmind-root (200/404),
// /.well-known/vaultmind-directory (raw signed registry bytes), /whoami.
const (
	// WellKnownRootPath serves {root_pubkey, network_id, root_key_epoch?}.
	WellKnownRootPath = "/.well-known/vaultmind-root"
	// WellKnownDirectoryPath serves the byte-exact signed registry artifact.
	WellKnownDirectoryPath = "/.well-known/vaultmind-directory"
	// WhoamiPath serves the daemon's static configured identity.
	WhoamiPath = "/whoami"
)

// Body caps. The well-known root + whoami payloads are tiny (~150 B); 64 KiB is
// generous headroom. The directory is the full signed registry; 1 MiB mirrors
// the signer's maxRequestBytes. An over-cap body fails LOUD.
const (
	maxWellKnownBodyBytes = 64 << 10
	maxDirectoryBodyBytes = 1 << 20
)

// defaultFetchTimeout bounds EVERY fetch unless overridden via WithTimeout. It is
// the inescapable deadline a caller cannot defeat.
const defaultFetchTimeout = 5 * time.Second

// Scheme allowlist (SSOT).
const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// Error strings (SSOT). Each rejection is a distinct, named reason.
const (
	// errBadScheme is returned when the daemon base URL is unparseable, has no
	// host, or carries a non-http(s) scheme.
	errBadScheme = "doctorclient: daemon URL must be an http(s) URL with a host"
	// errNonLoopback is returned (fail-closed) when the daemon host resolves to
	// zero IPs or to ANY non-loopback IP. The allowlist admits ONLY loopback.
	errNonLoopback = "doctorclient: daemon host does not resolve to loopback (refusing off-loopback probe)"
	// errRedirect is returned when the daemon responds with a redirect — the
	// well-known/whoami endpoints never legitimately redirect.
	errRedirect = "doctorclient: refusing redirect from local daemon"
	// errResolve wraps a resolver failure.
	errResolve = "doctorclient: resolve daemon host"
	// errBuildRequest wraps a request-construction failure (should not happen).
	errBuildRequest = "doctorclient: build request"
	// errFetch wraps a transport failure (dial/timeout/cancelled context).
	errFetch = "doctorclient: fetch"
	// errStatus wraps an unexpected non-200/non-404 status.
	errStatus = "doctorclient: unexpected status"
	// errDecode wraps a JSON decode failure of a well-known/whoami body.
	errDecode = "doctorclient: decode response JSON"
	// errBodyTooLarge is returned when a response body exceeds its cap.
	errBodyTooLarge = "doctorclient: response body exceeds cap"
	// errReadBody wraps a body read failure.
	errReadBody = "doctorclient: read response body"
)

// ErrNotConfigured is returned by FetchRoot/FetchDirectory when the daemon
// answers 404 — i.e. the endpoint exists but no root/registry is configured
// (plaintext daemon). It is a typed sentinel so the caller can distinguish
// "served nothing" from a transport error.
var ErrNotConfigured = errors.New("doctorclient: daemon endpoint not configured (404)")

// WellKnownRoot is the daemon-advertised PUBLIC trust anchor. It is UNTRUSTED:
// the doctor logic verifies a registry against a PINNED root, never this.
type WellKnownRoot struct {
	RootPubKey   string `json:"root_pubkey"`
	NetworkID    string `json:"network_id"`
	RootKeyEpoch int64  `json:"root_key_epoch,omitempty"`
}

// whoamiResponse mirrors the daemon's /whoami body.
type whoamiResponse struct {
	Agent string `json:"agent"`
}

// resolverFunc resolves a host to its IPs. It is a seam so tests can drive the
// loopback-allowlist check without real DNS. Production uses net.DefaultResolver.
type resolverFunc func(ctx context.Context, host string) ([]net.IP, error)

// Client is a loopback-pinned read-only probe of one local daemon base URL.
type Client struct {
	baseURL  *url.URL
	host     string
	resolve  resolverFunc
	timeout  time.Duration
	pinnedIP net.IP
}

// Option configures a Client.
type Option func(*Client)

// WithResolver overrides the DNS resolver (test seam).
func WithResolver(r resolverFunc) Option { return func(c *Client) { c.resolve = r } }

// WithTimeout overrides the inescapable per-fetch deadline.
func WithTimeout(d time.Duration) Option { return func(c *Client) { c.timeout = d } }

// New validates baseURL (scheme + host) and returns a Client. The loopback
// resolve-check happens per-fetch (so a transient resolver failure surfaces at
// the call, not at construction), pinning the resolved IP for the dial.
func New(baseURL string, opts ...Option) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != schemeHTTP && u.Scheme != schemeHTTPS) || u.Host == "" {
		return nil, fmt.Errorf("%s", errBadScheme)
	}
	c := &Client{
		baseURL: u,
		host:    u.Hostname(),
		resolve: defaultResolve,
		timeout: defaultFetchTimeout,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// defaultResolve resolves via net.DefaultResolver.
func defaultResolve(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}

// FetchRoot GETs the well-known root and decodes it. A 404 returns
// ErrNotConfigured (the daemon is plaintext / no root configured).
func (c *Client) FetchRoot(ctx context.Context) (WellKnownRoot, error) {
	body, err := c.get(ctx, WellKnownRootPath, maxWellKnownBodyBytes)
	if err != nil {
		return WellKnownRoot{}, err
	}
	var root WellKnownRoot
	if err := json.Unmarshal(body, &root); err != nil {
		return WellKnownRoot{}, fmt.Errorf("%s: %w", errDecode, err)
	}
	return root, nil
}

// FetchDirectory GETs the well-known directory and returns the RAW byte-exact
// signed-registry artifact (the bytes the root signature covers). A 404 returns
// ErrNotConfigured.
func (c *Client) FetchDirectory(ctx context.Context) ([]byte, error) {
	return c.get(ctx, WellKnownDirectoryPath, maxDirectoryBodyBytes)
}

// Whoami probes /whoami. It is a LIVENESS check: a 200 proves only that
// something listens (the returned agent is the DAEMON identity, never the member
// slug). It returns (reachable, agent, err); a transport failure yields
// (false, "", nil) — an unreachable daemon is an INFO state, not a hard error.
func (c *Client) Whoami(ctx context.Context) (bool, string, error) {
	body, err := c.get(ctx, WhoamiPath, maxWellKnownBodyBytes)
	if err != nil {
		// A 404 still means something answered (reachable); any other error is a
		// genuine transport failure (not reachable). Either way Whoami is a
		// liveness probe — it never propagates the error to its caller.
		return errors.Is(err, ErrNotConfigured), "", nil
	}
	// Reachable: a parse failure leaves the agent empty but still reachable (a
	// live listener served a body we could not decode).
	var wr whoamiResponse
	_ = json.Unmarshal(body, &wr)
	return true, wr.Agent, nil
}

// get resolves+loopback-checks the host, pins the resolved IP, performs the GET
// under an inescapable deadline, refuses redirects, and returns the capped body.
// A 404 maps to ErrNotConfigured; any other non-200 is an error.
func (c *Client) get(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	if err := c.ensureLoopbackPin(ctx); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := c.baseURL.JoinPath(path).String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errBuildRequest, err)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errFetch, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotConfigured
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %d", errStatus, resp.StatusCode)
	}
	return readCapped(resp.Body, maxBytes)
}

// ensureLoopbackPin resolves the daemon host and refuses unless EVERY resolved
// IP is loopback. It pins the first loopback IP for the dial (DNS-rebind
// defense). When the host is already a literal IP, it is checked directly.
func (c *Client) ensureLoopbackPin(ctx context.Context) error {
	if ip := net.ParseIP(c.host); ip != nil {
		if !ip.IsLoopback() {
			return fmt.Errorf("%s", errNonLoopback)
		}
		c.pinnedIP = ip
		return nil
	}
	ips, err := c.resolve(ctx, c.host)
	if err != nil {
		return fmt.Errorf("%s: %w", errResolve, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("%s", errNonLoopback)
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return fmt.Errorf("%s", errNonLoopback)
		}
	}
	c.pinnedIP = ips[0]
	return nil
}

// httpClient builds the hardened *http.Client: a DialContext that pins the
// resolved loopback IP (rewriting the dial address to the pinned IP, defeating a
// rebind), CheckRedirect that refuses all redirects, and the inescapable Timeout
// is provided by the per-fetch context (Timeout here is a belt-and-suspenders).
func (c *Client) httpClient() *http.Client {
	dialer := &net.Dialer{Timeout: c.timeout}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			// Pin to the resolved loopback IP regardless of the dialed host.
			pinned := net.JoinHostPort(c.pinnedIP.String(), port)
			return dialer.DialContext(ctx, network, pinned)
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   c.timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New(errRedirect)
		},
	}
}

// readCapped reads body up to maxBytes; an over-cap body (one more byte than
// the cap) fails LOUD with errBodyTooLarge rather than silently truncating.
func readCapped(body io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errReadBody, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%s (> %d bytes)", errBodyTooLarge, maxBytes)
	}
	return data, nil
}
