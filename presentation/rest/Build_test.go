// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func buildRoutes() *Mux {
	mux := NewMux()
	mux.HandleFunc(http.MethodGet+" "+APIBasePath+"/items", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func buildRequest(path, clientBuild string) *http.Request {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	if clientBuild != "" {
		r.Header.Set(ClientBuildHeader, clientBuild)
	}
	return r
}

// During a rolling update a client of the new release may reach a pod of the old one, which does
// not know a route the new release added. That is not "there is nothing here": the answer is a
// 503 the client retries, so the next attempt can land on a pod that knows the route.
func TestAnUnknownRouteFromAnotherBuildIsRetriedRatherThanMissing(t *testing.T) {
	routes := buildRoutes()
	cases := []struct {
		name        string
		path        string
		clientBuild string
		want        int
	}{
		{"another build, unknown route", APIBasePath + "/new-thing", "/assets/index-NEW00000.js", http.StatusServiceUnavailable},
		{"same build, unknown route", APIBasePath + "/new-thing", "/assets/index-OLD00000.js", http.StatusNotFound},
		{"no build named, unknown route", APIBasePath + "/new-thing", "", http.StatusNotFound},
		{"another build, known route", APIBasePath + "/items", "/assets/index-NEW00000.js", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			middleware := BuildMarked{Build: "/assets/index-OLD00000.js", Routes: routes, Next: routes}
			response := httptest.NewRecorder()
			middleware.ServeHTTP(response, buildRequest(tc.path, tc.clientBuild))

			if response.Code != tc.want {
				t.Fatalf("answered %d, want %d", response.Code, tc.want)
			}
			if got := response.Header().Get(BuildHeader); got != "/assets/index-OLD00000.js" {
				t.Errorf("%s = %q, want the server's build on every answer", BuildHeader, got)
			}
			if tc.want != http.StatusServiceUnavailable {
				return
			}
			if got := response.Header().Get("Retry-After"); got != "1" {
				t.Errorf("Retry-After = %q, want 1", got)
			}
			var problem struct {
				DetailCode string `json:"detail_code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
				t.Fatalf("the body is not a problem document: %v", err)
			}
			if problem.DetailCode != "route.build_mismatch" {
				t.Errorf("detail_code = %q, want route.build_mismatch", problem.DetailCode)
			}
		})
	}
}

// A binary built without a bundle has no build to compare with, and then an unknown route stays
// what it is.
func TestABinaryWithoutABuildAnswersAnUnknownRouteAsMissing(t *testing.T) {
	routes := buildRoutes()
	middleware := BuildMarked{Routes: routes, Next: routes}
	response := httptest.NewRecorder()
	middleware.ServeHTTP(response, buildRequest(APIBasePath+"/new-thing", "/assets/index-NEW00000.js"))
	if response.Code != http.StatusNotFound {
		t.Fatalf("answered %d, want 404", response.Code)
	}
	if got := response.Header().Get(BuildHeader); got != "" {
		t.Errorf("%s = %q, want none", BuildHeader, got)
	}
}
