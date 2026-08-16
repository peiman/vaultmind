package release

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stubProxy(t *testing.T, version string, status int) (calls *int) {
	t.Helper()
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n++
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_ = json.NewEncoder(w).Encode(proxyResponse{Version: version})
	}))
	t.Cleanup(srv.Close)

	prev := proxyEndpointOverride
	proxyEndpointOverride = srv.URL
	t.Cleanup(func() { proxyEndpointOverride = prev })
	return &n
}

func TestCheck_ReportsANewerRelease(t *testing.T) {
	stubProxy(t, "v0.6.0", http.StatusOK)

	info, ok := Check(context.Background(), "v0.5.0", t.TempDir())
	require.True(t, ok)
	assert.True(t, info.Newer)
	assert.Equal(t, "v0.6.0", info.Latest)
}

func TestCheck_UpToDateIsNotNewer(t *testing.T) {
	stubProxy(t, "v0.5.0", http.StatusOK)

	info, ok := Check(context.Background(), "v0.5.0", t.TempDir())
	require.True(t, ok)
	assert.False(t, info.Newer, "running the latest is not an update notice")
}

// Running AHEAD of the proxy is normal right after a tag, while the module
// index catches up. Telling someone to upgrade to an older version would be
// worse than saying nothing.
func TestCheck_AheadOfTheProxyIsNotNewer(t *testing.T) {
	stubProxy(t, "v0.4.2", http.StatusOK)

	info, ok := Check(context.Background(), "v0.5.0", t.TempDir())
	require.True(t, ok)
	assert.False(t, info.Newer)
}

// A CLI that reaches the network with no way to stop it is not something a
// privacy-minded user can keep.
func TestCheck_OptOutMakesNoRequest(t *testing.T) {
	calls := stubProxy(t, "v9.9.9", http.StatusOK)
	t.Setenv(DisableEnv, "1")

	_, ok := Check(context.Background(), "v0.5.0", t.TempDir())
	assert.False(t, ok)
	assert.Zero(t, *calls, "opting out must not touch the network at all")
}

// A version with no semver at all has nothing to compare against, and its
// operator is the one person who certainly knows what they are running.
func TestCheck_UnversionedBuildIsSkipped(t *testing.T) {
	calls := stubProxy(t, "v9.9.9", http.StatusOK)

	for _, v := range []string{"", "dev", "vdev", "unknown"} {
		_, ok := Check(context.Background(), v, t.TempDir())
		assert.Falsef(t, ok, "%q carries no version to compare", v)
	}
	assert.Zero(t, *calls, "an unversioned build must not reach the network")
}

// A git-describe build (`v0.3.0-15-gabc123`, what a local `task build` stamps)
// IS checked, and deliberately so: the semver prefix is real information. Such a
// build is genuinely 15 commits past v0.3.0, so if v0.5.0 exists it is behind
// and saying so is useful rather than noise.
func TestCheck_GitDescribeBuildIsComparedOnItsSemverPrefix(t *testing.T) {
	stubProxy(t, "v0.5.0", http.StatusOK)

	info, ok := Check(context.Background(), "v0.3.0-15-gabc123", t.TempDir())
	require.True(t, ok)
	assert.True(t, info.Newer, "a build 15 commits past v0.3.0 is behind v0.5.0")
}

// The same shape built from main AFTER the latest tag must NOT be told to
// upgrade to the version it already contains.
func TestCheck_GitDescribeAheadOfLatestIsNotNewer(t *testing.T) {
	stubProxy(t, "v0.5.0", http.StatusOK)

	info, ok := Check(context.Background(), "v0.5.0-3-gdef456", t.TempDir())
	require.True(t, ok)
	assert.False(t, info.Newer, "a build past v0.5.0 already has v0.5.0")
}

// An unreachable or broken proxy is a silent skip. A health command that fails
// because a network was down would be worse than not having the check.
func TestCheck_ProxyFailureIsSilent(t *testing.T) {
	stubProxy(t, "", http.StatusInternalServerError)

	_, ok := Check(context.Background(), "v0.5.0", t.TempDir())
	assert.False(t, ok, "a proxy error is not an error the user has to see")
}

// The second call in a day must not hit the network again — repeated doctor runs
// are normal and should stay local.
func TestCheck_SecondCallUsesTheCache(t *testing.T) {
	calls := stubProxy(t, "v0.6.0", http.StatusOK)
	cacheDir := t.TempDir()

	_, ok := Check(context.Background(), "v0.5.0", cacheDir)
	require.True(t, ok)
	_, ok = Check(context.Background(), "v0.5.0", cacheDir)
	require.True(t, ok)

	assert.Equal(t, 1, *calls, "the cached answer serves the second call")
}

// A stale cache must be refreshed, or a check that ran once would report the
// same answer forever — the exact staleness this feature exists to end.
func TestCheck_StaleCacheIsRefreshed(t *testing.T) {
	calls := stubProxy(t, "v0.7.0", http.StatusOK)
	cacheDir := t.TempDir()

	stale := Info{Latest: "v0.6.0", CheckedAt: time.Now().Add(-48 * time.Hour)}
	body, err := json.Marshal(stale)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "latest-version.json"), body, 0o600))

	info, ok := Check(context.Background(), "v0.5.0", cacheDir)
	require.True(t, ok)
	assert.Equal(t, "v0.7.0", info.Latest)
	assert.Equal(t, 1, *calls)
}

// A corrupt cache must not be fatal, and must not pin a wrong answer.
func TestCheck_CorruptCacheFallsBackToTheNetwork(t *testing.T) {
	stubProxy(t, "v0.6.0", http.StatusOK)
	cacheDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "latest-version.json"),
		[]byte("{not json"), 0o600))

	info, ok := Check(context.Background(), "v0.5.0", cacheDir)
	require.True(t, ok)
	assert.Equal(t, "v0.6.0", info.Latest)
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v0.5.0", "v0.6.0", true},
		{"v0.5.0", "v0.5.1", true},
		{"v0.5.0", "v1.0.0", true},
		{"v0.5.0", "v0.5.0", false},
		{"v0.5.0", "v0.4.9", false},
		{"v0.10.0", "v0.9.0", false}, // numeric, not lexical — v0.10 > v0.9
		{"v0.9.0", "v0.10.0", true},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, isNewer(c.current, c.latest), "%s -> %s", c.current, c.latest)
	}
}
