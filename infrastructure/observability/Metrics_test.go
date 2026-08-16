// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	env "github.com/Jersyfi/hubtask/core/port/environment"
)

func newTestMetrics(t *testing.T, cfg env.Config) *Metrics {
	t.Helper()
	cfg.Version, cfg.Commit = "1.2.3", "abc1234"
	m, err := NewMetrics(cfg)
	if err != nil {
		t.Fatalf("building the metrics: %v", err)
	}
	t.Cleanup(func() { _ = m.Shutdown(context.Background()) })
	return m
}

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("scrape returned %d", rec.Code)
	}
	return rec.Body.String()
}

// The acceptance criterion of A-04: the panic counter exists, and it reads zero.
func TestThePanicCounterExistsAndStaysAtZero(t *testing.T) {
	m := newTestMetrics(t, env.Config{})

	body := scrape(t, m)

	if !strings.Contains(body, "hubtask_panics_recovered_total") {
		t.Fatalf("the panic counter is missing from the scrape:\n%s", body)
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "hubtask_panics_recovered_total{") && !strings.HasSuffix(line, " 0") {
			t.Errorf("the panic counter is not zero: %s", line)
		}
	}
}

// And it moves when a panic is caught - a counter that cannot count is worse than none, because
// an alert on it would stay quiet forever.
func TestThePanicCounterCountsWhatItIsFor(t *testing.T) {
	m := newTestMetrics(t, env.Config{})

	m.PanicRecovered(context.Background(), "server.ops")

	body := scrape(t, m)
	if !strings.Contains(body, `hubtask_panics_recovered_total{component="server.ops"} 1`) {
		t.Errorf("the panic was not counted:\n%s", body)
	}
}

func TestBuildInfoCarriesTheVersion(t *testing.T) {
	m := newTestMetrics(t, env.Config{})

	body := scrape(t, m)

	if !strings.Contains(body, `version="1.2.3"`) || !strings.Contains(body, `commit="abc1234"`) {
		t.Errorf("build info does not carry the build:\n%s", body)
	}
	if !strings.Contains(body, "go_version=") {
		t.Errorf("build info does not carry the Go version:\n%s", body)
	}
}

// Label cardinality is the failure that only shows up in production, months later, as a
// Prometheus that will not start. The status class is a series; the status code would be five.
func TestTheStatusIsReducedToItsClass(t *testing.T) {
	m := newTestMetrics(t, env.Config{})
	ctx := context.Background()

	for _, status := range []int{200, 201, 404, 422, 500, 503} {
		m.HTTPRequest(ctx, "/items/{id}", http.MethodGet, status, 0.01)
	}

	body := scrape(t, m)
	for _, want := range []string{`status_class="2xx"`, `status_class="4xx"`, `status_class="5xx"`} {
		if !strings.Contains(body, want) {
			t.Errorf("%s is missing:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{`status="200"`, `status_class="200"`, `status_class="404"`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the exact status became a label: %s", unwanted)
		}
	}
}

// The tenant label is off unless the operator asks for it (§3.2).
func TestTheTenantLabelIsOffByDefault(t *testing.T) {
	m := newTestMetrics(t, env.Config{})

	m.UseCase(context.Background(), "CreateContainer", "ok", "01936f2a-7c1e-7000-8000-00000000000a")

	body := scrape(t, m)
	if strings.Contains(body, "tenant_id=") {
		t.Errorf("the tenant reached the labels although it is switched off:\n%s", body)
	}
	if !strings.Contains(body, `use_case="CreateContainer"`) {
		t.Errorf("the use case is missing:\n%s", body)
	}
}

func TestTheTenantLabelAppearsWhenTheOperatorAsksForIt(t *testing.T) {
	m := newTestMetrics(t, env.Config{Metrics: env.MetricsConfig{TenantLabel: true}})

	m.UseCase(context.Background(), "CreateContainer", "ok", "01936f2a-7c1e-7000-8000-00000000000a")

	if body := scrape(t, m); !strings.Contains(body, "tenant_id=") {
		t.Errorf("the tenant label was requested but is missing:\n%s", body)
	}
}

// No identifier may appear as a label value - not an item, not a user, not a rule (§3.2). This
// reads the actual scrape rather than the call sites, so a label added later is caught too.
func TestNoIdentifierReachesTheLabels(t *testing.T) {
	m := newTestMetrics(t, env.Config{})
	ctx := context.Background()

	// Everything an unwary caller might pass: a resolved path, an identifier as a use case, a
	// dependency named after a row.
	m.HTTPRequest(ctx, "/items/{id}", http.MethodGet, 200, 0.01)
	m.UseCase(ctx, "CreateContainer", "ok", "01936f2a-7c1e-7000-8000-00000000000a")
	m.DependencyUp(ctx, "postgres", true)
	m.DegradedMode(ctx, "media", false)
	m.ConfigInvalid(ctx, "HUBTASK_DB_MAX_CONNS")
	m.InflightDelta(ctx, "api", 1)

	body := scrape(t, m)

	uuid := regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	if match := uuid.FindString(body); match != "" {
		t.Errorf("an identifier appears in the metrics: %s", match)
	}
	// A route template keeps its placeholder; a resolved path would carry the identifier.
	if !strings.Contains(body, `route="/items/{id}"`) {
		t.Errorf("the route template is missing:\n%s", body)
	}
}

// Every metric carries the project prefix, so a dashboard can select the project's series
// without listing them (§4).
func TestEveryMetricCarriesTheNamespace(t *testing.T) {
	m := newTestMetrics(t, env.Config{})
	ctx := context.Background()
	m.HTTPRequest(ctx, "/items", http.MethodGet, 200, 0.01)
	m.UseCase(ctx, "CreateContainer", "ok", "")

	for _, line := range strings.Split(scrape(t, m), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, _, _ := strings.Cut(line, "{")
		name, _, _ = strings.Cut(name, " ")
		if !strings.HasPrefix(name, namespace+"_") {
			t.Errorf("the metric %q is outside the namespace", name)
		}
	}
}

// The duration buckets have to straddle the targets from engineering-guidelines.md §4 - P95 read
// under 200 ms, write under 300 ms. Default buckets put both in the same one, which makes the
// SLO unmeasurable.
func TestTheDurationBucketsStraddleTheTargets(t *testing.T) {
	m := newTestMetrics(t, env.Config{})

	m.HTTPRequest(context.Background(), "/items", http.MethodGet, 200, 0.15)

	body := scrape(t, m)
	for _, bound := range []string{`le="0.2"`, `le="0.3"`} {
		if !strings.Contains(body, bound) {
			t.Errorf("the bucket %s is missing:\n%s", bound, body)
		}
	}
}
