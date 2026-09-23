package relayclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// WellKnownDirectoryPath serves the hub's signed registry, byte-exact.
const WellKnownDirectoryPath = "/.well-known/vaultmind-directory"

// MaxDirectoryBytes caps the registry body. A real registry is a few KB; the cap
// keeps a hostile or broken hub from exhausting memory.
const MaxDirectoryBytes = 1 << 20

const (
	errDirFetch   = "relayclient: fetch signed registry"
	errDirStatus  = "relayclient: signed registry returned non-200"
	errDirTooBig  = "relayclient: signed registry exceeds the size cap"
	errDirReadErr = "relayclient: read signed registry"
)

// FetchDirectory GETs {baseURL}/.well-known/vaultmind-directory and returns the
// body byte-exact. Remote-permissive, like FetchRoot (the hub is remote), with
// the same guards: http(s) only, its own deadline, fail closed on non-200.
//
// The bytes are UNTRUSTED. The caller verifies the root signature against a
// PINNED root before relying on them; the channel proves nothing.
//
// Over the cap is an error, not a truncation: a cut-short registry would fail
// signature verification for a misleading reason.
func FetchDirectory(ctx context.Context, client *http.Client, baseURL string) ([]byte, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != schemeHTTP && u.Scheme != schemeHTTPS) || u.Host == "" {
		return nil, fmt.Errorf("%s", errBadScheme)
	}
	endpoint := u.JoinPath(WellKnownDirectoryPath).String()
	if client == nil {
		client = &http.Client{}
	}
	ctx, cancel := context.WithTimeout(ctx, relayFetchTimeout)
	defer cancel()

	//nolint:gosec // hub URL is operator-declared (agents.yaml daemon_url); scheme validated above
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errBuildRequest, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errDirFetch, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %d", errDirStatus, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxDirectoryBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errDirReadErr, err)
	}
	if len(body) > MaxDirectoryBytes {
		return nil, fmt.Errorf("%s (%d bytes)", errDirTooBig, MaxDirectoryBytes)
	}
	return body, nil
}
