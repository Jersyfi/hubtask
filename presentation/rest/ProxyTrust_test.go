// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// resolvedThrough runs r through the trust and answers the address the handlers below it see.
func resolvedThrough(t *testing.T, trust *ProxyTrust, r *http.Request) string {
	t.Helper()
	var seen string
	trust.Wrap(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = ClientAddress(r)
	})).ServeHTTP(httptest.NewRecorder(), r)
	return seen
}

func forwardedRequest(t *testing.T, peer string, header, value string) *http.Request {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/auth/sessions", nil)
	r.RemoteAddr = peer
	if header != "" {
		r.Header.Set(header, value)
	}
	return r
}

func TestTheClientAddressBelievesOnlyATrustedProxy(t *testing.T) {
	trust := NewProxyTrust([]string{"10.42.0.0/16", "192.0.2.1"})
	cases := []struct {
		name, peer, header, value, want string
	}{
		{"no header, the peer", "198.51.100.4:5000", "", "", "198.51.100.4"},
		{"an untrusted peer's header is ignored", "198.51.100.4:5000", "X-Forwarded-For", "203.0.113.9", "198.51.100.4"},
		{"a trusted peer's hop is the client", "10.42.0.17:5000", "X-Forwarded-For", "203.0.113.9", "203.0.113.9"},
		{"the right-most untrusted hop, not what the client wrote", "10.42.0.17:5000", "X-Forwarded-For", "1.1.1.1, 203.0.113.9", "203.0.113.9"},
		{"a chain of trusted proxies is walked", "10.42.0.17:5000", "X-Forwarded-For", "203.0.113.9, 192.0.2.1, 10.42.3.3", "203.0.113.9"},
		{"a garbled hop ends the walk", "10.42.0.17:5000", "X-Forwarded-For", "203.0.113.9, nonsense", "10.42.0.17"},
		{"Forwarded is read where X-Forwarded-For is absent", "10.42.0.17:5000", "Forwarded", `for="[2001:db8::7]:443";proto=https`, "2001:db8::7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolvedThrough(t, trust, forwardedRequest(t, c.peer, c.header, c.value)); got != c.want {
				t.Errorf("client address = %q, want %q", got, c.want)
			}
		})
	}
}

// The finding's failure: behind an ingress every sign-in shares the proxy's budget. Through a
// trusted proxy two people are two buckets; from an untrusted peer the header buys nothing.
func TestTheAuthBucketCountsTheForwardedAddressOnlyThroughATrustedProxy(t *testing.T) {
	trust := NewProxyTrust([]string{"10.42.0.0/16"})
	limited := Limited{
		Limiter: NewRateLimiter(), Level: "auth",
		Bucket: AuthBucket(1, 1),
		Clock:  func() time.Time { return start },
	}
	reached := func(r *http.Request) bool {
		got := false
		limited.Next = http.HandlerFunc(func(http.ResponseWriter, *http.Request) { got = true })
		trust.Wrap(limited).ServeHTTP(httptest.NewRecorder(), r)
		return got
	}

	if !reached(forwardedRequest(t, "10.42.0.17:1", "X-Forwarded-For", "203.0.113.1")) {
		t.Fatal("the first person was refused")
	}
	if !reached(forwardedRequest(t, "10.42.0.17:2", "X-Forwarded-For", "203.0.113.2")) {
		t.Error("a second person behind the same ingress spent the first one's budget")
	}
	if reached(forwardedRequest(t, "10.42.0.17:3", "X-Forwarded-For", "203.0.113.1")) {
		t.Error("the first person was not limited")
	}

	if !reached(forwardedRequest(t, "198.51.100.4:1", "X-Forwarded-For", "203.0.113.50")) {
		t.Fatal("the untrusted peer was refused on its first request")
	}
	if reached(forwardedRequest(t, "198.51.100.4:2", "X-Forwarded-For", "203.0.113.51")) {
		t.Error("an untrusted peer bought a fresh budget by writing X-Forwarded-For")
	}
}

func TestAForwardedHeaderWithoutATrustedProxyWarns(t *testing.T) {
	trust := NewProxyTrust(nil)
	if len(trust.Warnings(t.Context())) != 0 {
		t.Fatal("a warning before any forwarded request")
	}
	resolvedThrough(t, trust, forwardedRequest(t, "10.42.0.17:5000", "", ""))
	if len(trust.Warnings(t.Context())) != 0 {
		t.Fatal("a warning for a request without a forwarded header")
	}
	if got := resolvedThrough(t, trust, forwardedRequest(t, "10.42.0.17:5000", "X-Forwarded-For", "203.0.113.9")); got != "10.42.0.17" {
		t.Errorf("client address = %q, want the peer while no proxy is trusted", got)
	}
	warnings := trust.Warnings(t.Context())
	if len(warnings) != 1 || warnings[0].Code != "config.trusted_proxies_missing" {
		t.Errorf("warnings = %+v, want config.trusted_proxies_missing", warnings)
	}
}
