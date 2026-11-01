// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"

	env "github.com/Jersyfi/hubtask/core/port/environment"
	health "github.com/Jersyfi/hubtask/core/port/health"
)

type clientAddressKey struct{}

// ProxyTrust resolves the address a request came from once, at the edge, so that every bucket and
// every session counts the same one (HUBTASK_TRUSTED_PROXIES, security.md §9).
//
// X-Forwarded-For is believed only from a peer inside a trusted network, and then only up to the
// right-most hop that is not itself trusted: everything left of that hop was written by the
// client, and believing it would hand every request a fresh budget.
type ProxyTrust struct {
	trusted []netip.Prefix
	// forwardedUntrusted records that a forwarded header arrived while no proxy is trusted - the
	// sign of an ingress in front that the configuration does not name, which makes every limit
	// keyed by address one budget for the whole installation.
	forwardedUntrusted atomic.Bool
}

// NewProxyTrust takes the validated configuration; an entry that does not parse is skipped, since
// startup has already refused it.
func NewProxyTrust(entries []string) *ProxyTrust {
	trust := &ProxyTrust{}
	for _, entry := range entries {
		if prefix, err := env.ParseTrustedProxy(entry); err == nil {
			trust.trusted = append(trust.trusted, prefix)
		}
	}
	return trust
}

// Wrap resolves the client address for everything below it.
func (p *ProxyTrust) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		address := p.resolve(r)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientAddressKey{}, address)))
	})
}

// Warnings is the health report's hint that an ingress forwards addresses nobody trusts.
func (p *ProxyTrust) Warnings(context.Context) []health.Warning {
	if !p.forwardedUntrusted.Load() {
		return nil
	}
	return []health.Warning{{
		Code: "config.trusted_proxies_missing", Severity: "warn",
		Params: map[string]string{"variable": "HUBTASK_TRUSTED_PROXIES"},
	}}
}

func (p *ProxyTrust) resolve(r *http.Request) string {
	peer := peerAddress(r)
	hops := forwardedHops(r)
	if len(p.trusted) == 0 {
		if len(hops) > 0 {
			p.forwardedUntrusted.Store(true)
		}
		return peer
	}
	current, err := netip.ParseAddr(peer)
	if err != nil || !p.isTrusted(current) {
		return peer
	}
	// Right to left: each hop was appended by the proxy to its right. The first one that is not a
	// trusted proxy is the client; a hop that does not parse ends the walk at the last one that
	// did, because nothing left of a garbled entry can be attributed to a proxy.
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := parseHop(hops[i])
		if err != nil {
			break
		}
		current = hop
		if !p.isTrusted(hop) {
			break
		}
	}
	return current.String()
}

func (p *ProxyTrust) isTrusted(address netip.Addr) bool {
	address = address.Unmap()
	for _, prefix := range p.trusted {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// forwardedHops reads X-Forwarded-For, or the for= parameters of Forwarded (RFC 7239) where the
// former is absent, in order from the client towards this process.
func forwardedHops(r *http.Request) []string {
	var hops []string
	for _, value := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				hops = append(hops, part)
			}
		}
	}
	if len(hops) > 0 {
		return hops
	}
	for _, value := range r.Header.Values("Forwarded") {
		for _, element := range strings.Split(value, ",") {
			for _, pair := range strings.Split(element, ";") {
				name, hop, found := strings.Cut(strings.TrimSpace(pair), "=")
				if found && strings.EqualFold(name, "for") {
					hops = append(hops, strings.Trim(hop, `"`))
				}
			}
		}
	}
	return hops
}

// parseHop accepts an address with or without a port, IPv6 in brackets included.
func parseHop(raw string) (netip.Addr, error) {
	raw = strings.TrimSpace(raw)
	if addrPort, err := netip.ParseAddrPort(raw); err == nil {
		return addrPort.Addr().Unmap(), nil
	}
	address, err := netip.ParseAddr(strings.Trim(raw, "[]"))
	if err != nil {
		return netip.Addr{}, err
	}
	return address.Unmap(), nil
}

func peerAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
