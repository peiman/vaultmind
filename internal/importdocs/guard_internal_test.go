package importdocs

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
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
	}
	public := []string{"93.184.216.34", "8.8.8.8", "100.128.0.1", "172.32.0.1", "2606:4700::1111"}
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
