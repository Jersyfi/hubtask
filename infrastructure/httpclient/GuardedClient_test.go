// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package httpclient_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	port "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// A test server listens on loopback, which is exactly what the guard refuses. Every test that
// wants to reach one therefore opens private networks deliberately - the same switch a
// self-hoster with an internal target uses. The refusal itself is tested with the default
// configuration, in TestALoopbackTargetIsRefusedByDefault and in the SSRF suite.
func openClient(t *testing.T, cfg env.OutboundConfig) *httpclient.GuardedClient {
	t.Helper()
	cfg.AllowPrivateNetworks = true
	if cfg.Timeout == 0 {
		cfg.Timeout = 2 * time.Second
	}
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = time.Second
	}
	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = 1 << 20
	}
	return httpclient.NewGuardedClient(cfg, httpclient.NewGuard(cfg))
}

func TestACallCarriesMethodHeadersAndBody(t *testing.T) {
	var seen *http.Request
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r
		body, _ = readAll(r)
		w.Header().Set("X-Answer", "42")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("received"))
	}))
	defer server.Close()

	client := openClient(t, env.OutboundConfig{})
	resp, err := client.Do(context.Background(), port.Request{
		Method:      http.MethodPost,
		URL:         server.URL + "/hook",
		Header:      map[string][]string{"X-Signature": {"sha256=abc"}},
		Body:        []byte(`{"event":"created"}`),
		TargetClass: "webhook",
	})
	if err != nil {
		t.Fatalf("the call failed: %v", err)
	}

	if seen.Method != http.MethodPost || seen.URL.Path != "/hook" {
		t.Errorf("the target saw %s %s", seen.Method, seen.URL.Path)
	}
	if got := seen.Header.Get("X-Signature"); got != "sha256=abc" {
		t.Errorf("signature header = %q", got)
	}
	if string(body) != `{"event":"created"}` {
		t.Errorf("body = %q", body)
	}
	if resp.Status != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.Status)
	}
	if string(resp.Body) != "received" {
		t.Errorf("response body = %q", resp.Body)
	}
	if resp.Header["X-Answer"][0] != "42" {
		t.Errorf("response header = %v", resp.Header)
	}
}

// A target that answers is a call that worked, whatever it thinks of the request. Only the
// caller knows whether a 500 from a webhook target is a failure worth retrying.
func TestAnUnhappyStatusIsNotATransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	resp, err := openClient(t, env.OutboundConfig{}).Do(context.Background(),
		port.Request{URL: server.URL, TargetClass: "webhook"})

	if err != nil {
		t.Fatalf("a 500 was reported as an error: %v", err)
	}
	if resp.Status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.Status)
	}
}

// Truncating and carrying on would hand the caller half a JSON document (T-17).
func TestAnOversizedResponseIsRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 4096))
	}))
	defer server.Close()

	client := openClient(t, env.OutboundConfig{MaxResponseBytes: 1024})
	_, err := client.Do(context.Background(), port.Request{URL: server.URL, TargetClass: "webhook"})

	if err == nil {
		t.Fatal("a response four times the limit was accepted")
	}
	if got := shared.AsError(err).DetailCode; got != "dependency.response_too_large" {
		t.Errorf("detail code = %q, want dependency.response_too_large", got)
	}
}

// The limit is a limit, not a threshold one byte below it.
func TestAResponseExactlyAtTheLimitIsAccepted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 1024))
	}))
	defer server.Close()

	client := openClient(t, env.OutboundConfig{MaxResponseBytes: 1024})
	resp, err := client.Do(context.Background(), port.Request{URL: server.URL, TargetClass: "webhook"})

	if err != nil {
		t.Fatalf("a response of exactly the limit was refused: %v", err)
	}
	if len(resp.Body) != 1024 {
		t.Errorf("body length = %d, want 1024", len(resp.Body))
	}
}

func TestRedirectsAreFollowedUpToTheBudget(t *testing.T) {
	var hops int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			_, _ = w.Write([]byte("arrived"))
			return
		}
		hops++
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer server.Close()

	client := openClient(t, env.OutboundConfig{MaxRedirects: 3})
	resp, err := client.Do(context.Background(), port.Request{URL: server.URL, TargetClass: "webhook"})

	if err != nil {
		t.Fatalf("a single redirect was not followed: %v", err)
	}
	if hops != 1 || string(resp.Body) != "arrived" {
		t.Errorf("hops = %d, body = %q", hops, resp.Body)
	}
}

// A chain longer than the budget is not a site that moved.
func TestAnEndlessRedirectChainIsCutOff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer server.Close()

	client := openClient(t, env.OutboundConfig{MaxRedirects: 2})
	_, err := client.Do(context.Background(), port.Request{URL: server.URL, TargetClass: "webhook"})

	if err == nil {
		t.Fatal("an endless redirect chain was followed to the end")
	}
	if got := shared.AsError(err).DetailCode; got != "dependency.too_many_redirects" {
		t.Errorf("detail code = %q, want dependency.too_many_redirects", got)
	}
}

// Zero redirects is the strictest setting and has to mean what it says.
func TestARedirectIsRefusedWhenTheBudgetIsZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			_, _ = w.Write([]byte("arrived"))
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer server.Close()

	client := openClient(t, env.OutboundConfig{MaxRedirects: 0})
	_, err := client.Do(context.Background(), port.Request{URL: server.URL, TargetClass: "webhook"})

	if err == nil {
		t.Fatal("a redirect was followed although the budget is zero")
	}
}

// The hop has to satisfy the same rules as the first request. A 302 into another scheme is the
// oldest trick in the book.
func TestARedirectIntoAnotherSchemeIsRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	defer server.Close()

	client := openClient(t, env.OutboundConfig{MaxRedirects: 3})
	_, err := client.Do(context.Background(), port.Request{URL: server.URL, TargetClass: "webhook"})

	if err == nil {
		t.Fatal("a redirect to file:// was followed")
	}
	if !httpclient.IsBlocked(err) {
		t.Errorf("error = %v, want a guard refusal that survives the transport", err)
	}
}

// The refusal has to keep its identity through the *url.Error the transport wraps it in -
// the dispatcher decides "disable this subscription" or "try again later" by exactly that.
func TestARedirectOffTheAllowlistIsRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://elsewhere.example.org/", http.StatusFound)
	}))
	defer server.Close()

	// The allowlist holds the test server's host and nothing else. The redirect target is never
	// resolved, let alone dialled: the hop is refused on its name.
	cfg := env.OutboundConfig{
		MaxRedirects:         3,
		AllowedHosts:         []string{hostOf(t, server.URL)},
		AllowPrivateNetworks: true,
		Timeout:              2 * time.Second,
		ConnectTimeout:       time.Second,
		MaxResponseBytes:     1 << 20,
	}
	client := httpclient.NewGuardedClient(cfg, httpclient.NewGuard(cfg))

	_, err := client.Do(context.Background(), port.Request{URL: server.URL, TargetClass: "webhook"})
	if err == nil {
		t.Fatal("a redirect to a host outside the allowlist was followed")
	}
	if got := shared.AsError(err).DetailCode; got != "dependency.target_not_allowed" {
		t.Errorf("detail code = %q, want dependency.target_not_allowed", got)
	}
}

func TestASlowTargetHitsTheBudget(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	client := openClient(t, env.OutboundConfig{Timeout: 50 * time.Millisecond})

	start := time.Now()
	_, err := client.Do(context.Background(), port.Request{URL: server.URL, TargetClass: "webhook"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a target that never answers was waited for indefinitely")
	}
	if elapsed > time.Second {
		t.Errorf("the call took %v - the budget was not enforced", elapsed)
	}
	domainErr := shared.AsError(err)
	if domainErr.Category != shared.CategoryUnavailable {
		t.Errorf("category = %s, want %s", domainErr.Category, shared.CategoryUnavailable)
	}
	if domainErr.DetailCode != "dependency.timeout" {
		t.Errorf("detail code = %q, want dependency.timeout", domainErr.DetailCode)
	}
}

// Without an explicit switch, a target on loopback is refused before a socket is opened - a
// webhook must not reach the installation's own admin interface.
func TestALoopbackTargetIsRefusedByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("should never be reached"))
	}))
	defer server.Close()

	cfg := env.OutboundConfig{Timeout: time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 1024}
	client := httpclient.NewGuardedClient(cfg, httpclient.NewGuard(cfg))

	_, err := client.Do(context.Background(), port.Request{URL: server.URL, TargetClass: "webhook"})
	if err == nil {
		t.Fatal("a loopback target was called")
	}
	if !httpclient.IsBlocked(err) {
		t.Errorf("error = %v, want a blocked target", err)
	}
}

func TestTheDurationIsReported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	var class string
	var seconds float64
	var calls int
	client := openClient(t, env.OutboundConfig{}).WithObserver(
		func(_ context.Context, targetClass string, s float64) {
			class, seconds, calls = targetClass, s, calls+1
		})

	if _, err := client.Do(context.Background(), port.Request{URL: server.URL, TargetClass: "webhook"}); err != nil {
		t.Fatalf("the call failed: %v", err)
	}

	if calls != 1 {
		t.Errorf("the observer was called %d times, want once", calls)
	}
	if class != "webhook" {
		t.Errorf("target class = %q, want webhook", class)
	}
	if seconds < 0 {
		t.Errorf("duration = %v", seconds)
	}
}

// A call without a class would produce an empty label, which reads as a bug in the dashboard
// rather than as a caller that forgot something.
func TestACallWithoutAClassIsStillLabelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	var class string
	client := openClient(t, env.OutboundConfig{}).WithObserver(
		func(_ context.Context, targetClass string, _ float64) { class = targetClass })

	if _, err := client.Do(context.Background(), port.Request{URL: server.URL}); err != nil {
		t.Fatalf("the call failed: %v", err)
	}
	if class != "unclassified" {
		t.Errorf("target class = %q, want unclassified", class)
	}
}

// GuardedClient is the adapter of the port, and nothing but the port should be visible to a
// caller in the core.
func TestGuardedClientImplementsThePort(t *testing.T) {
	var _ port.Port = httpclient.NewGuardedClient(env.OutboundConfig{}, httpclient.NewGuard(env.OutboundConfig{}))
}

func hostOf(t *testing.T, rawURL string) string {
	t.Helper()
	trimmed := strings.TrimPrefix(rawURL, "http://")
	host, _, found := strings.Cut(trimmed, ":")
	if !found {
		t.Fatalf("%s has no port", rawURL)
	}
	return host
}

func readAll(r *http.Request) ([]byte, error) {
	defer func() { _ = r.Body.Close() }()
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}

// A webhook recipient that understands W3C trace context should be able to join the trace, and
// two Hubtask installations calling each other should produce one trace rather than two (§3.3).
func TestTheTraceContextTravelsWithTheCall(t *testing.T) {
	var seen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("traceparent")
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	// The process installs the propagator at startup, whether tracing exports anything or not
	// (observability.NewTracing). A unit test builds no process, so it installs it here.
	previous := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previous) })

	provider := sdktrace.NewTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()
	client := openClient(t, env.OutboundConfig{}).WithTracer(provider.Tracer("test"))

	ctx, span := provider.Tracer("test").Start(context.Background(), "caller")
	defer span.End()

	if _, err := client.Do(ctx, port.Request{URL: server.URL, TargetClass: "webhook"}); err != nil {
		t.Fatalf("the call failed: %v", err)
	}

	if seen == "" {
		t.Fatal("the target received no traceparent header")
	}
	if traceID := span.SpanContext().TraceID().String(); !strings.Contains(seen, traceID) {
		t.Errorf("traceparent = %q, want the caller's trace %s", seen, traceID)
	}
}

// A URL can carry a token in its query string, and a span is stored and read by more people
// than a log line (security.md §12).
func TestTheSpanCarriesNoURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	client := openClient(t, env.OutboundConfig{}).WithTracer(provider.Tracer("test"))
	if _, err := client.Do(context.Background(), port.Request{
		URL: server.URL + "/deliver?token=hunter2", TargetClass: "webhook",
	}); err != nil {
		t.Fatalf("the call failed: %v", err)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want exactly one client span", len(spans))
	}
	for _, attr := range spans[0].Attributes() {
		if strings.Contains(attr.Value.AsString(), "hunter2") {
			t.Errorf("the span attribute %s carries the query string: %s", attr.Key, attr.Value.AsString())
		}
	}
	if spans[0].SpanKind() != trace.SpanKindClient {
		t.Errorf("span kind = %v, want client", spans[0].SpanKind())
	}
}
