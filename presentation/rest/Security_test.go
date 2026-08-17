// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	env "github.com/Jersyfi/hubtask/core/port/environment"
)

func secured(t *testing.T, cors env.CORSConfig, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	Secured{
		CORS: cors,
		Next: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}.ServeHTTP(response, r)
	return response
}

func TestTheSecurityHeadersAreOnEveryAnswer(t *testing.T) {
	response := secured(t, env.CORSConfig{},
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/meta/capabilities", nil))

	want := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"Referrer-Policy":              "no-referrer",
		"Cross-Origin-Resource-Policy": "same-site",
		"Strict-Transport-Security":    hstsValue,
		"Content-Security-Policy":      contentSecurityPolicy,
		"Permissions-Policy":           permissionsPolicy,
		"Server":                       serverName,
	}
	for name, value := range want {
		if got := response.Header().Get(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
}

// The version belongs behind /meta, not on every answer (security.md §9).
func TestTheServerHeaderCarriesNoVersion(t *testing.T) {
	response := secured(t, env.CORSConfig{},
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/meta/capabilities", nil))

	if got := response.Header().Get("Server"); got != "Hubtask" {
		t.Errorf("Server = %q - it must name the product and nothing else", got)
	}
}

func TestCrossOriginIsClosedByDefault(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/meta/capabilities", nil)
	request.Header.Set("Origin", "https://app.example.com")

	response := secured(t, env.CORSConfig{}, request)

	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q with an empty allowlist", got)
	}
	if response.Code != http.StatusOK {
		t.Errorf("status %d - a closed allowlist must not block the request itself, only the browser", response.Code)
	}
}

func TestAnAllowedOriginIsEchoedWithVary(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/meta/capabilities", nil)
	request.Header.Set("Origin", "https://app.example.com")

	response := secured(t, env.CORSConfig{
		AllowedOrigins: []string{"https://other.example.com", "https://app.example.com"},
	}, request)

	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
	if got := response.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q - without it a cache serves one origin's answer to another", got)
	}
	if got := response.Header().Get("Access-Control-Expose-Headers"); got == "" {
		t.Error("no exposed headers - a browser client cannot read its own request ID")
	}
}

// The classic mistake: `https://evil-app.example.com` must not pass because it ends with an
// allowed origin, and `https://app.example.com.evil.test` must not pass because it starts with
// one.
func TestASimilarOriginIsNotAnAllowedOrigin(t *testing.T) {
	allowlist := env.CORSConfig{AllowedOrigins: []string{"https://app.example.com"}}

	for _, origin := range []string{
		"https://evil-app.example.com",
		"https://app.example.com.evil.test",
		"http://app.example.com",
		"https://app.example.com:8443",
		"https://app.example.com/",
	} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/meta/capabilities", nil)
		request.Header.Set("Origin", origin)

		if got := secured(t, allowlist, request).Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("%s was allowed as %q", origin, got)
		}
	}
}

func TestAPreflightIsAnsweredWithoutReachingTheRouter(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/api/v1/containers", nil)
	request.Header.Set("Origin", "https://app.example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)

	reached := false
	response := httptest.NewRecorder()
	Secured{
		CORS: env.CORSConfig{AllowedOrigins: []string{"https://app.example.com"}, MaxAge: 10 * time.Minute},
		Next: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }),
	}.ServeHTTP(response, request)

	if reached {
		t.Error("the preflight reached the router - it would answer 405")
	}
	if response.Code != http.StatusNoContent {
		t.Errorf("status %d, want 204", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("no allowed request headers - a browser cannot send Authorization")
	}
	if got := response.Header().Get("Access-Control-Max-Age"); got != "600" {
		t.Errorf("Access-Control-Max-Age = %q", got)
	}
}

func TestAPreflightFromAnUnknownOriginIsRefused(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/api/v1/containers", nil)
	request.Header.Set("Origin", "https://evil.test")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)

	response := secured(t, env.CORSConfig{AllowedOrigins: []string{"https://app.example.com"}}, request)

	if response.Code != http.StatusForbidden {
		t.Errorf("status %d, want 403", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q for an unknown origin", got)
	}
}

// A bearer API has no cookie to send, and the absence of this header is what keeps the wildcard
// safe.
func TestCredentialsAreNeverAllowed(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/meta/capabilities", nil)
	request.Header.Set("Origin", "https://app.example.com")

	for _, cors := range []env.CORSConfig{
		{AllowedOrigins: []string{"https://app.example.com"}},
		{AllowedOrigins: []string{env.CORSWildcard}},
	} {
		response := secured(t, cors, request)
		if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "" {
			t.Errorf("Access-Control-Allow-Credentials = %q", got)
		}
	}
}

func TestTheWildcardAllowsEveryOrigin(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/meta/capabilities", nil)
	request.Header.Set("Origin", "https://anything.test")

	response := secured(t, env.CORSConfig{AllowedOrigins: []string{env.CORSWildcard}}, request)

	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
}
