package importdocs

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// nonPublic are the ranges net.IP's own predicates miss: "this network"
// (0.0.0.0/8, which reaches the local host on Linux) and CGNAT
// (100.64.0.0/10, where Tailscale and carrier NAT hand out addresses).
var nonPublic = []*net.IPNet{
	mustCIDR("0.0.0.0/8"),
	mustCIDR("100.64.0.0/10"),
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
// unspecified, multicast, "this network" and CGNAT. IPv4-mapped IPv6 is
// judged as its IPv4 address.
func blockedAddr(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
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
// redirect hop and a host whose DNS answer points inside. A proxy from the
// environment is dialed through the same check.
func guardedTransport(allowPrivate bool) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   pageTimeout,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if allowPrivate {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if blockedAddr(net.ParseIP(host)) {
				return fmt.Errorf("refusing to connect to %s: a private address, and the URL you named is public", host)
			}
			return nil
		},
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = dialer.DialContext
	return t
}
