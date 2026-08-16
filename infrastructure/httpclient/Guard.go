// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package httpclient is the only way out of the process. Every outbound call goes through
// GuardedClient (rule 6, ADR-0015, security.md §T-07); http.DefaultClient is banned, and an
// architecture test enforces it.
//
// The threat is server-side request forgery: a webhook URL, an automation action, or an AI
// endpoint is configured by a user, and the server is the one that dials it. Without a guard,
// "https://169.254.169.254/latest/meta-data/" is a request the server makes with the server's
// network position - which is inside the network everything else is protecting.
package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// The detail codes of a refused target. Constants rather than literals at the call sites,
// because IsBlocked needs the same set and a second copy of it would be a second place to
// forget a code.
const (
	codeTargetMalformed    = "dependency.target_malformed"
	codeTargetScheme       = "dependency.target_scheme"
	codeTargetCredentials  = "dependency.target_credentials"
	codeTargetNotAllowed   = "dependency.target_not_allowed"
	codeTargetBlocked      = "dependency.target_blocked"
	codeTargetUnresolvable = "dependency.target_unresolvable"
)

// Guard decides whether an address may be dialled. It is separate from the client because the
// decision is needed twice: once when a URL is configured or a call is prepared, and once at
// dial time, on the concrete IP the connection is about to use. Only the second one closes DNS
// rebinding, and only the first one produces a good error message.
type Guard struct {
	// allowed is the egress allowlist by host name. Empty means every public address.
	allowed map[string]struct{}
	// allowPrivate opens RFC 1918, loopback, and link-local. The metadata addresses stay
	// blocked either way - see blockedAlways.
	allowPrivate bool
	// lookupIP resolves a host. Injected so that the SSRF suite can serve a rebinding
	// resolver without a DNS server.
	lookupIP func(ctx context.Context, host string) ([]netip.Addr, error)
}

// NewGuard builds the guard from the outbound configuration.
func NewGuard(cfg env.OutboundConfig) *Guard {
	allowed := make(map[string]struct{}, len(cfg.AllowedHosts))
	for _, host := range cfg.AllowedHosts {
		allowed[strings.ToLower(host)] = struct{}{}
	}
	return &Guard{
		allowed:      allowed,
		allowPrivate: cfg.AllowPrivateNetworks,
		lookupIP:     defaultLookup,
	}
}

// WithResolver returns a copy that resolves through lookup. For the SSRF suite: a rebinding
// attack needs a resolver that answers differently the second time, and a test must not depend
// on a DNS server to have one.
func (g *Guard) WithResolver(lookup func(ctx context.Context, host string) ([]netip.Addr, error)) *Guard {
	copied := *g
	copied.lookupIP = lookup
	return &copied
}

func defaultLookup(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// CheckURL validates a target before anything is dialled: the scheme, the shape, and the
// allowlist. It does not resolve - a caller that only wants to know whether a configured
// webhook URL is acceptable should not make a DNS query, and an operator typing a URL into a
// form should get an answer that does not depend on the network.
func (g *Guard) CheckURL(rawURL string) (*url.URL, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return nil, shared.ErrValidation.
			WithDetail(codeTargetMalformed).
			WithCause(fmt.Errorf("parsing the target URL: %w", err))
	}

	switch strings.ToLower(target.Scheme) {
	case "http", "https":
	default:
		// file://, gopher://, ftp:// and friends are the classic escapes out of an HTTP
		// client into something that reads the local disk or speaks a protocol nobody
		// reviewed (T-07).
		return nil, blockedError(codeTargetScheme, target.Scheme)
	}

	if target.Host == "" {
		return nil, blockedError(codeTargetMalformed, rawURL)
	}
	if target.User != nil {
		// Credentials in the URL end up in logs, in redirects, and in the Referer header of
		// whatever the target renders. There is a header for this.
		return nil, blockedError(codeTargetCredentials, target.Hostname())
	}

	host := strings.ToLower(target.Hostname())
	if len(g.allowed) > 0 {
		if _, ok := g.allowed[host]; !ok {
			return nil, shared.ErrValidation.
				WithDetail(codeTargetNotAllowed).
				WithParams(map[string]string{"host": host})
		}
	}

	// A literal IP needs no resolution, and checking it here means the caller learns about
	// http://127.0.0.1/ without a DNS round trip that was never going to happen.
	if addr, err := netip.ParseAddr(host); err == nil {
		if err := g.CheckAddr(addr); err != nil {
			return nil, err
		}
	}
	return target, nil
}

// Resolve checks every address a host resolves to. All of them have to pass: a name with one
// public and one private A record is not half acceptable, it is the standard way of hiding a
// private target behind a public one.
//
// The returned addresses are what the dialler should use. Resolving here and dialling there
// leaves a window in which DNS can change its mind, which is why CheckAddr runs again at dial
// time - this pass is for the error message and for failing before a connection is attempted.
func (g *Guard) Resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		if err := g.CheckAddr(addr); err != nil {
			return nil, err
		}
		return []netip.Addr{addr}, nil
	}

	addrs, err := g.lookupIP(ctx, host)
	if err != nil {
		// DNS trouble is temporary far more often than it is permanent, so this is
		// UNAVAILABLE rather than a rejection of the target.
		return nil, shared.ErrUnavailable.
			WithDetail(codeTargetUnresolvable).
			WithParams(map[string]string{"host": host}).
			WithCause(fmt.Errorf("resolving %s: %w", host, err))
	}
	if len(addrs) == 0 {
		return nil, shared.ErrUnavailable.
			WithDetail(codeTargetUnresolvable).
			WithParams(map[string]string{"host": host})
	}
	for _, addr := range addrs {
		if err := g.CheckAddr(addr); err != nil {
			return nil, err
		}
	}
	return addrs, nil
}

// CheckControl is the hook for net.Dialer.Control. It runs after resolution, with the concrete
// address the socket is about to connect to, and it is the only check that closes DNS
// rebinding: between Resolve and the connection, the answer can change, and a resolver under
// somebody else's control will make sure it does.
func (g *Guard) CheckControl(_, address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return blockedError(codeTargetMalformed, address)
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		// At this point the address is always numeric. If it is not, something replaced the
		// dialler, and refusing is the only safe answer.
		return blockedError(codeTargetMalformed, host)
	}
	return g.CheckAddr(addr)
}

// CheckAddr is the address policy in one place.
func (g *Guard) CheckAddr(addr netip.Addr) error {
	if !addr.IsValid() {
		return blockedError(codeTargetBlocked, addr.String())
	}
	addr = addr.Unmap() // ::ffff:127.0.0.1 is 127.0.0.1 wearing a hat

	// An IPv6 address that carries an IPv4 one inside it is checked as that IPv4 address too:
	// 6to4 and NAT64 are two more ways of writing 127.0.0.1.
	if embedded, ok := embeddedIPv4(addr); ok {
		if err := g.CheckAddr(embedded); err != nil {
			return err
		}
	}
	// Blocked whatever the configuration says: the cloud metadata services. Opening private
	// networks is a decision a self-hoster can reasonably make for a target on their own LAN;
	// handing out the instance's credentials is not part of that decision.
	for _, blocked := range blockedAlways {
		if blocked.Contains(addr) {
			return blockedError(codeTargetBlocked, addr.String())
		}
	}
	if addr.IsUnspecified() || addr.IsMulticast() || addr.IsInterfaceLocalMulticast() {
		// 0.0.0.0 means "this host" to a connect(), and multicast is not an HTTP target
		// under any configuration.
		return blockedError(codeTargetBlocked, addr.String())
	}

	if g.allowPrivate {
		return nil
	}
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return blockedError(codeTargetBlocked, addr.String())
	}
	for _, blocked := range blockedRanges {
		if blocked.Contains(addr) {
			return blockedError(codeTargetBlocked, addr.String())
		}
	}
	return nil
}

// AllowsPrivateNetworks reports whether the guard has been opened to private addresses. The
// health report reads it, so that the warning and the actual behaviour cannot drift apart.
func (g *Guard) AllowsPrivateNetworks() bool { return g.allowPrivate }

// embeddedIPv4 extracts the IPv4 address carried inside a 6to4 (2002::/16) or NAT64
// (64:ff9b::/96) address. Both are legitimate transition mechanisms and both are a way to spell
// a private IPv4 address in IPv6.
func embeddedIPv4(addr netip.Addr) (netip.Addr, bool) {
	if !addr.Is6() {
		return netip.Addr{}, false
	}
	raw := addr.As16()
	switch {
	case raw[0] == 0x20 && raw[1] == 0x02: // 2002:V4ADDR::/48
		return netip.AddrFrom4([4]byte{raw[2], raw[3], raw[4], raw[5]}), true
	case nat64.Contains(addr): // 64:ff9b::V4ADDR
		return netip.AddrFrom4([4]byte{raw[12], raw[13], raw[14], raw[15]}), true
	default:
		return netip.Addr{}, false
	}
}

var nat64 = netip.MustParsePrefix("64:ff9b::/96")

// blockedAlways is blocked even with private networks opened: the cloud metadata endpoints.
// They are the single most valuable SSRF target there is - one GET and the attacker holds the
// instance's credentials.
var blockedAlways = []netip.Prefix{
	netip.MustParsePrefix("169.254.169.254/32"), // AWS, GCP, Azure, DigitalOcean, Hetzner
	netip.MustParsePrefix("169.254.170.2/32"),   // ECS task metadata
	netip.MustParsePrefix("100.100.100.200/32"), // Alibaba Cloud
	netip.MustParsePrefix("fd00:ec2::254/128"),  // AWS IMDS over IPv6
}

// blockedRanges is what the standard library's predicates do not cover
// (security.md §T-07: RFC 1918, loopback, link-local, ULA).
var blockedRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),   // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // documentation
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // documentation
	netip.MustParsePrefix("203.0.113.0/24"),  // documentation
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, includes 255.255.255.255
	netip.MustParsePrefix("fc00::/7"),        // unique local addresses
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
	netip.MustParsePrefix("::/128"),          // unspecified
	netip.MustParsePrefix("::1/128"),         // loopback, for the avoidance of doubt
}

// blockedError is a VALIDATION error, not UNAVAILABLE: the target is wrong and will still be
// wrong in an hour, so nobody should retry it. value names what was refused - the host or the
// address the caller supplied, never anything else about the request.
func blockedError(detailCode, value string) *shared.Error {
	return shared.ErrValidation.
		WithDetail(detailCode).
		WithParams(map[string]string{"target": value})
}

// refusals is what IsBlocked recognises. target_unresolvable is deliberately absent: a name
// that does not resolve today may resolve tomorrow, and treating a resolver hiccup as a
// permanently wrong target would disable a working webhook subscription.
var refusals = map[string]bool{
	codeTargetMalformed:   true,
	codeTargetScheme:      true,
	codeTargetCredentials: true,
	codeTargetNotAllowed:  true,
	codeTargetBlocked:     true,
}

// IsBlocked reports whether err is the guard refusing a target. The webhook dispatcher needs it
// to disable a subscription rather than retry it for 24 hours.
func IsBlocked(err error) bool {
	var typed *shared.Error
	if !errors.As(err, &typed) {
		return false
	}
	return refusals[typed.DetailCode]
}
