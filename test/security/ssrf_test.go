// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package security is gate SG-6 and its neighbours: the security properties that are checked
// against the running building blocks rather than against the source.
//
// SG-6 is the SSRF suite (security.md §13). It exercises GuardedClient the way an attacker
// would: with the metadata addresses, with private networks, with a resolver that changes its
// mind between the check and the connection, and with redirect chains that try to walk out of
// all three.
package security

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	port "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

func outbound() env.OutboundConfig {
	return env.OutboundConfig{
		Timeout:          2 * time.Second,
		ConnectTimeout:   time.Second,
		MaxResponseBytes: 1 << 16,
		MaxRedirects:     3,
	}
}

func clientFor(cfg env.OutboundConfig) *httpclient.GuardedClient {
	return httpclient.NewGuardedClient(cfg, httpclient.NewGuard(cfg))
}

func call(t *testing.T, client *httpclient.GuardedClient, url string) error {
	t.Helper()
	_, err := client.Do(context.Background(), port.Request{URL: url, TargetClass: "webhook"})
	return err
}

// One GET against the metadata service and the attacker holds the instance's credentials. This
// is the single most valuable SSRF target there is, and it must be unreachable in every
// configuration Hubtask offers - including the one that opens private networks for a
// self-hoster's LAN.
func TestSG6MetadataServicesAreUnreachable(t *testing.T) {
	targets := []string{
		"http://169.254.169.254/latest/meta-data/iam/security-credentials/",
		"http://169.254.169.254/computeMetadata/v1/instance/service-accounts/",
		"http://169.254.170.2/v2/credentials/",
		"http://100.100.100.200/latest/meta-data/",
		"http://[fd00:ec2::254]/latest/meta-data/",
		// The same address, written so that a naive string comparison misses it.
		"http://[::ffff:169.254.169.254]/latest/meta-data/",
		"http://[64:ff9b::a9fe:a9fe]/latest/meta-data/",
		"http://[2002:a9fe:a9fe::]/latest/meta-data/",
	}

	for _, allowPrivate := range []bool{false, true} {
		cfg := outbound()
		cfg.AllowPrivateNetworks = allowPrivate
		client := clientFor(cfg)

		for _, target := range targets {
			if err := call(t, client, target); err == nil {
				t.Errorf("private_networks=%v: %s was called", allowPrivate, target)
			} else if !httpclient.IsBlocked(err) {
				t.Errorf("private_networks=%v: %s failed with %v, want a refusal by the guard",
					allowPrivate, target, err)
			}
		}
	}
}

// The network the server sits in is the network everything else is protecting: a webhook must
// not become a port scanner of it.
func TestSG6PrivateNetworksAreUnreachable(t *testing.T) {
	client := clientFor(outbound())

	for _, target := range []string{
		"http://127.0.0.1:8080/admin",
		"http://localhost:9090/metrics",
		"http://10.0.0.5/internal",
		"http://172.16.4.4/internal",
		"http://192.168.1.1/router",
		"http://[::1]:9090/metrics",
		"http://[fd12:3456::1]/internal",
		"http://0.0.0.0:8080/",
		"http://[::ffff:127.0.0.1]:8080/",
	} {
		if err := call(t, client, target); err == nil {
			t.Errorf("%s was called", target)
		}
	}
}

// Nothing but http and https. file:// reads the disk, gopher:// speaks to anything that listens,
// and neither of them was reviewed as a protocol Hubtask speaks.
func TestSG6OnlyHTTPSchemesLeaveTheProcess(t *testing.T) {
	client := clientFor(outbound())

	for _, target := range []string{
		"file:///etc/passwd",
		"file:///proc/self/environ",
		"gopher://127.0.0.1:6379/_INFO",
		"dict://127.0.0.1:11211/stats",
		"ftp://internal.example.org/backup.tar",
	} {
		if err := call(t, client, target); err == nil {
			t.Errorf("%s was called", target)
		}
	}
}

// A host name is refused on what it resolves to, not on how it is spelled. Decimal, octal, and
// dotted-hex spellings of 127.0.0.1 are accepted by some resolvers and rejected by others, and
// a guard that only reads the text would let through whichever spelling it had not thought of.
func TestSG6ANameIsJudgedByWhatItResolvesTo(t *testing.T) {
	cfg := outbound()
	// Every name resolves to loopback here - the situation a resolver under somebody else's
	// control produces, whatever was typed into the webhook form.
	guard := httpclient.NewGuard(cfg).WithResolver(
		func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		})
	client := httpclient.NewGuardedClient(cfg, guard)

	for _, target := range []string{
		"http://2130706433/",             // 127.0.0.1 in decimal
		"http://0177.0.0.1/",             // 127.0.0.1 in octal
		"http://hooks.attacker.test/win", // an ordinary name, pointed inwards
	} {
		err := call(t, client, target)
		if err == nil {
			t.Errorf("%s was called", target)
			continue
		}
		if !httpclient.IsBlocked(err) {
			t.Errorf("%s failed with %v, want a refusal by the guard", target, err)
		}
	}
}

// DNS rebinding: the name resolves to a public address when it is checked and to a private one
// when the socket is opened. Only the check inside net.Dialer.Control sees the second answer -
// this test fails the moment that hook is removed.
func TestSG6DNSRebindingIsCaughtAtDialTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("this is the internal service"))
	}))
	defer server.Close()

	cfg := outbound()
	// The resolver of the guard lies exactly once, the way a rebinding attack does: the
	// pre-flight check is told a public address, while the dialler goes on to resolve
	// "localhost" through the system and lands on loopback.
	lying := httpclient.NewGuard(cfg).WithResolver(
		func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		})
	client := httpclient.NewGuardedClient(cfg, lying)

	target := "http://localhost:" + portOf(t, server.URL) + "/internal"
	err := call(t, client, target)

	if err == nil {
		t.Fatal("the rebound address was connected to - the dial-time check is not in place")
	}
	if !httpclient.IsBlocked(err) {
		t.Errorf("error = %v, want a refusal by the guard", err)
	}
}

// A redirect is a second request, and it gets the same treatment as the first. Here the first
// hop is a legitimate target on the LAN of a self-hoster who opened private networks - and the
// second hop tries to walk from there into the metadata service.
func TestSG6ARedirectCannotReachTheMetadataService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer server.Close()

	cfg := outbound()
	cfg.AllowPrivateNetworks = true
	err := call(t, clientFor(cfg), server.URL)

	if err == nil {
		t.Fatal("the redirect into the metadata service was followed")
	}
	if !httpclient.IsBlocked(err) {
		t.Errorf("error = %v, want a refusal by the guard", err)
	}
}

func TestSG6ARedirectChainIsBounded(t *testing.T) {
	var hops int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		http.Redirect(w, r, "/hop", http.StatusFound)
	}))
	defer server.Close()

	cfg := outbound()
	cfg.AllowPrivateNetworks = true
	cfg.MaxRedirects = 2

	err := call(t, clientFor(cfg), server.URL)
	if err == nil {
		t.Fatal("an endless redirect chain was followed to the end")
	}
	if got := shared.AsError(err).DetailCode; got != "dependency.too_many_redirects" {
		t.Errorf("detail code = %q, want dependency.too_many_redirects", got)
	}
	if hops > cfg.MaxRedirects+1 {
		t.Errorf("the target was called %d times for a budget of %d redirects", hops, cfg.MaxRedirects)
	}
}

// A target that answers with a stream until the process runs out of memory is a denial of
// service that needs no privileges at all (T-17).
func TestSG6AnEndlessResponseIsCutOff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := strings.Repeat("x", 4096)
		for range 1000 {
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
			if r.Context().Err() != nil {
				return
			}
		}
	}))
	defer server.Close()

	cfg := outbound()
	cfg.AllowPrivateNetworks = true
	cfg.MaxResponseBytes = 8192

	err := call(t, clientFor(cfg), server.URL)
	if err == nil {
		t.Fatal("a response of four megabytes was accepted against a limit of eight kilobytes")
	}
	if got := shared.AsError(err).DetailCode; got != "dependency.response_too_large" {
		t.Errorf("detail code = %q, want dependency.response_too_large", got)
	}
}

// In provider operation the allowlist is mandatory (T-07). What it must not do is let a target
// through because it merely looks like an entry.
func TestSG6TheAllowlistCannotBeTrickedByASuffix(t *testing.T) {
	cfg := outbound()
	cfg.AllowedHosts = []string{"hooks.example.org"}
	client := clientFor(cfg)

	for _, target := range []string{
		"https://hooks.example.org.attacker.test/deliver",
		"https://attacker.test/hooks.example.org",
		"https://attackerhooks.example.org/deliver",
		"https://hooks.example.org@attacker.test/deliver", // the userinfo trick
	} {
		if err := call(t, client, target); err == nil {
			t.Errorf("%s passed the allowlist", target)
		}
	}
}

func portOf(t *testing.T, rawURL string) string {
	t.Helper()
	_, hostPort, found := strings.Cut(rawURL, "//")
	if !found {
		t.Fatalf("%s has no host", rawURL)
	}
	_, p, found := strings.Cut(hostPort, ":")
	if !found {
		t.Fatalf("%s has no port", rawURL)
	}
	return strings.TrimSuffix(p, "/")
}

// A webhook target is an egress channel exactly as a backup target is, and G-03's acceptance asks
// for the test to say so by name: a subscription pointed at the metadata service or at a private
// range is refused by the guard, and the private range is released only by explicit configuration.
//
// The two halves are deliberately different. The metadata address is unreachable in every
// configuration this product offers, because no self-hoster's LAN contains it and one GET against
// it hands over the instance's credentials. A private range is somebody's own network, and an
// installation that legitimately calls a service on it says so once, for the whole installation -
// which is BK-9's shape for a backup target, and this is its sibling.
func TestAWebhookTargetIsGuardedLikeABackupTarget(t *testing.T) {
	const metadata = "http://169.254.169.254/latest/meta-data/"
	const private = "http://10.0.0.7/hooks/hubtask"

	closed := outbound()
	closed.AllowPrivateNetworks = false
	if err := call(t, clientFor(closed), private); err == nil {
		t.Error("a webhook to a private address was allowed by default")
	}
	if err := call(t, clientFor(closed), metadata); err == nil {
		t.Error("a webhook to the metadata service was allowed")
	}

	// Released deliberately and once, for the installation rather than for the subscription: a
	// per-target opt-out would be an opt-out a request could ask for.
	opened := outbound()
	opened.AllowPrivateNetworks = true
	if err := call(t, clientFor(opened), private); err != nil && isGuardRefusal(err) {
		t.Errorf("a released private network still refused a webhook: %v", err)
	}
	if err := call(t, clientFor(opened), metadata); err == nil {
		t.Fatal("releasing private networks made the metadata service reachable")
	}
}

// isGuardRefusal separates "this installation would not dial that" from "the dial did not work",
// which is the same distinction the deliverer records. A released private address is expected to
// fail to connect in a test - nothing is listening - and that is not the guard talking.
func isGuardRefusal(err error) bool {
	return errors.Is(err, shared.ErrForbidden)
}
