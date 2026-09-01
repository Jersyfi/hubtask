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
	m.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil))
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

// allowedLabels is the label set of the metric catalogue (observability-reliability.md §4).
// Explicit rather than derived: adding a label is a cardinality decision, and this list is where
// it gets made rather than noticed six months later in a Prometheus that will not start.
var allowedLabels = map[string]bool{
	// hubtask_http_requests_total, hubtask_http_request_duration_seconds
	"route": true, "method": true, "status_class": true, "le": true,
	// hubtask_usecase_total
	"use_case": true, "result": true,
	// hubtask_inflight_requests
	"role": true,
	// hubtask_dependency_up, hubtask_degraded_mode
	"dependency": true, "feature": true,
	// hubtask_panics_recovered_total, hubtask_config_invalid_total
	"component": true, "key": true,
	// hubtask_outbound_http_duration_seconds, hubtask_rate_limited_total
	"target_class": true, "scope": true,
	// hubtask_job_duration_seconds, hubtask_job_failures_total, hubtask_job_dead_letter_total,
	// hubtask_job_queue_depth
	"job_type": true, "attempt_class": true,
	// hubtask_build_info
	"version": true, "commit": true, "go_version": true,
	// hubtask_db_pool_connections
	"pool": true, "state": true,
	// hubtask_rule_runs_total, hubtask_rule_disabled_total
	"trigger_type": true, "reason": true,
	// Only with HUBTASK_METRICS_TENANT_LABEL, and covered by its own test.
	"tenant_id": true,
	// The exporter's own marker on a counter series.
	"otel_scope_name": true, "otel_scope_version": true, "otel_scope_schema_url": true,
}

// exerciseEveryInstrument drives every instrument the adapter owns, so the scrape below sees the
// whole label surface rather than the part a test happened to touch.
func exerciseEveryInstrument(m *Metrics) {
	ctx := context.Background()
	m.HTTPRequest(ctx, "/items/{id}", http.MethodGet, 200, 0.01)
	m.HTTPRequest(ctx, "/items/{id}", http.MethodDelete, 500, 1.5)
	m.InflightDelta(ctx, "api", 1)
	m.InflightDelta(ctx, "api", -1)
	m.UseCase(ctx, "CreateContainer", "ok", "01936f2a-7c1e-7000-8000-00000000000a")
	m.UseCase(ctx, "MoveItem", "conflict", "")
	m.DependencyUp(ctx, "postgres", true)
	m.DependencyUp(ctx, "object_storage", false)
	m.DegradedMode(ctx, "media", true)
	m.PanicRecovered(ctx, "rest.request")
	m.ConfigInvalid(ctx, "HUBTASK_DB_MAX_CONNS")
	m.CircuitBreakerState(ctx, "object_storage", 2)
	m.OutboundHTTP(ctx, "webhook", 0.42)
	m.RateLimited(ctx, "load_shed")
	m.JobFinished(ctx, "outbox.dispatch", 0.03)
	m.JobFailed(ctx, "outbox.dispatch", "retry")
	m.JobDeadLettered(ctx, "outbox.dispatch")
	m.QueueDepth(ctx, "outbox.dispatch", 12)
	m.OutboxLag(ctx, 1.5)
	m.SchedulerTickLag(ctx, 2)
	m.PoolConnections(ctx, "api", 4, 6, 10)
	m.MigrationVersion(ctx, 68)
	m.RuleRun(ctx, "succeeded", "event")
	m.RuleDisabled(ctx, "consecutive_failures")
}

// The gate for label cardinality: no label may appear that the catalogue does not list. A test
// on values alone would pass a brand new label whose values happen to look harmless today.
func TestNoLabelAppearsThatTheCatalogueDoesNotList(t *testing.T) {
	m := newTestMetrics(t, env.Config{Metrics: env.MetricsConfig{TenantLabel: true}})
	exerciseEveryInstrument(m)

	found := labelsIn(scrape(t, m))
	if len(found) < len(allowedLabels)/2 {
		// A test that reads no labels passes for the wrong reason. The scrape has to have been
		// parsed, or the check above is vacuous.
		t.Fatalf("only %d labels were read from the scrape - the parser no longer matches", len(found))
	}
	for _, label := range found {
		if !allowedLabels[label] {
			t.Errorf("the label %q is not in the catalogue (observability-reliability.md §4)", label)
		}
	}
}

// The same scrape from the other side: every label value has to be a bounded token. An unbounded
// value is the failure that only shows up in production, months later, as a Prometheus that will
// not start.
func TestEveryLabelValueIsABoundedToken(t *testing.T) {
	m := newTestMetrics(t, env.Config{})
	exerciseEveryInstrument(m)

	// Anything that looks like an identifier, a path with a resolved segment, or free text.
	unbounded := regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}|[0-9a-f]{32}|\s{2,}`)

	for _, line := range strings.Split(scrape(t, m), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		labels, _, found := strings.Cut(line, "}")
		if !found {
			continue
		}
		if _, values, ok := strings.Cut(labels, "{"); ok {
			if match := unbounded.FindString(values); match != "" {
				t.Errorf("an unbounded label value %q in: %s", match, line)
			}
			for _, pair := range strings.Split(values, `",`) {
				if len(pair) > 120 {
					t.Errorf("a label value far too long to be a token: %s", pair)
				}
			}
		}
	}
}

// labelsIn collects the label names of a scrape.
func labelsIn(body string) []string {
	var names []string
	seen := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		inner, _, found := strings.Cut(line, "}")
		if !found {
			continue
		}
		_, inner, found = strings.Cut(inner, "{")
		if !found {
			continue
		}
		for _, pair := range strings.Split(inner, ",") {
			name, _, ok := strings.Cut(pair, "=")
			name = strings.TrimSpace(name)
			if ok && name != "" && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

// The breaker gauge is what a dashboard reads to show a dependency as cut off, and its values
// are a contract: 0 closed, 1 half-open, 2 open (§4).
func TestTheBreakerGaugeReportsTheState(t *testing.T) {
	m := newTestMetrics(t, env.Config{})
	ctx := context.Background()

	m.CircuitBreakerState(ctx, "object_storage", 2)
	m.CircuitBreakerState(ctx, "smtp", 0)

	body := scrape(t, m)
	if !strings.Contains(body, `hubtask_circuit_breaker_state{dependency="object_storage"`) {
		t.Errorf("the breaker state of the object storage is missing:\n%s", body)
	}
	for _, want := range []string{`dependency="object_storage"} 2`, `dependency="smtp"} 0`} {
		if !strings.Contains(body, want) {
			t.Errorf("%s is missing from the scrape:\n%s", want, body)
		}
	}
}

// A webhook target is chosen by a tenant. A label per URL would grow a series per customer
// integration, so the histogram takes a class and nothing else (rule 10).
func TestTheOutboundHistogramLabelsTheClassNotTheTarget(t *testing.T) {
	m := newTestMetrics(t, env.Config{})

	m.OutboundHTTP(context.Background(), "webhook", 0.42)

	body := scrape(t, m)
	if !strings.Contains(body, `hubtask_outbound_http_duration_seconds_bucket{target_class="webhook"`) {
		t.Errorf("the outbound histogram is missing:\n%s", body)
	}
	// A third-party system that answers in five seconds is unpleasant but real; the inbound
	// scale would put everything slow into one bucket.
	for _, bound := range []string{`le="5"`, `le="30"`} {
		if !strings.Contains(body, bound) {
			t.Errorf("the bucket %s is missing:\n%s", bound, body)
		}
	}
}

func TestRateLimitedCountsByScope(t *testing.T) {
	m := newTestMetrics(t, env.Config{})
	ctx := context.Background()

	m.RateLimited(ctx, "load_shed")
	m.RateLimited(ctx, "load_shed")
	m.RateLimited(ctx, "bulkhead:automation")

	body := scrape(t, m)
	if !strings.Contains(body, `hubtask_rate_limited_total{scope="load_shed"} 2`) {
		t.Errorf("the shed calls were not counted:\n%s", body)
	}
	if !strings.Contains(body, `scope="bulkhead:automation"`) {
		t.Errorf("the full compartment was not counted:\n%s", body)
	}
}

// The signals of A-08. The names are the ones the alert catalogue and the dashboards use, so they
// are asserted as strings rather than derived - a renamed metric is a silent dashboard, and a
// silent dashboard is discovered during the incident it was meant to explain.
func TestTheQueueSignalsAreScrapableUnderTheirCatalogueNames(t *testing.T) {
	m := newTestMetrics(t, env.Config{})
	ctx := context.Background()

	m.JobFinished(ctx, "outbox.dispatch", 0.25)
	m.JobFailed(ctx, "outbox.dispatch", "retry")
	m.JobFailed(ctx, "reminder.fire", "final")
	m.JobDeadLettered(ctx, "reminder.fire")
	m.QueueDepth(ctx, "outbox.dispatch", 12)
	m.OutboxLag(ctx, 1.5)
	m.SchedulerTickLag(ctx, 4)

	body := scrape(t, m)
	for _, want := range []string{
		`hubtask_job_failures_total{attempt_class="retry",job_type="outbox.dispatch"} 1`,
		`hubtask_job_failures_total{attempt_class="final",job_type="reminder.fire"} 1`,
		`hubtask_job_dead_letter_total{job_type="reminder.fire"} 1`,
		`hubtask_job_queue_depth{job_type="outbox.dispatch"} 12`,
		`hubtask_scheduler_tick_lag_seconds 4`,
		`hubtask_job_duration_seconds_sum{job_type="outbox.dispatch"} 0.25`,
		`hubtask_outbox_lag_seconds_sum 1.5`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the scrape does not contain %s", want)
		}
	}
}

// The saturation and rollout gauges H-12 added, under their catalogue names (§4): the pool with
// its three states including the max A-11 divides by, and the embedded migration version whose
// spread across pods is what A-13 reads.
func TestThePoolAndMigrationGaugesAreScrapable(t *testing.T) {
	m := newTestMetrics(t, env.Config{})
	ctx := context.Background()

	m.PoolConnections(ctx, "api", 8, 2, 10)
	m.PoolConnections(ctx, "background", 1, 4, 20)
	m.MigrationVersion(ctx, 68)

	body := scrape(t, m)
	for _, want := range []string{
		`hubtask_db_pool_connections{pool="api",state="in_use"} 8`,
		`hubtask_db_pool_connections{pool="api",state="idle"} 2`,
		`hubtask_db_pool_connections{pool="api",state="max"} 10`,
		`hubtask_db_pool_connections{pool="background",state="max"} 20`,
		`hubtask_migration_version 68`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the scrape does not contain %s", want)
		}
	}
}

// SLO-4 is a P99 under thirty seconds, so the histogram needs a boundary there. Without one the
// percentile is interpolated across whatever bucket happens to span it, and the answer to "are we
// meeting the objective" is a guess.
func TestTheOutboxLagBucketsStraddleTheObjective(t *testing.T) {
	m := newTestMetrics(t, env.Config{})
	m.OutboxLag(context.Background(), 1)

	if !strings.Contains(scrape(t, m), `hubtask_outbox_lag_seconds_bucket{le="30"}`) {
		t.Error("the outbox lag histogram has no boundary at the SLO-4 target of 30 seconds")
	}
}
