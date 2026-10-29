// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/presentation/rest"
)

// stubRouter answers everything it is asked, and reports which template matched.
type stubRouter struct{ template string }

func (s stubRouter) Handler(*http.Request) (http.Handler, string) {
	return http.HandlerFunc(s.ServeHTTP), s.template
}

func (s stubRouter) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("X-Answered-By", "api")
	w.WriteHeader(http.StatusOK)
}

func stubUI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Answered-By", "ui")
		w.WriteHeader(http.StatusOK)
	})
}

func fallback(ui http.Handler) rest.Fallback {
	api := stubRouter{template: "GET " + rest.APIBasePath + "/items"}
	return rest.Fallback{
		API:      api,
		Reserved: []string{rest.APIBasePath + "/", "/mcp"},
		Serve:    api,
		UI:       ui,
	}
}

func answeredBy(t *testing.T, h http.Handler, path string) (string, int) {
	t.Helper()
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return recorder.Header().Get("X-Answered-By"), recorder.Code
}

func TestTheAPIIsNeverShadowedByTheInterface(t *testing.T) {
	t.Parallel()
	f := fallback(stubUI())

	// Every one of these belongs to the API, and an unmatched path under /api/ has to keep
	// getting the API's own answer rather than a document a client cannot parse (ADR-0028).
	for _, path := range []string{
		rest.APIBasePath + "/items",
		rest.APIBasePath + "/meta/capabilities",
		rest.APIBasePath + "/nothing-here",
		rest.APIBasePath,
		rest.APIBasePath + "/",
		"/mcp",
	} {
		if by, _ := answeredBy(t, f, path); by != "api" {
			t.Errorf("GET %s was answered by %q, want the API", path, by)
		}
	}
}

func TestEverythingElseReachesTheInterface(t *testing.T) {
	t.Parallel()
	f := fallback(stubUI())

	for _, path := range []string{"/", "/index.html", "/assets/index-CBRxRnMw.js", "/containers/01JB", "/apiary"} {
		if by, _ := answeredBy(t, f, path); by != "ui" {
			t.Errorf("GET %s was answered by %q, want the interface", path, by)
		}
	}
}

func TestASwitchedOffInterfaceIs404AndNot401(t *testing.T) {
	t.Parallel()
	f := fallback(nil)

	// The distinction matters. Passing the request down the chain would authenticate first and
	// answer 401, which tells a visitor there is something there to log in to. There is not.
	by, status := answeredBy(t, f, "/")
	if status != http.StatusNotFound {
		t.Errorf("GET / with the interface off = %d, want 404", status)
	}
	if by == "api" {
		t.Error("the request reached the API chain; it should be answered by the fallback itself")
	}
}

func TestASwitchedOffInterfaceLeavesTheAPIAlone(t *testing.T) {
	t.Parallel()
	f := fallback(nil)

	if by, status := answeredBy(t, f, rest.APIBasePath+"/items"); by != "api" || status != http.StatusOK {
		t.Errorf("the API answered %q/%d with the interface off", by, status)
	}
}

func TestTheRefusalCarriesTheSecurityHeadersAndAProblemBody(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	fallback(nil).ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	if got := recorder.Header().Get("Content-Security-Policy"); got == "" {
		t.Error("no content security policy on the refusal")
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/problem+json") {
		t.Errorf("Content-Type = %q, want a problem document", got)
	}
	if !strings.Contains(recorder.Body.String(), "route.unknown") {
		t.Errorf("body = %q, want the route.unknown message code", recorder.Body.String())
	}
}

func TestTheInterfaceIsRecordedUnderOneRouteTemplate(t *testing.T) {
	t.Parallel()
	f := fallback(stubUI())

	// A single-page application invents its own paths, so using them as a metric label would make
	// the cardinality somebody else's decision (observability-reliability.md §3.2).
	for _, path := range []string{"/", "/containers/01JB", "/anything/at/all"} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		if _, template := f.Handler(request); template != rest.UIRoute {
			t.Errorf("GET %s: template = %q, want %q", path, template, rest.UIRoute)
		}
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, rest.APIBasePath+"/items", nil)
	if _, template := f.Handler(request); template != "GET "+rest.APIBasePath+"/items" {
		t.Errorf("the API's own template did not survive: %q", template)
	}
}

// redirectsFake sends a browser from one host to another base.
type redirectsFake map[string]string

func (f redirectsFake) RedirectFor(_ context.Context, host string) (string, error) {
	return f[host], nil
}

// UC-INS-15 check 7: a page load on the subdomain beside a main own domain is sent on to the same
// path there; the API on that host still answers, and so do the assets and any other host.
func TestAPageLoadBesideTheMainAddressIsSentOn(t *testing.T) {
	t.Parallel()
	f := fallback(stubUI())
	f.Browsers = redirectsFake{"acme.tasks.example.net": "https://tasks.acme.com"}

	ask := func(host, path, mode string) *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		request.Host = host
		if mode != "" {
			request.Header.Set("Sec-Fetch-Mode", mode)
		}
		recorder := httptest.NewRecorder()
		f.ServeHTTP(recorder, request)
		return recorder
	}

	page := ask("acme.tasks.example.net", "/hubs/1?view=board", "navigate")
	if page.Code != http.StatusFound || page.Header().Get("Location") != "https://tasks.acme.com/hubs/1?view=board" {
		t.Errorf("a page load answered %d to %q", page.Code, page.Header().Get("Location"))
	}
	if api := ask("acme.tasks.example.net", rest.APIBasePath+"/items", "cors"); api.Header().Get("X-Answered-By") != "api" {
		t.Errorf("the API on the subdomain answered %d by %q", api.Code, api.Header().Get("X-Answered-By"))
	}
	if asset := ask("acme.tasks.example.net", "/assets/app.js", "no-cors"); asset.Header().Get("X-Answered-By") != "ui" {
		t.Errorf("an asset answered %d", asset.Code)
	}
	if main := ask("tasks.acme.com", "/", "navigate"); main.Header().Get("X-Answered-By") != "ui" {
		t.Errorf("the main address answered %d", main.Code)
	}
}
