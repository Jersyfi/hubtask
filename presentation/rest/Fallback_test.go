// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest_test

import (
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
