// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The middleware is tested against a shedder of its own rather than against the one in
// infrastructure: an inbound adapter does not import the outbound side (project-structure.md §2),
// and what is under test here is the refusal's shape, not the counting. The counting has its own
// tests beside the shedder.
type fakeShedder struct {
	mu         sync.Mutex
	inflight   int
	limit      int
	retryAfter int
	admitted   int
	released   int
}

func (f *fakeShedder) admit(deferrable bool) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if deferrable && f.inflight >= f.limit {
		return func() {}, shared.ErrUnavailable.
			WithDetail("capacity.shed").
			WithParams(map[string]string{
				"class":               "deferrable",
				"retry_after_seconds": strconv.Itoa(f.retryAfter),
			})
	}
	f.inflight++
	f.admitted++
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.inflight--
		f.released++
	}, nil
}

type shedSignal struct {
	scopes []string
}

func (s *shedSignal) RateLimited(_ context.Context, scope string) { s.scopes = append(s.scopes, scope) }

// routes builds a router carrying one interactive and one deferrable template, so that the
// classification is exercised through the same lookup the server uses.
func shedRoutes(served *int) *Mux {
	mux := NewMux()
	handler := func(w http.ResponseWriter, _ *http.Request) {
		*served++
		w.WriteHeader(http.StatusOK)
	}
	mux.HandleFunc(http.MethodPost+" "+APIBasePath+"/items:query", handler)
	mux.HandleFunc(http.MethodPost+" "+APIBasePath+"/items", handler)
	return mux
}

func shedRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	return httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, nil)
}

// The whole point of the classification: the query is refused at the threshold and the create is
// not. A person ticking off a task under load must still be answered.
func TestOnlyDeferrableWorkIsShed(t *testing.T) {
	served := 0
	shedder := &fakeShedder{limit: 0, retryAfter: 7}
	middleware := Shedding{Next: shedRoutes(&served), Routes: shedRoutes(&served), Admit: shedder.admit}

	deferrable := httptest.NewRecorder()
	middleware.ServeHTTP(deferrable, shedRequest(t, APIBasePath+"/items:query"))
	if deferrable.Code != http.StatusServiceUnavailable {
		t.Errorf("the query answered %d, want 503", deferrable.Code)
	}
	if served != 0 {
		t.Error("the shed request reached the handler; the refusal has to cost nothing")
	}

	interactive := httptest.NewRecorder()
	middleware.ServeHTTP(interactive, shedRequest(t, APIBasePath+"/items"))
	if interactive.Code != http.StatusOK {
		t.Errorf("creating an item answered %d while the process was busy, want 200", interactive.Code)
	}
}

// A refusal a client cannot act on is an outage with a status code. Retry-After and the problem
// document are what make it a back-pressure signal instead (api-guidelines.md §6).
func TestAShedRequestCarriesRetryAfterAndAProblemDocument(t *testing.T) {
	served := 0
	shedder := &fakeShedder{limit: 0, retryAfter: 7}
	middleware := Shedding{Next: shedRoutes(&served), Routes: shedRoutes(&served), Admit: shedder.admit}

	response := httptest.NewRecorder()
	middleware.ServeHTTP(response, shedRequest(t, APIBasePath+"/items:query"))

	if got := response.Header().Get("Retry-After"); got != "7" {
		t.Errorf("Retry-After = %q, want 7", got)
	}
	var problem map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("the refusal is not a problem document: %v", err)
	}
	// The code and not the detail code: every 5xx is stripped of its detail and its parameters on
	// the way out, because a five hundred may have been raised by a driver (Problem.go). The
	// actionable half of a shed answer therefore travels in the header, which is why the header
	// is the assertion above and not a nicety.
	if problem["code"] != "dependency_unavailable" {
		t.Errorf("code = %v, want dependency_unavailable", problem["code"])
	}
	if problem["status"] != float64(http.StatusServiceUnavailable) {
		t.Errorf("status = %v, want 503", problem["status"])
	}
}

// The counter is what an operator sees before the latency does, so a refusal that is not counted
// is a refusal nobody knows about (observability-reliability.md §6).
func TestAShedRequestIsCountedByItsClass(t *testing.T) {
	served := 0
	signals := &shedSignal{}
	shedder := &fakeShedder{limit: 0, retryAfter: 5}
	middleware := Shedding{
		Next: shedRoutes(&served), Routes: shedRoutes(&served),
		Admit: shedder.admit, Signals: signals,
	}

	middleware.ServeHTTP(httptest.NewRecorder(), shedRequest(t, APIBasePath+"/items:query"))
	middleware.ServeHTTP(httptest.NewRecorder(), shedRequest(t, APIBasePath+"/items"))

	if len(signals.scopes) != 1 || signals.scopes[0] != "load_shed:deferrable" {
		t.Errorf("counted %v, want one load_shed:deferrable", signals.scopes)
	}
}

// The admission has to come back down when the response is written. A middleware that leaked one
// slot per request would shed everything after a few minutes of ordinary traffic, and the first
// symptom would be exports failing on an idle installation.
func TestTheAdmissionIsReleasedAfterTheResponse(t *testing.T) {
	served := 0
	shedder := &fakeShedder{limit: 1, retryAfter: 5}
	middleware := Shedding{Next: shedRoutes(&served), Routes: shedRoutes(&served), Admit: shedder.admit}

	for range 3 {
		response := httptest.NewRecorder()
		middleware.ServeHTTP(response, shedRequest(t, APIBasePath+"/items:query"))
		if response.Code != http.StatusOK {
			t.Fatalf("a query on an idle process answered %d", response.Code)
		}
	}
	if shedder.released != shedder.admitted {
		t.Errorf("%d admitted, %d released", shedder.admitted, shedder.released)
	}
}

// Zero as a threshold means "off", not "refuse everything" - the configuration says so, and this
// is the half of it that lives in the middleware.
func TestSheddingSwitchedOffServesEverything(t *testing.T) {
	served := 0
	middleware := Shedding{Next: shedRoutes(&served), Routes: shedRoutes(&served), Admit: nil}

	response := httptest.NewRecorder()
	middleware.ServeHTTP(response, shedRequest(t, APIBasePath+"/items:query"))
	if response.Code != http.StatusOK {
		t.Errorf("the query answered %d with shedding off, want 200", response.Code)
	}
}

// An unmatched path has no template, so it classifies as interactive and is served - which is
// what lets the router answer its own 404 rather than a 503 that says the wrong thing.
func TestAnUnknownRouteIsNotShed(t *testing.T) {
	served := 0
	shedder := &fakeShedder{limit: 0, retryAfter: 5}
	middleware := Shedding{Next: shedRoutes(&served), Routes: shedRoutes(&served), Admit: shedder.admit}

	response := httptest.NewRecorder()
	middleware.ServeHTTP(response, shedRequest(t, APIBasePath+"/nothing-here"))
	if response.Code == http.StatusServiceUnavailable {
		t.Error("an unknown route was shed; it has no class and cannot be deferred")
	}
}
