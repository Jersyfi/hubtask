// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package httpclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	port "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// UC-MED-06/8. The one exception to the private-network rule (security.md §10, ADR-0104): the document server
// the administration entered may sit on a private network, and only calls to its own host use it.

func byName(names map[string]string) func(context.Context, string) ([]netip.Addr, error) {
	return func(_ context.Context, host string) ([]netip.Addr, error) {
		if a, ok := names[host]; ok {
			return []netip.Addr{netip.MustParseAddr(a)}, nil
		}
		return nil, nil
	}
}

func TestTheEnteredDocumentServerMayBePrivate(t *testing.T) {
	g := httpclient.NewGuard(env.OutboundConfig{}).
		WithResolver(byName(map[string]string{"office.intern": "10.20.0.5"})).
		ForEnteredServer("Office.Intern")
	if _, err := g.CheckURL("https://office.intern/hosting/discovery"); err != nil {
		t.Fatalf("the entered server's address: %v", err)
	}
	if _, err := g.Resolve(context.Background(), "office.intern"); err != nil {
		t.Fatalf("the entered server's resolution: %v", err)
	}
	if err := g.ControlFor("office.intern")("tcp", "10.20.0.5:443"); err != nil {
		t.Fatalf("the entered server's dial: %v", err)
	}
	// The metadata addresses stay closed even for it.
	if err := g.ControlFor("office.intern")("tcp", "169.254.169.254:80"); err == nil {
		t.Error("the entered server reached a metadata address")
	}
}

func TestAnyOtherPrivateAddressIsStillRefused(t *testing.T) {
	g := httpclient.NewGuard(env.OutboundConfig{}).
		WithResolver(byName(map[string]string{"office.intern": "10.20.0.5", "files.intern": "10.20.0.6"})).
		ForEnteredServer("office.intern")
	if _, err := g.Resolve(context.Background(), "files.intern"); !httpclient.IsPrivateTarget(err) {
		t.Errorf("another private host: %v", err)
	}
	if _, err := g.CheckURL("https://10.20.0.5/"); !httpclient.IsPrivateTarget(err) {
		t.Errorf("the entered server's address written as another host: %v", err)
	}
	if err := g.ControlFor("files.intern")("tcp", "10.20.0.6:443"); !httpclient.IsPrivateTarget(err) {
		t.Errorf("another private host at dial time: %v", err)
	}
}

// A name that answered public and connects private - the rebind - is refused at the dial, even
// on the client that may reach the entered server.
func TestADNSRebindIsStillRefused(t *testing.T) {
	g := httpclient.NewGuard(env.OutboundConfig{}).
		WithResolver(byName(map[string]string{"office.intern": "10.20.0.5", "rebind.example": "93.184.216.34"})).
		ForEnteredServer("office.intern")
	if _, err := g.Resolve(context.Background(), "rebind.example"); err != nil {
		t.Fatalf("the public answer: %v", err)
	}
	if err := g.ControlFor("rebind.example")("tcp", "10.20.0.5:443"); !httpclient.IsPrivateTarget(err) {
		t.Errorf("a rebind to the entered server's own address: %v", err)
	}
}

// The entered server (here loopback, as a LAN host would be) answers; its redirect to another
// private host is refused.
func TestARedirectToAnotherPrivateAddressIsStillRefused(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("inside")) }))
	defer other.Close()
	otherURL, _ := url.Parse(other.URL)
	elsewhere := "http://localhost:" + otherURL.Port() + "/"
	entered := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/away" {
			http.Redirect(w, r, elsewhere, http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("office"))
	}))
	defer entered.Close()
	enteredURL, _ := url.Parse(entered.URL)

	cfg := env.OutboundConfig{MaxRedirects: 3, Timeout: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 1 << 16}
	client := httpclient.NewGuardedClient(cfg, httpclient.NewGuard(cfg)).ForEnteredServer(enteredURL.Hostname())
	answer, err := client.Do(context.Background(), port.Request{URL: entered.URL + "/", TargetClass: "document_server"})
	if err != nil || string(answer.Body) != "office" {
		t.Fatalf("the entered server: %q, %v", answer.Body, err)
	}
	_, err = client.Do(context.Background(), port.Request{URL: entered.URL + "/away", TargetClass: "document_server"})
	if !httpclient.IsPrivateTarget(err) {
		t.Errorf("a redirect to another private host: %v (%s)", err, shared.AsError(err).DetailCode)
	}
	// The client the exception was not made for refuses the entered server too.
	if _, err := httpclient.NewGuardedClient(cfg, httpclient.NewGuard(cfg)).Do(context.Background(), port.Request{URL: entered.URL + "/", TargetClass: "document_server"}); !httpclient.IsPrivateTarget(err) {
		t.Errorf("the plain client reached a private server: %v", err)
	}
}
