package importdocs

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"syscall"
	"time"
)

// nonPublic are the ranges net.IP's own predicates miss: "this network"
// (0.0.0.0/8, which reaches the local host on Linux), CGNAT (100.64.0.0/10,
// where Tailscale and carrier NAT hand out addresses), Teredo
// (2001::/32, whose IPv4 is obfuscated and cannot be judged) and local-use
// NAT64 (64:ff9b:1::/48, which may map to anything).
var nonPublic = []*net.IPNet{
	mustCIDR("0.0.0.0/8"),
	mustCIDR("100.64.0.0/10"),
	mustCIDR("2001::/32"),
	mustCIDR("64:ff9b:1::/48"),
}

// Transition prefixes that carry an IPv4 address in the clear: the address
// is judged, so 64:ff9b::127.0.0.1 is loopback and 64:ff9b::8.8.8.8 is not.
var (
	nat64     = mustCIDR("64:ff9b::/96") // RFC 6052: the IPv4 is the last 4 bytes
	sixToFour = mustCIDR("2002::/16")    // RFC 3056: the IPv4 is bytes 2–5
)

// embeddedV4 is the IPv4 address a NAT64 or 6to4 address carries, or nil.
func embeddedV4(ip net.IP) net.IP {
	switch {
	case nat64.Contains(ip):
		return ip[12:16]
	case sixToFour.Contains(ip):
		return ip[2:6]
	}
	return nil
}

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// blockedAddr reports an address a public page must not make us reach:
// loopback, private, link-local (the cloud metadata address among them),
// unspecified, multicast, "this network" and CGNAT. An IPv6 address that
// carries an IPv4 one (mapped, NAT64, 6to4) is judged by that address.
func blockedAddr(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	} else if v4 := embeddedV4(ip.To16()); v4 != nil {
		return blockedAddr(v4)
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, n := range nonPublic {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// lookupIP resolves a host. A test replaces it.
var lookupIP = func(ctx context.Context, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

// startAllowsPrivate reports whether the operator's own URL names a
// non-public address: then they chose it (docs served on localhost, an
// intranet wiki), and the run may reach such addresses. A host that does not
// resolve allows nothing; the fetch fails on its own.
func startAllowsPrivate(ctx context.Context, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		return blockedAddr(ip)
	}
	if host == "localhost" {
		return true
	}
	ips, err := lookupIP(ctx, host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if blockedAddr(ip) {
			return true
		}
	}
	return false
}

// guardedTransport dials only public addresses unless allowPrivate. The
// check runs on the address actually dialed, after DNS, so it covers every
// redirect hop and a host whose DNS answer points inside. Through a proxy
// the destination is dialed by the proxy, so guardedProxy checks it first;
// the proxy itself is the operator's setting and may be private.
func guardedTransport(allowPrivate bool) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   pageTimeout,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if allowPrivate {
				return nil
			}
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if blockedAddr(net.ParseIP(host)) && !proxyAddrs.has(net.JoinHostPort(host, port)) {
				return fmt.Errorf("refusing to connect to %s: a private address, and the URL you named is public", host)
			}
			return nil
		},
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = dialer.DialContext
	t.Proxy = guardedProxy(allowPrivate)
	return t
}

// envProxy is the proxy the environment names for a request. A test
// replaces it: http.ProxyFromEnvironment reads the environment only once.
var envProxy = http.ProxyFromEnvironment

// guardedProxy is the environment's proxy for a request whose destination
// is public; a destination that is or resolves private is refused unless
// allowPrivate. The proxy's own addresses are recorded so the dial check
// lets the connection to it through.
func guardedProxy(allowPrivate bool) func(*http.Request) (*url.URL, error) {
	return func(req *http.Request) (*url.URL, error) {
		p, err := envProxy(req)
		if err != nil || p == nil || allowPrivate {
			return p, err
		}
		if ip, bad := privateDestination(req.Context(), req.URL.Hostname()); bad {
			return nil, fmt.Errorf("refusing to fetch %s through the proxy: it is the private address %s, and the URL you named is public", req.URL.Host, ip)
		}
		proxyAddrs.add(req.Context(), p)
		return p, nil
	}
}

// privateDestination resolves host and reports the first blocked address.
func privateDestination(ctx context.Context, host string) (net.IP, bool) {
	ips := []net.IP{net.ParseIP(host)}
	if ips[0] == nil {
		var err error
		if ips, err = lookupIP(ctx, host); err != nil {
			return nil, false
		}
	}
	for _, ip := range ips {
		if blockedAddr(ip) {
			return ip, true
		}
	}
	return nil, false
}

// proxyAddrs are the resolved host:port of the proxies in use.
var proxyAddrs = addrSet{m: map[string]bool{}}

type addrSet struct {
	mu sync.Mutex
	m  map[string]bool
}

func (s *addrSet) add(ctx context.Context, p *url.URL) {
	port := p.Port()
	if port == "" {
		port = defaultPort(p.Scheme)
	}
	ips := []net.IP{net.ParseIP(p.Hostname())}
	if ips[0] == nil {
		ips, _ = lookupIP(ctx, p.Hostname())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ip := range ips {
		s.m[net.JoinHostPort(ip.String(), port)] = true
	}
}

func (s *addrSet) has(addr string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[addr]
}
