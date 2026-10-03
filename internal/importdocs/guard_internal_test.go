package importdocs

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockedAddr_RefusesEveryNonPublicRange(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "127.255.0.9", "::1", // loopback
		"10.1.2.3", "172.16.0.1", "172.31.255.255", "192.168.1.1", // RFC 1918
		"fd00::1", "fc12::1", // unique local
		"169.254.169.254", "fe80::1", // link-local, the cloud metadata address among them
		"0.0.0.0", "0.1.2.3", "::", // unspecified and "this network"
		"224.0.0.1", "ff02::1", // multicast
		"100.64.0.1", "100.127.255.254", // CGNAT, Tailscale's range
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", // IPv4-mapped
		"64:ff9b::7f00:1", "64:ff9b::a9fe:a9fe", // NAT64 carrying 127.0.0.1 and 169.254.169.254
		"2002:a00:1::1", "2002:7f00:1::", // 6to4 carrying 10.0.0.1 and 127.0.0.1
		"2001:0:4136:e378::1", // Teredo: its IPv4 is obfuscated, so it is refused whole
		"64:ff9b:1::1",        // local-use NAT64
	}
	public := []string{"93.184.216.34", "8.8.8.8", "100.128.0.1", "172.32.0.1", "2606:4700::1111",
		"64:ff9b::808:808", "2002:808:808::1"} // NAT64 and 6to4 carrying 8.8.8.8
	var errs []error
	for _, s := range blocked {
		if !blockedAddr(net.ParseIP(s)) {
			errs = append(errs, errors.New(s+" should be blocked"))
		}
	}
	for _, s := range public {
		if blockedAddr(net.ParseIP(s)) {
			errs = append(errs, errors.New(s+" should be allowed"))
		}
	}
	assert.NoError(t, errors.Join(errs...))
}

func TestStartAllowsPrivate_WhenTheOperatorNamedAPrivateHost(t *testing.T) {
	prev := lookupIP
	t.Cleanup(func() { lookupIP = prev })
	lookupIP = func(_ context.Context, host string) ([]net.IP, error) {
		switch host {
		case "docs.internal":
			return []net.IP{net.ParseIP("10.0.0.5")}, nil
		case "example.com":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		}
		return nil, errors.New("no such host")
	}
	got := map[string]bool{}
	for _, u := range []string{
		"http://localhost:8000/docs/", "http://127.0.0.1/", "http://[::1]:3000/",
		"http://docs.internal/guide", "https://example.com/docs", "https://nowhere.invalid/",
	} {
		got[u] = startAllowsPrivate(context.Background(), u)
	}
	assert.Equal(t, map[string]bool{
		"http://localhost:8000/docs/": true, "http://127.0.0.1/": true, "http://[::1]:3000/": true,
		"http://docs.internal/guide": true, "https://example.com/docs": false, "https://nowhere.invalid/": false,
	}, got)
}

// A public start URL: every connection to a private address is refused at
// dial time, including a redirect's.
func TestHTTPFetcher_APublicStartCannotReachAPrivateAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("internal secret\n"))
	}))
	t.Cleanup(srv.Close)
	prev := lookupIP
	t.Cleanup(func() { lookupIP = prev })
	lookupIP = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}

	_, err := HTTPFetcher("https://example.com/docs/")(t.Context(), srv.URL+"/x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to connect to 127.0.0.1")

	lookupIP = prev
	page, err := HTTPFetcher(srv.URL)(t.Context(), srv.URL+"/x")
	require.NoError(t, err, "a private start URL was the operator's choice")
	assert.Equal(t, "internal secret\n", string(page.Body))
}

// A proxy from the environment dials the destination itself, so the dial
// check only ever sees the proxy. The destination is checked before the
// request goes to the proxy.
func TestGuardedProxy_ChecksTheDestinationNotJustTheProxy(t *testing.T) {
	prevProxy, prevLookup := envProxy, lookupIP
	t.Cleanup(func() { envProxy, lookupIP = prevProxy, prevLookup })
	envProxy = func(*http.Request) (*url.URL, error) { return url.Parse("http://proxy.example:3128") }
	lookupIP = func(_ context.Context, host string) ([]net.IP, error) {
		if host == "intranet.example" {
			return []net.IP{net.ParseIP("10.1.2.3")}, nil
		}
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	req := func(u string) *http.Request {
		r, err := http.NewRequest(http.MethodGet, u, nil)
		require.NoError(t, err)
		return r
	}

	_, err := guardedProxy(false)(req("http://intranet.example/wiki"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "10.1.2.3")
	_, err = guardedProxy(false)(req("http://169.254.169.254/latest"))
	require.Error(t, err)

	p, err := guardedProxy(false)(req("https://docs.example.com/"))
	require.NoError(t, err)
	assert.Equal(t, "proxy.example:3128", p.Host)
	assert.True(t, proxyAddrs.has("93.184.216.34:3128"), "the dial check lets the connection to the proxy through")
	assert.False(t, proxyAddrs.has("10.1.2.3:3128"))
	p, err = guardedProxy(true)(req("http://intranet.example/wiki"))
	require.NoError(t, err, "a private start URL allows private destinations")
	assert.NotNil(t, p)
}
