// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestActionSegmentGivesTheActionItsOwnSegment(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"action after a wildcard", "/items/{itemId}:complete", "/items/{itemId}/:complete"},
		{"action on a collection", "/items:query", "/items/:query"},
		{"action on a single segment", "/audit:verify", "/audit/:verify"},
		{"a plain resource is untouched", "/containers/{containerId}", "/containers/{containerId}"},
		{"a nested resource is untouched", "/items/{itemId}/comments", "/items/{itemId}/comments"},
		{"already rewritten stays put", "/items/{itemId}/:complete", "/items/{itemId}/:complete"},
		{"the root is untouched", "/", "/"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := actionSegment(c.in); got != c.want {
				t.Errorf("actionSegment(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestRoutablePatternKeepsTheMethod(t *testing.T) {
	if got := routablePattern("POST /api/v1/items/{itemId}:complete"); got != "POST /api/v1/items/{itemId}/:complete" {
		t.Errorf("got %q", got)
	}
	if got := routablePattern("GET /api/v1/containers"); got != "GET /api/v1/containers" {
		t.Errorf("got %q", got)
	}
}

// The rewrite exists because net/http refuses the specification's own form. If that ever stops
// being true, this test says so and the whole detour can go.
func TestTheSpecificationsActionFormIsStillUnroutable(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("net/http now accepts an action suffix - Mux can be simplified away")
		}
	}()
	http.NewServeMux().HandleFunc("POST /items/{itemId}:complete", func(http.ResponseWriter, *http.Request) {})
}

func TestMuxRoutesAnActionAndBindsThePathValue(t *testing.T) {
	mux := NewMux()
	var seen string
	mux.HandleFunc("POST /api/v1/items/{itemId}:complete", func(w http.ResponseWriter, r *http.Request) {
		seen = r.PathValue("itemId")
		w.WriteHeader(http.StatusNoContent)
	})

	for _, path := range []string{
		"/api/v1/items/018f0000-0000-7000-8000-000000000001:complete",
		// A client that percent-encoded the colon reaches the same route.
		"/api/v1/items/018f0000-0000-7000-8000-000000000001%3Acomplete",
	} {
		seen = ""
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil))

		if response.Code != http.StatusNoContent {
			t.Errorf("%s: status %d, want 204", path, response.Code)
		}
		if seen != "018f0000-0000-7000-8000-000000000001" {
			t.Errorf("%s: itemId %q", path, seen)
		}
	}
}

// An action must not be reachable as the identifier of the resource itself - otherwise
// `/items/{id}` would swallow `/items/{id}:complete` and the metric would count it as a read.
func TestAnActionDoesNotFallIntoTheResourceRoute(t *testing.T) {
	mux := NewMux()
	mux.HandleFunc("GET /api/v1/items/{itemId}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/items/abc:complete", nil))

	if response.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", response.Code)
	}
}

func TestMuxReportsTheSpecificationsRouteTemplate(t *testing.T) {
	mux := NewMux()
	mux.HandleFunc("POST /api/v1/items/{itemId}:complete", func(http.ResponseWriter, *http.Request) {})

	_, route := mux.Handler(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/items/x:complete", nil))
	if route != "POST /api/v1/items/{itemId}:complete" {
		t.Errorf("route %q - the rewritten form must not reach a metric label", route)
	}

	_, unmatched := mux.Handler(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/nothing", nil))
	if unmatched != "" {
		t.Errorf("unmatched route reported as %q", unmatched)
	}
}

func TestRoutesReportTheSpecificationsTemplates(t *testing.T) {
	mux := NewMux()
	mux.HandleFunc("GET /api/v1/containers", func(http.ResponseWriter, *http.Request) {})
	mux.HandleFunc("POST /api/v1/items:query", func(http.ResponseWriter, *http.Request) {})

	want := []string{"GET /api/v1/containers", "POST /api/v1/items:query"}
	got := mux.Routes()
	if len(got) != len(want) {
		t.Fatalf("routes %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("route %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAnUnknownRouteIsAProblemDocument(t *testing.T) {
	mux := NewMux()
	mux.HandleFunc("GET /api/v1/containers", func(http.ResponseWriter, *http.Request) {})

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/nothing", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != ProblemContentType {
		t.Errorf("content type %q, want %q", got, ProblemContentType)
	}

	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("the body is not a problem document: %v", err)
	}
	if problem.Code != "not_found" {
		t.Errorf("code %q", problem.Code)
	}
	if problem.DetailCode != "route.unknown" {
		t.Errorf("detail code %q", problem.DetailCode)
	}
}

func TestAWrongMethodIsA405WithAllow(t *testing.T) {
	mux := NewMux()
	mux.HandleFunc("POST /api/v1/containers", func(http.ResponseWriter, *http.Request) {})

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/api/v1/containers", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 405", response.Code)
	}
	if allow := response.Header().Get("Allow"); !strings.Contains(allow, http.MethodPost) {
		t.Errorf("Allow = %q, want it to name POST", allow)
	}

	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("the body is not a problem document: %v", err)
	}
	if problem.Code != codeMethodNotAllowed {
		t.Errorf("code %q", problem.Code)
	}
}

// The router is built from the generated registration list, so every route the specification
// declares has to be on it - and nothing else.
func TestTheControllerRegistersTheSpecificationsRoutes(t *testing.T) {
	routes := NewRestController().Routes().Routes()

	if len(routes) < 30 {
		t.Fatalf("only %d routes registered - the generated router did not run", len(routes))
	}

	seen := map[string]bool{}
	for _, route := range routes {
		if seen[route] {
			t.Errorf("route %q registered twice", route)
		}
		seen[route] = true
		if !strings.HasPrefix(strings.SplitN(route, " ", 2)[1], APIBasePath+"/") {
			t.Errorf("route %q is not below %s", route, APIBasePath)
		}
	}
	if !seen["GET "+APIBasePath+"/meta/capabilities"] {
		t.Error("the capability manifest is not routed")
	}
}

// Until a use case lands, an operation the specification declares answers 404 rather than a
// panic or an empty 200.
//
// The example is a backup route rather than a container one: /containers is served since B-04, and this
// test needs an operation that genuinely has no use case yet. It also has to be one with no path
// parameter - a probe against `{containerId}` fails to bind before it reaches the pending set.
func TestAPendingOperationAnswersAProblem(t *testing.T) {
	response := httptest.NewRecorder()
	NewRestController().Routes().ServeHTTP(response,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/backup-targets", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", response.Code)
	}
	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("the body is not a problem document: %v", err)
	}
	if problem.DetailCode != "route.operation_not_available" {
		t.Errorf("detail code %q", problem.DetailCode)
	}
}
