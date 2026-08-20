// Package release answers one question: is there a newer VaultMind than the
// one running?
//
// Nothing told anyone that. A release could ship a fix for a silent failure and
// the people it affected would keep running the broken version indefinitely,
// because the only way to find out was to go and look. That is the same shape as
// hook drift — a real difference that no surface reports.
package release

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProxyURL is the Go module proxy's "latest version" endpoint. Deliberately the
// proxy rather than the GitHub API: it needs no auth, is not rate-limited per
// IP, and is the exact source `go install …@latest` resolves against — so this
// check answers "what would an install get?" rather than "what did someone tag?".
const ProxyURL = "https://proxy.golang.org/github.com/peiman/vaultmind/@latest"

// DisableEnv opts out entirely. A CLI that reaches the network without a way to
// stop it is not something a privacy-minded user can keep.
const DisableEnv = "VAULTMIND_NO_UPDATE_CHECK"

// cacheTTL bounds how often the network is touched. A day is well inside the
// useful window for "a release happened" and keeps repeated doctor runs local.
const cacheTTL = 24 * time.Hour

// httpTimeout keeps a hung proxy from holding up a health command. On timeout
// the check is skipped, not failed — doctor's job is the vault, not the network.
const httpTimeout = 3 * time.Second

// Info is the outcome of a check.
type Info struct {
	Current   string    `json:"current"`
	Latest    string    `json:"latest"`
	Newer     bool      `json:"newer"`
	CheckedAt time.Time `json:"checked_at"`
}

type proxyResponse struct {
	Version string `json:"Version"`
}

// Check reports whether a newer version than current exists.
//
// The bool is "did we get an answer" — false covers opted out, no network, a
// dev build, a cache miss that could not be filled. Every one of those is a
// silent skip: an update notice is a courtesy, and a health command that fails
// because a proxy was unreachable would be worse than not having it.
func Check(ctx context.Context, current, cacheDir string) (Info, bool) {
	if os.Getenv(DisableEnv) != "" {
		return Info{}, false
	}
	// A dev build has no meaningful version to compare — reporting "v0.5.0 is
	// available" to someone running their own build is noise, and they are the
	// one person who certainly knows what they are running.
	if !isReleaseVersion(current) {
		return Info{}, false
	}

	if cached, ok := readCache(cacheDir); ok {
		cached.Current = current
		cached.Newer = isNewer(current, cached.Latest)
		return cached, true
	}

	latest, err := fetchLatest(ctx)
	if err != nil {
		return Info{}, false
	}
	info := Info{
		Current:   current,
		Latest:    latest,
		Newer:     isNewer(current, latest),
		CheckedAt: time.Now().UTC(),
	}
	writeCache(cacheDir, info) // best effort; a cache miss costs one request
	return info, true
}

// isReleaseVersion reports whether v looks like a tagged release. Goreleaser
// stamps a semver tag; a local `task build` stamps something else (or nothing).
func isReleaseVersion(v string) bool {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "v") || len(v) < 2 {
		return false
	}
	// A release tag is v<digit>…; "vdev" and "" are not.
	return v[1] >= '0' && v[1] <= '9'
}

// isNewer compares two semver-ish tags by segment. Deliberately not a full
// semver implementation: this decides whether to print one advisory line, and a
// wrong answer on a prerelease suffix costs a redundant notice, not correctness.
func isNewer(current, latest string) bool {
	c := splitVersion(current)
	l := splitVersion(latest)
	// Range the array being indexed rather than the literal 3. Both give exactly
	// three iterations — splitVersion returns [3]int, so the bound was already in
	// the type — but gosec does not model Go 1.22's range-over-int as bounding the
	// index and reported G602 here. Ranging l makes the index valid by
	// construction instead of by an argument the analyzer cannot follow, which is
	// a better answer than a #nosec that suppresses the question.
	for i := range l {
		switch {
		case l[i] > c[i]:
			return true
		case l[i] < c[i]:
			return false
		}
	}
	return false
}

func splitVersion(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	for i, part := range strings.SplitN(v, ".", 3) {
		if i > 2 {
			break
		}
		n := 0
		for _, r := range part {
			if r < '0' || r > '9' {
				n = 0
				break
			}
			n = n*10 + int(r-'0')
		}
		out[i] = n
	}
	return out
}

func fetchLatest(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyEndpoint(), nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("proxy returned %d", resp.StatusCode)
	}
	var pr proxyResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return "", err
	}
	if pr.Version == "" {
		return "", fmt.Errorf("proxy returned no version")
	}
	return pr.Version, nil
}

// proxyEndpoint allows tests to point at a local server instead of reaching the
// real proxy — a test suite that needs the network is a test suite that fails on
// a plane.
var proxyEndpointOverride string

func proxyEndpoint() string {
	if proxyEndpointOverride != "" {
		return proxyEndpointOverride
	}
	return ProxyURL
}

func cachePath(cacheDir string) string {
	return filepath.Join(cacheDir, "latest-version.json")
}

func readCache(cacheDir string) (Info, bool) {
	if cacheDir == "" {
		return Info{}, false
	}
	body, err := os.ReadFile(cachePath(cacheDir)) // #nosec G304 -- path is the app's own XDG cache dir
	if err != nil {
		return Info{}, false
	}
	var info Info
	if err := json.Unmarshal(body, &info); err != nil {
		return Info{}, false
	}
	if time.Since(info.CheckedAt) > cacheTTL {
		return Info{}, false
	}
	return info, true
}

func writeCache(cacheDir string, info Info) {
	if cacheDir == "" {
		return
	}
	body, err := json.Marshal(info)
	if err != nil {
		return
	}
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		return
	}
	_ = os.WriteFile(cachePath(cacheDir), body, 0o600)
}
