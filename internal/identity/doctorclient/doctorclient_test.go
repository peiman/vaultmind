package doctorclient

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stubResolver records the host it is asked to resolve and returns a fixed set
// of IPs, so a test can drive the loopback-allowlist check without real DNS.
type stubResolver struct {
	ips  []net.IP
	host string
}

func (s *stubResolver) lookup(_ context.Context, host string) ([]net.IP, error) {
	s.host = host
	return s.ips, nil
}

func loopbackResolver(t *testing.T) resolverFunc {
	t.Helper()
	r := &stubResolver{ips: []net.IP{net.ParseIP("127.0.0.1")}}
	return r.lookup
}

func TestNew_RejectsNonHTTPScheme(t *testing.T) {
	_, err := New("file:///etc/passwd", WithResolver(loopbackResolver(t)))
	require.Error(t, err)
	require.Contains(t, err.Error(), errBadScheme)
}

func TestNew_RejectsEmptyHost(t *testing.T) {
	_, err := New("http://", WithResolver(loopbackResolver(t)))
	require.Error(t, err)
	require.Contains(t, err.Error(), errBadScheme)
}

func TestFetchRoot_LoopbackAccepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"root_pubkey":"AAAA","network_id":"vmnet1:abcd","root_key_epoch":1}`))
	}))
	defer srv.Close()

	// httptest binds 127.0.0.1; pin the resolver to loopback explicitly so the
	// allowlist accepts it deterministically (independent of system resolver).
	c, err := New(srv.URL, WithResolver(loopbackResolver(t)))
	require.NoError(t, err)

	root, err := c.FetchRoot(context.Background())
	require.NoError(t, err)
	require.Equal(t, "vmnet1:abcd", root.NetworkID)
	require.Equal(t, "AAAA", root.RootPubKey)
	require.EqualValues(t, 1, root.RootKeyEpoch)
}

func TestFetchRoot_NonLoopbackResolveRejected(t *testing.T) {
	// A resolver that maps the daemon host to a PUBLIC IP must be rejected
	// fail-closed BEFORE any dial — this is the SSRF/rebind defense.
	r := &stubResolver{ips: []net.IP{net.ParseIP("203.0.113.5")}}
	c, err := New("http://daemon.local:7777", WithResolver(r.lookup))
	require.NoError(t, err)

	_, err = c.FetchRoot(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), errNonLoopback)
}

func TestFetchRoot_MixedResolveRejected(t *testing.T) {
	// If ANY resolved IP is non-loopback, the allowlist fails closed.
	r := &stubResolver{ips: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("10.0.0.1")}}
	c, err := New("http://daemon.local:7777", WithResolver(r.lookup))
	require.NoError(t, err)

	_, err = c.FetchRoot(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), errNonLoopback)
}

func TestFetchRoot_EmptyResolveRejected(t *testing.T) {
	r := &stubResolver{ips: []net.IP{}}
	c, err := New("http://daemon.local:7777", WithResolver(r.lookup))
	require.NoError(t, err)

	_, err = c.FetchRoot(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), errNonLoopback)
}

func TestFetchRoot_RedirectRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, "http://127.0.0.1:9/evil", http.StatusFound)
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithResolver(loopbackResolver(t)))
	require.NoError(t, err)

	_, err = c.FetchRoot(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), errRedirect)
}

func TestFetchRoot_OversizedBodyRejected(t *testing.T) {
	// Body far larger than the 64 KiB root cap fails LOUD, never silent-truncates.
	huge := strings.Repeat("A", int(maxWellKnownBodyBytes)+1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"network_id":"vmnet1:abcd","root_pubkey":"` + huge + `"}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithResolver(loopbackResolver(t)))
	require.NoError(t, err)

	_, err = c.FetchRoot(context.Background())
	require.Error(t, err)
}

func TestFetchRoot_Non200Rejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithResolver(loopbackResolver(t)))
	require.NoError(t, err)

	_, err = c.FetchRoot(context.Background())
	require.ErrorIs(t, err, ErrNotConfigured)
}

func TestFetchDirectory_ReturnsRawBytes(t *testing.T) {
	raw := []byte(`{"registry":"AAAA","root_sig":"BBBB"}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithResolver(loopbackResolver(t)))
	require.NoError(t, err)

	got, err := c.FetchDirectory(context.Background())
	require.NoError(t, err)
	require.Equal(t, raw, got)
}

func TestFetchDirectory_OversizedBodyRejected(t *testing.T) {
	huge := strings.Repeat("A", int(maxDirectoryBodyBytes)+1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(huge))
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithResolver(loopbackResolver(t)))
	require.NoError(t, err)

	_, err = c.FetchDirectory(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), errBodyTooLarge)
}

func TestFetchDirectory_Non200IsNotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithResolver(loopbackResolver(t)))
	require.NoError(t, err)

	_, err = c.FetchDirectory(context.Background())
	require.ErrorIs(t, err, ErrNotConfigured)
}

func TestWhoami_Reachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agent":"agent:mira"}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithResolver(loopbackResolver(t)))
	require.NoError(t, err)

	reachable, agent, err := c.Whoami(context.Background())
	require.NoError(t, err)
	require.True(t, reachable)
	require.Equal(t, "agent:mira", agent)
}

func TestWhoami_UnreachableNoServer(t *testing.T) {
	// Point at a closed loopback port — Whoami reports not-reachable, no error
	// is required to be non-nil (it is a liveness probe), but reachable=false.
	c, err := New("http://127.0.0.1:1", WithResolver(loopbackResolver(t)))
	require.NoError(t, err)

	reachable, _, _ := c.Whoami(context.Background())
	require.False(t, reachable)
}

func TestFetchRoot_DefaultResolverLiteralLoopback(t *testing.T) {
	// No WithResolver: exercise the production literal-IP loopback path + the
	// default resolver code path (httptest binds a literal 127.0.0.1 URL).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"root_pubkey":"AAAA","network_id":"vmnet1:lit"}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	require.NoError(t, err)
	root, err := c.FetchRoot(context.Background())
	require.NoError(t, err)
	require.Equal(t, "vmnet1:lit", root.NetworkID)
}

func TestDefaultResolve_LocalhostIsLoopback(t *testing.T) {
	// Exercise the PRODUCTION resolver directly: localhost must resolve to at
	// least one loopback IP on every CI host, and every returned IP must be
	// loopback (so the allowlist would admit it).
	ips, err := defaultResolve(context.Background(), "localhost")
	require.NoError(t, err)
	require.NotEmpty(t, ips)
	for _, ip := range ips {
		require.True(t, ip.IsLoopback(), "localhost resolved a non-loopback IP: %s", ip)
	}
}

func TestFetchRoot_LiteralNonLoopbackRejected(t *testing.T) {
	// A literal public IP host is rejected without any resolver call.
	c, err := New("http://203.0.113.9:7777")
	require.NoError(t, err)
	_, err = c.FetchRoot(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), errNonLoopback)
}

func TestFetchRoot_ResolverErrorSurfaced(t *testing.T) {
	failing := func(_ context.Context, _ string) ([]net.IP, error) {
		return nil, errResolveFailure
	}
	c, err := New("http://daemon.local:7777", WithResolver(failing))
	require.NoError(t, err)
	_, err = c.FetchRoot(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), errResolve)
}

func TestFetchRoot_UnexpectedStatusSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c, err := New(srv.URL, WithResolver(loopbackResolver(t)))
	require.NoError(t, err)
	_, err = c.FetchRoot(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), errStatus)
}

func TestReadCapped_ExactlyAtCapAccepted(t *testing.T) {
	body := strings.NewReader(strings.Repeat("A", 10))
	got, err := readCapped(body, 10)
	require.NoError(t, err)
	require.Len(t, got, 10)
}

// errResolveFailure is a sentinel resolver error for the resolver-error test.
var errResolveFailure = &resolverTestError{}

type resolverTestError struct{}

func (*resolverTestError) Error() string { return "stub resolver failure" }

func TestFetch_DeadlineBoundsNoTimeoutCaller(t *testing.T) {
	// A handler that hangs longer than the client's internal deadline must not
	// hang the fetch: the inescapable context.WithTimeout bounds it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithResolver(loopbackResolver(t)), WithTimeout(150*time.Millisecond))
	require.NoError(t, err)

	start := time.Now()
	_, err = c.FetchRoot(context.Background())
	require.Error(t, err)
	require.Less(t, time.Since(start), 1500*time.Millisecond, "deadline must bound a slow daemon")
}
