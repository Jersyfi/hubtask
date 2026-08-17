// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

func TestADeclaredBodyOverTheLimitIsRefusedBeforeItIsRead(t *testing.T) {
	reached := false
	response := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/items",
		strings.NewReader(strings.Repeat("x", 4096)))

	Bounded{
		MaxBodyBytes: 1024,
		Next:         http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }),
	}.ServeHTTP(response, request)

	if reached {
		t.Error("the handler saw a body that is over the limit")
	}
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413", response.Code)
	}

	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("the body is not a problem document: %v", err)
	}
	if problem.Code != codePayloadTooLarge {
		t.Errorf("code %q", problem.Code)
	}
	if problem.Params["limit_bytes"] != "1024" {
		t.Errorf("params %v - a client cannot guess the limit", problem.Params)
	}
}

// A chunked body declares no length, so the limit has to bite while it is being read.
func TestAnUndeclaredBodyIsCutOffAtTheLimit(t *testing.T) {
	var readErr error
	response := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/items",
		strings.NewReader(strings.Repeat("x", 4096)))
	request.ContentLength = -1

	Bounded{
		MaxBodyBytes: 1024,
		Next: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			_, readErr = io.ReadAll(r.Body)
		}),
	}.ServeHTTP(response, request)

	if readErr == nil {
		t.Error("the whole body was readable although it is over the limit")
	}
}

func TestABodyWithinTheLimitPassesThrough(t *testing.T) {
	var body []byte
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/items",
		strings.NewReader("small"))

	Bounded{
		MaxBodyBytes: 1024,
		Next: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			body, _ = io.ReadAll(r.Body)
		}),
	}.ServeHTTP(httptest.NewRecorder(), request)

	if string(body) != "small" {
		t.Errorf("body = %q", body)
	}
}

// Rule 7 of CLAUDE.md: no call without a deadline. A handler that forgets one still gets it.
func TestEveryHandlerInheritsADeadline(t *testing.T) {
	var deadline time.Time
	var hasDeadline bool

	Bounded{
		MaxBodyBytes: 1024,
		Timeout:      30 * time.Second,
		Next: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			deadline, hasDeadline = r.Context().Deadline()
		}),
	}.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/containers", nil))

	if !hasDeadline {
		t.Fatal("the handler ran without a deadline")
	}
	if time.Until(deadline) > 30*time.Second {
		t.Errorf("deadline is %v away, want at most 30s", time.Until(deadline))
	}
}

func TestPreferredLocale(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"no header falls back", "", "en"},
		{"a single tag", "de", "de"},
		{"a region tag", "pt-BR", "pt-BR"},
		{"the highest weight wins", "de;q=0.3, fr;q=0.9, en;q=0.5", "fr"},
		{"equal weights keep the client's order", "de, fr", "de"},
		{"an unweighted tag beats a weighted one", "de;q=0.8, fr", "fr"},
		{"q=0 means not acceptable", "de;q=0", "en"},
		{"the wildcard is not a language", "*", "en"},
		{"a script subtag", "zh-Hans-CN", "zh-Hans-CN"},
		{"rubbish falls back", "!!;q=x", "en"},
		{"a header that is too long falls back", strings.Repeat("de,", 200), "en"},
		{"an injection attempt falls back", "de\nX-Evil: 1", "en"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := preferredLocale(c.header, "en"); got != c.want {
				t.Errorf("preferredLocale(%q) = %q, want %q", c.header, got, c.want)
			}
		})
	}
}

func TestTheLocaleReachesTheActorContext(t *testing.T) {
	var actor appshared.ActorContext
	var found bool

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/meta/capabilities", nil)
	request.Header.Set("Accept-Language", "de-AT;q=0.9, en;q=0.4")

	Localised{
		Locale: env.LocaleConfig{DefaultLocale: "en", DefaultTimeZone: "Europe/Berlin"},
		Next: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			actor, found = appshared.ActorFrom(r.Context())
		}),
	}.ServeHTTP(httptest.NewRecorder(), request)

	if !found {
		t.Fatal("no actor in the context")
	}
	if actor.Locale != "de-AT" {
		t.Errorf("locale %q", actor.Locale)
	}
	if actor.TimeZone != "Europe/Berlin" {
		t.Errorf("time zone %q", actor.TimeZone)
	}
	if actor.IsAuthenticated() {
		t.Error("resolving a language authenticated the request")
	}
}
