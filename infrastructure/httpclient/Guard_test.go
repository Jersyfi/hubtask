// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package httpclient_test

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

func guard(t *testing.T, cfg env.OutboundConfig) *httpclient.Guard {
	t.Helper()
	return httpclient.NewGuard(cfg)
}

// resolvingTo answers every lookup with the given addresses, so that a test can describe a DNS
// answer without needing a DNS server.
func resolvingTo(addrs ...string) func(context.Context, string) ([]netip.Addr, error) {
	parsed := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		parsed = append(parsed, netip.MustParseAddr(a))
	}
	return func(context.Context, string) ([]netip.Addr, error) { return parsed, nil }
}

func TestOnlyHTTPSchemesAreAccepted(t *testing.T) {
	g := guard(t, env.OutboundConfig{})

	for _, rawURL := range []string{
		"file:///etc/passwd",
		"gopher://example.org:70/_data",
		"ftp://example.org/secret",
		"dict://example.org:11211/stat",
		"jar:http://example.org!/",
	} {
		if _, err := g.CheckURL(rawURL); err == nil {
			t.Errorf("%s was accepted", rawURL)
		}
	}

	for _, rawURL := range []string{"http://example.org/hook", "HTTPS://example.org/hook"} {
		if _, err := g.CheckURL(rawURL); err != nil {
			t.Errorf("%s was rejected: %v", rawURL, err)
		}
	}
}

// Credentials in a URL end up in logs, in redirects, and in the Referer header of whatever the
// target renders.
func TestCredentialsInTheURLAreRefused(t *testing.T) {
	g := guard(t, env.OutboundConfig{})

	if _, err := g.CheckURL("https://user:hunter2@example.org/hook"); err == nil {
		t.Error("a URL with credentials was accepted")
	} else if shared.AsError(err).DetailCode != "dependency.target_credentials" {
		t.Errorf("detail code = %q, want dependency.target_credentials", shared.AsError(err).DetailCode)
	}
}

// The metadata endpoint is the single most valuable SSRF target there is: one GET and the
// attacker holds the instance's credentials.
func TestTheMetadataAddressesAreBlockedEvenWithPrivateNetworksOpen(t *testing.T) {
	g := guard(t, env.OutboundConfig{AllowPrivateNetworks: true})

	for _, addr := range []string{
		"169.254.169.254", // AWS, GCP, Azure, DigitalOcean, Hetzner
		"169.254.170.2",   // ECS task metadata
		"100.100.100.200", // Alibaba Cloud
		"fd00:ec2::254",   // AWS IMDS over IPv6
	} {
		if err := g.CheckAddr(netip.MustParseAddr(addr)); err == nil {
			t.Errorf("%s was accepted although private networks only cover the LAN", addr)
		}
	}
}

func TestPrivateAndLocalAddressesAreBlocked(t *testing.T) {
	g := guard(t, env.OutboundConfig{})

	blocked := []string{
		"127.0.0.1", "127.1.2.3", // loopback
		"10.0.0.5", "172.16.0.1", "172.31.255.254", "192.168.1.1", // RFC 1918
		"169.254.1.1",        // link-local
		"0.0.0.0", "0.1.2.3", // "this network"
		"100.64.0.1",              // carrier-grade NAT
		"192.0.0.1",               // IETF protocol assignments
		"198.18.0.1",              // benchmarking
		"255.255.255.255",         // broadcast
		"224.0.0.1",               // multicast
		"::1",                     // IPv6 loopback
		"fe80::1",                 // IPv6 link-local
		"fc00::1", "fd12:3456::1", // unique local
		"::",                 // unspecified
		"::ffff:127.0.0.1",   // IPv4-mapped loopback
		"::ffff:10.0.0.1",    // IPv4-mapped private
		"2002:7f00:1::1",     // 6to4 wrapping 127.0.0.1
		"2002:a00:1::1",      // 6to4 wrapping 10.0.0.1
		"64:ff9b::7f00:1",    // NAT64 wrapping 127.0.0.1
		"64:ff9b::a9fe:a9fe", // NAT64 wrapping the metadata address
	}
	for _, addr := range blocked {
		if err := g.CheckAddr(netip.MustParseAddr(addr)); err == nil {
			t.Errorf("%s was accepted", addr)
		}
	}

	for _, addr := range []string{"93.184.216.34", "8.8.8.8", "2606:2800:220:1::1"} {
		if err := g.CheckAddr(netip.MustParseAddr(addr)); err != nil {
			t.Errorf("the public address %s was rejected: %v", addr, err)
		}
	}
}

// A self-hoster with a target on their own LAN may open private networks - and gets loopback
// and RFC 1918 with it, but not the metadata services.
func TestPrivateNetworksCanBeOpenedDeliberately(t *testing.T) {
	g := guard(t, env.OutboundConfig{AllowPrivateNetworks: true})

	for _, addr := range []string{"127.0.0.1", "10.0.0.5", "192.168.1.1", "::1", "fd12:3456::1"} {
		if err := g.CheckAddr(netip.MustParseAddr(addr)); err != nil {
			t.Errorf("%s was rejected although private networks are open: %v", addr, err)
		}
	}
	if !g.AllowsPrivateNetworks() {
		t.Error("the guard does not report that private networks are open")
	}
	// Never an HTTP target under any configuration.
	for _, addr := range []string{"0.0.0.0", "224.0.0.1", "::"} {
		if err := g.CheckAddr(netip.MustParseAddr(addr)); err == nil {
			t.Errorf("%s was accepted", addr)
		}
	}
}

// A name with one public and one private record is not half acceptable: it is the standard way
// of hiding a private target behind a public name.
func TestEveryResolvedAddressHasToPass(t *testing.T) {
	g := guard(t, env.OutboundConfig{}).WithResolver(resolvingTo("93.184.216.34", "10.0.0.5"))

	if _, err := g.Resolve(context.Background(), "split.example.org"); err == nil {
		t.Error("a host with one private record was accepted")
	}
}

func TestResolveAcceptsAPublicHost(t *testing.T) {
	g := guard(t, env.OutboundConfig{}).WithResolver(resolvingTo("93.184.216.34"))

	addrs, err := g.Resolve(context.Background(), "example.org")
	if err != nil {
		t.Fatalf("a public host was rejected: %v", err)
	}
	if len(addrs) != 1 || addrs[0].String() != "93.184.216.34" {
		t.Errorf("addresses = %v, want the resolved one", addrs)
	}
}

// DNS trouble is temporary far more often than it is permanent, so it must not be reported as
// "this target is wrong" - a webhook subscription would be disabled for a resolver hiccup.
func TestAFailedLookupIsUnavailableNotInvalid(t *testing.T) {
	g := guard(t, env.OutboundConfig{}).WithResolver(
		func(context.Context, string) ([]netip.Addr, error) { return nil, errors.New("SERVFAIL") })

	_, err := g.Resolve(context.Background(), "example.org")

	domainErr := shared.AsError(err)
	if domainErr.Category != shared.CategoryUnavailable {
		t.Errorf("category = %s, want %s", domainErr.Category, shared.CategoryUnavailable)
	}
	if domainErr.DetailCode != "dependency.target_unresolvable" {
		t.Errorf("detail code = %q, want dependency.target_unresolvable", domainErr.DetailCode)
	}
}

// An empty answer is the same situation as a failed one, and it used to be the case that slips
// through: len(addrs) == 0 with err == nil.
func TestAnEmptyAnswerIsNotASilentPass(t *testing.T) {
	g := guard(t, env.OutboundConfig{}).WithResolver(resolvingTo())

	if _, err := g.Resolve(context.Background(), "example.org"); err == nil {
		t.Error("a host that resolved to nothing was accepted")
	}
}

func TestTheAllowlistIsExclusive(t *testing.T) {
	g := guard(t, env.OutboundConfig{AllowedHosts: []string{"hooks.example.org"}})

	if _, err := g.CheckURL("https://hooks.example.org/deliver"); err != nil {
		t.Errorf("the allowed host was rejected: %v", err)
	}
	// Case is not a distinction a host name makes.
	if _, err := g.CheckURL("https://Hooks.Example.ORG/deliver"); err != nil {
		t.Errorf("the allowed host in mixed case was rejected: %v", err)
	}
	for _, rawURL := range []string{
		"https://evil.example.org/deliver",
		"https://hooks.example.org.evil.test/deliver", // the suffix trick
		"https://93.184.216.34/deliver",
	} {
		if _, err := g.CheckURL(rawURL); err == nil {
			t.Errorf("%s passed an allowlist that does not contain it", rawURL)
		} else if shared.AsError(err).DetailCode != "dependency.target_not_allowed" {
			t.Errorf("%s: detail code = %q, want dependency.target_not_allowed",
				rawURL, shared.AsError(err).DetailCode)
		}
	}
}

// The dial-time hook is the one that closes rebinding, and it sees "host:port" rather than a URL.
func TestTheDialHookChecksTheConcreteAddress(t *testing.T) {
	g := guard(t, env.OutboundConfig{})

	if err := g.CheckControl("tcp4", "93.184.216.34:443"); err != nil {
		t.Errorf("a public address was refused at dial time: %v", err)
	}
	for _, address := range []string{
		"127.0.0.1:8080",
		"169.254.169.254:80",
		"[::1]:443",
		"not-an-address",
		"example.org:443", // a name at dial time means something replaced the dialler
	} {
		if err := g.CheckControl("tcp", address); err == nil {
			t.Errorf("%s was accepted at dial time", address)
		}
	}
}

// A refused target is wrong and will still be wrong in an hour. The webhook dispatcher needs to
// tell that apart from "unreachable right now", because one disables a subscription and the
// other one retries.
func TestABlockedTargetIsDistinguishableFromAnOutage(t *testing.T) {
	g := guard(t, env.OutboundConfig{})

	_, blocked := g.CheckURL("http://127.0.0.1/hook")
	if !httpclient.IsBlocked(blocked) {
		t.Errorf("%v is not recognised as a blocked target", blocked)
	}
	if got := shared.AsError(blocked).Category; got != shared.CategoryValidation {
		t.Errorf("category = %s, want %s - nobody should retry a blocked target", got, shared.CategoryValidation)
	}

	if httpclient.IsBlocked(shared.ErrUnavailable) {
		t.Error("a plain outage was reported as a blocked target")
	}
	if httpclient.IsBlocked(errors.New("connection reset")) {
		t.Error("an untyped error was reported as a blocked target")
	}
}
