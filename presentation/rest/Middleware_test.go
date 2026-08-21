// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// recordedRequest is one call to the metrics adapter, kept so a test can assert on the labels
// rather than on a scrape.
type recordedRequest struct {
	route   string
	method  string
	status  int
	seconds float64
}

type fakeMetrics struct {
	mu       sync.Mutex
	requests []recordedRequest
	inflight int64
	peak     int64
}

func (f *fakeMetrics) HTTPRequest(_ context.Context, route, method string, status int, seconds float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, recordedRequest{route, method, status, seconds})
}

func (f *fakeMetrics) InflightDelta(_ context.Context, _ string, delta int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inflight += delta
	if f.inflight > f.peak {
		f.peak = f.inflight
	}
}

func (f *fakeMetrics) only(t *testing.T) recordedRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) != 1 {
		t.Fatalf("expected exactly one recorded request, got %d", len(f.requests))
	}
	return f.requests[0]
}

func serve(t *testing.T, mux *http.ServeMux, req *http.Request) (*httptest.ResponseRecorder, *fakeMetrics) {
	t.Helper()
	metrics := &fakeMetrics{}
	handler := Observed{
		Router:  mux,
		Metrics: metrics,
		Tracer:  noop.NewTracerProvider().Tracer("test"),
		Role:    "api",
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec, metrics
}

// The route label is the template, never the resolved path. A path label carries identifiers,
// and an unbounded label is how a Prometheus dies (§3.2).
func TestTheRouteLabelIsTheTemplate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	_, metrics := serve(t, mux, httptest.NewRequestWithContext(
		context.Background(), http.MethodGet, "/items/01936f2a-7c1e-7000-8000-00000000000a", nil))

	got := metrics.only(t)
	if got.route != "GET /items/{id}" {
		t.Errorf("route = %q, want the template", got.route)
	}
	if strings.Contains(got.route, "01936f2a") {
		t.Errorf("an identifier reached the route label: %q", got.route)
	}
}

// A request that matches nothing still needs a bounded label: a scanner walking random paths
// must not create a series per path.
func TestAnUnmatchedRequestGetsABoundedLabel(t *testing.T) {
	_, metrics := serve(t, http.NewServeMux(), httptest.NewRequestWithContext(
		context.Background(), http.MethodGet, "/does/not/exist", nil))

	if got := metrics.only(t); got.route != routeUnmatched {
		t.Errorf("route = %q, want %q", got.route, routeUnmatched)
	}
}

// A handler that writes nothing produces a 200 on the wire, so the metric has to say 200 too -
// not the zero value of a status nobody set.
func TestAHandlerThatWritesNothingIsRecordedAs200(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /quiet", func(http.ResponseWriter, *http.Request) {})

	_, metrics := serve(t, mux, httptest.NewRequestWithContext(
		context.Background(), http.MethodGet, "/quiet", nil))

	if got := metrics.only(t); got.status != http.StatusOK {
		t.Errorf("status = %d, want 200", got.status)
	}
}

// The inflight gauge has to come back down, including when the handler panics - a gauge that
// only rises reads as an overload that is not happening.
func TestTheInflightGaugeReturnsToZero(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic("boom") })

	_, metrics := serve(t, mux, httptest.NewRequestWithContext(
		context.Background(), http.MethodGet, "/boom", nil))

	if metrics.inflight != 0 {
		t.Errorf("the inflight gauge stands at %d after the request", metrics.inflight)
	}
	if metrics.peak != 1 {
		t.Errorf("the inflight gauge peaked at %d, want 1", metrics.peak)
	}
}

// A panicking handler still owes the client an answer, and it has to be the RFC 9457 problem -
// a dropped connection tells a client nothing and a stack trace tells it too much.
func TestAPanicBecomesAProblemResponseAndIsCounted(t *testing.T) {
	var panics []string
	concurrency.SetPanicObserver(func(component string, _ any) {
		panics = append(panics, component)
	})
	t.Cleanup(func() { concurrency.SetPanicObserver(func(string, any) {}) })

	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic("boom") })

	rec, metrics := serve(t, mux, httptest.NewRequestWithContext(
		context.Background(), http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != ProblemContentType {
		t.Errorf("content type = %q, want %q", ct, ProblemContentType)
	}

	var problem Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("the body is not a problem document: %v", err)
	}
	if problem.RequestID == "" {
		t.Error("the problem carries no request ID, so the log cannot be found")
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("the panic value reached the client:\n%s", rec.Body.String())
	}

	if len(panics) != 1 || panics[0] != "rest.request" {
		t.Errorf("the panic counter saw %v, want one entry for rest.request", panics)
	}
	if got := metrics.only(t); got.status != http.StatusInternalServerError {
		t.Errorf("the metric recorded status %d, want 500", got.status)
	}
}

// A request ID a client already carries is kept, so one identifier spans the whole hop chain.
func TestAValidRequestIDIsAdopted(t *testing.T) {
	mux := http.NewServeMux()
	var seen string
	mux.HandleFunc("GET /items", func(_ http.ResponseWriter, r *http.Request) {
		seen = correlation.RequestIDFrom(r.Context())
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/items", nil)
	req.Header.Set(RequestIDHeader, "edge-proxy-42")
	rec, _ := serve(t, mux, req)

	if seen != "edge-proxy-42" {
		t.Errorf("the handler saw the request ID %q", seen)
	}
	if got := rec.Header().Get(RequestIDHeader); got != "edge-proxy-42" {
		t.Errorf("the response echoed %q", got)
	}
}

// An adopted request ID is written into a log line and into an error response. An arbitrary
// header value from outside would be a log injection and a reflected payload in one.
func TestAnUnsafeRequestIDIsReplaced(t *testing.T) {
	cases := map[string]string{
		"a newline":     "abc\ndef",
		"json":          `{"x":1}`,
		"a script tag":  "<script>alert(1)</script>",
		"far too long":  strings.Repeat("a", maxRequestIDLength+1),
		"a space":       "abc def",
		"a null byte":   "abc\x00def",
		"an empty head": "",
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			mux := http.NewServeMux()
			var seen string
			mux.HandleFunc("GET /items", func(_ http.ResponseWriter, r *http.Request) {
				seen = correlation.RequestIDFrom(r.Context())
			})

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/items", nil)
			req.Header.Set(RequestIDHeader, value)
			serve(t, mux, req)

			if seen == value {
				t.Errorf("the unsafe request ID %q was adopted", value)
			}
			if !isSafeRequestID(seen) {
				t.Errorf("the generated request ID is not safe either: %q", seen)
			}
		})
	}
}

// The incoming traceparent reaches the handler, which is what makes caller → API → job one
// trace rather than three (§3.3).
func TestTheIncomingTraceReachesTheHandler(t *testing.T) {
	// The propagator is installed once at startup by observability.NewTracing; the presentation
	// layer must not reach into infrastructure to do it, so the test installs the same one.
	otel.SetTextMapPropagator(propagation.TraceContext{})

	mux := http.NewServeMux()
	var seen string
	mux.HandleFunc("GET /items", func(_ http.ResponseWriter, r *http.Request) {
		seen = trace.SpanContextFromContext(r.Context()).TraceID().String()
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/items", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	serve(t, mux, req)

	if seen != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("the handler saw the trace %q", seen)
	}
}
