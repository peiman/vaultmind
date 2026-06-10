// Package relayclient fetches a Contract-B relay's PUBLIC trust anchor from its
// /.well-known/vaultmind-root endpoint. It performs NO trust decisions itself —
// the caller (the member-enroll flow) cross-checks the fetched root against the
// out-of-band-confirmed invite. This package only does the transport: validate
// the scheme, GET the well-known path, and decode the response.
package relayclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// WellKnownRootPath is the canonical path a relay serves its public root anchor
// from. SSOT so the fetch path and any docs reference one definition.
const WellKnownRootPath = "/.well-known/vaultmind-root"

// maxRootBodyBytes caps the well-known root response body before decode. The
// legit payload is ~150 B; 64 KiB is generous headroom while bounding a hostile
// or runaway relay so a no-cap decode can never OOM the keyless CLI. A body that
// hits the cap fails LOUD (a decode error), never silently truncated.
const maxRootBodyBytes = 64 << 10

// relayFetchTimeout bounds EVERY well-known fetch: FetchRoot wraps the request in
// a context.WithTimeout(relayFetchTimeout) of its own, so even a caller-supplied
// no-timeout *http.Client cannot hang the fetch. It is a var (not a const) ONLY so
// a test can shorten it via SetFetchTimeoutForTest; production never reassigns it.
var relayFetchTimeout = 15 * time.Second

// Error strings (SSOT). Each failure mode is a distinct typed reject so the
// enroll flow can tell a misconfigured base URL from a dead relay.
const (
	// errBadScheme is returned when the relay base URL is not parseable or its
	// scheme is not http/https. The well-known fetch refuses any other scheme so
	// a malicious invite cannot point it at file:// or similar.
	errBadScheme = "relayclient: relay base URL must be an http(s) URL"
	// errBuildRequest wraps a request-construction failure (should not happen for
	// a validated URL).
	errBuildRequest = "relayclient: build well-known request"
	// errFetch wraps a transport failure (dial/timeout/cancelled context).
	errFetch = "relayclient: fetch well-known root"
	// errStatus is returned (with the status code) on a non-200 response.
	errStatus = "relayclient: well-known root returned non-200"
	// errDecode wraps a JSON-decode failure of the well-known body.
	errDecode = "relayclient: decode well-known root JSON"
)

// schemeHTTP and schemeHTTPS are the only relay schemes the fetch permits.
const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// WellKnownRoot is the relay's advertised PUBLIC trust anchor. It is UNTRUSTED
// until the caller cross-checks it against the out-of-band-confirmed invite:
// the relay could be a MITM, so RootPubKey/NetworkID here prove nothing on their
// own.
type WellKnownRoot struct {
	// RootPubKey is base64-std (padded) of the 32-byte ed25519 ROOT public key the
	// relay claims anchors its network.
	RootPubKey string `json:"root_pubkey"`
	// NetworkID is the relay's claimed "vmnet1:…" id. The caller re-derives
	// NetworkID(root_pubkey) and rejects a mismatch.
	NetworkID string `json:"network_id"`
	// RootKeyEpoch is the relay's advertised root-key rotation epoch (optional;
	// 0 when absent). It is informational here — the trust decision is the
	// pubkey/network-id cross-check, not the epoch.
	RootKeyEpoch int64 `json:"root_key_epoch,omitempty"`
}

// FetchRoot GETs {relayBaseURL}/.well-known/vaultmind-root and decodes the
// WellKnownRoot. It validates the base URL scheme (http/https only) BEFORE
// dialing, wraps the request in its OWN context.WithTimeout(relayFetchTimeout) so
// a caller-supplied no-timeout client can never hang it, CAPS the response body
// at maxRootBodyBytes before decode, and FAILS CLOSED on a non-200 status or a
// malformed/over-cap body. It deliberately does NOT DisallowUnknownFields so the
// relay can add forward-compatible fields without breaking older members.
//
// This client is intentionally remote-permissive (enroll fetches REMOTE relays):
// it does NOT loopback-pin — that is a separate doctor-only client in a later
// slice. client may be nil; a default client is used in that case. The returned
// WellKnownRoot is UNTRUSTED — the caller must cross-check it against the invite
// before relying on it.
func FetchRoot(ctx context.Context, client *http.Client, relayBaseURL string) (WellKnownRoot, error) {
	endpoint, err := wellKnownURL(relayBaseURL)
	if err != nil {
		return WellKnownRoot{}, err
	}
	if client == nil {
		client = &http.Client{}
	}

	// Inescapable deadline: bound the request inside FetchRoot so a caller-supplied
	// *http.Client with no Timeout (and a context with no deadline) still cannot
	// hang the fetch.
	ctx, cancel := context.WithTimeout(ctx, relayFetchTimeout)
	defer cancel()

	//nolint:gosec // relay base URL is operator-provided via the signed invite; scheme validated above
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return WellKnownRoot{}, fmt.Errorf("%s: %w", errBuildRequest, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return WellKnownRoot{}, fmt.Errorf("%s: %w", errFetch, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return WellKnownRoot{}, fmt.Errorf("%s: %d", errStatus, resp.StatusCode)
	}

	// Cap the body before decode: a runaway/hostile relay cannot OOM the CLI. An
	// over-cap body surfaces as a decode error (the JSON value is cut short), which
	// fails loud rather than silently truncating.
	var root WellKnownRoot
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRootBodyBytes)).Decode(&root); err != nil {
		return WellKnownRoot{}, fmt.Errorf("%s: %w", errDecode, err)
	}
	return root, nil
}

// wellKnownURL validates the relay base URL scheme and joins the well-known
// path onto it. It rejects any non-http(s) scheme (and unparseable input) so the
// fetch can never be redirected to a non-HTTP scheme by a hostile invite.
func wellKnownURL(relayBaseURL string) (string, error) {
	u, err := url.Parse(relayBaseURL)
	if err != nil || (u.Scheme != schemeHTTP && u.Scheme != schemeHTTPS) || u.Host == "" {
		return "", fmt.Errorf("%s", errBadScheme)
	}
	return u.JoinPath(WellKnownRootPath).String(), nil
}
