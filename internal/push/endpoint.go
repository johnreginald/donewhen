package push

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrInvalidEndpoint means a push endpoint may not be stored or contacted.
var ErrInvalidEndpoint = errors.New("invalid_endpoint")

const (
	maxEndpointLen = 2048
	lookupTimeout  = 3 * time.Second
)

// nonPublic lists address ranges a push service never lives in. The standard
// library helpers cover loopback, RFC 1918, link-local and unspecified; these
// are the rest (carrier-grade NAT, documentation, benchmarking, reserved,
// NAT64 and discard ranges).
var nonPublic = mustCIDRs(
	"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
	"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
	"64:ff9b::/96", "100::/64", "2001:db8::/32",
)

func mustCIDRs(list ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(list))
	for _, c := range list {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

// IsPublicIP reports whether ip is a routable public address: not loopback,
// private, link-local, multicast, unspecified or otherwise reserved. IPv4-mapped
// IPv6 addresses are judged by their IPv4 form.
func IsPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	for _, n := range nonPublic {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// Resolver turns a host name into addresses. It is a variable so tests can
// avoid real DNS.
type Resolver func(ctx context.Context, host string) ([]net.IP, error)

// DefaultResolver uses the system resolver.
func DefaultResolver(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.IP)
	}
	return out, nil
}

func invalidEndpoint(why string) error {
	return fmt.Errorf("%w: %s", ErrInvalidEndpoint, why)
}

// ValidateEndpoint accepts only https URLs whose host is a public address (or
// resolves only to public addresses). A subscription endpoint is a URL the
// server will POST to, so without this a user could aim the notifier at
// internal services (SSRF). resolve may be nil for DefaultResolver.
func ValidateEndpoint(ctx context.Context, raw string, resolve Resolver) error {
	if raw == "" || len(raw) > maxEndpointLen || strings.TrimSpace(raw) != raw {
		return invalidEndpoint("not a valid URL")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return invalidEndpoint("not a valid URL")
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return invalidEndpoint("must be https")
	}
	if u.User != nil {
		return invalidEndpoint("credentials in the URL are not allowed")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return invalidEndpoint("not a valid URL")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !IsPublicIP(ip) {
			return invalidEndpoint("host is not a public address")
		}
		return nil
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return invalidEndpoint("host is not a public address")
	}
	if resolve == nil {
		resolve = DefaultResolver
	}
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	ips, err := resolve(ctx, host)
	if err != nil || len(ips) == 0 {
		return invalidEndpoint("host does not resolve")
	}
	for _, ip := range ips {
		if !IsPublicIP(ip) {
			return invalidEndpoint("host is not a public address")
		}
	}
	return nil
}

// safeControl runs when the dialer is about to connect, with the address
// already resolved. Checking here (not just at subscribe time) closes the DNS
// rebinding gap: a host that was public when validated cannot later resolve to
// an internal address and be reached.
func safeControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if !IsPublicIP(net.ParseIP(host)) {
		return fmt.Errorf("%w: refusing to connect to non-public address", ErrInvalidEndpoint)
	}
	return nil
}

// NewSafeClient returns an HTTP client for push services that refuses
// non-public addresses at connect time, never follows redirects (a redirect is
// another way to reach an internal host) and never uses a proxy.
func NewSafeClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: safeControl}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			MaxIdleConns:          20,
			IdleConnTimeout:       30 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
