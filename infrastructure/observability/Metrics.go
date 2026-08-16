// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package observability

import (
	"context"
	"fmt"
	"net/http"
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// namespace prefixes every metric, so that a dashboard can select the project's own series
// without knowing each name (observability-reliability.md §4).
const namespace = "hubtask"

// Metrics owns the meter and the instruments that exist before any use case does. Handing out an
// instrument rather than the meter is deliberate: an instrument created at a call site is created
// per call, and the label set of one created ad hoc is nobody's decision.
type Metrics struct {
	registry *prometheus.Registry
	provider *sdkmetric.MeterProvider

	panicsRecovered   metric.Int64Counter
	httpRequests      metric.Int64Counter
	httpDuration      metric.Float64Histogram
	inflightRequests  metric.Int64UpDownCounter
	useCaseTotal      metric.Int64Counter
	dependencyUp      metric.Int64Gauge
	degradedMode      metric.Int64Gauge
	configInvalid     metric.Int64Counter
	breakerState      metric.Int64Gauge
	outboundDuration  metric.Float64Histogram
	rateLimited       metric.Int64Counter
	tenantLabelActive bool
}

// NewMetrics builds the meter and registers what the process needs from the first second: the
// panic counter above all, whose value is the one an alert watches (ADR-0016).
func NewMetrics(cfg env.Config) (*Metrics, error) {
	registry := prometheus.NewRegistry()

	exporter, err := otelprom.New(
		otelprom.WithRegisterer(registry),
		// The runtime already reports its own version through build_info; the exporter's
		// duplicate of it is noise on every series.
		otelprom.WithoutScopeInfo(),
		otelprom.WithoutTargetInfo(),
	)
	if err != nil {
		return nil, fmt.Errorf("prometheus exporter: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	meter := provider.Meter(namespace)

	m := &Metrics{
		registry:          registry,
		provider:          provider,
		tenantLabelActive: cfg.Metrics.TenantLabel,
	}

	if err := m.instruments(meter); err != nil {
		return nil, err
	}

	// A counter that has never counted has no series, and a series that does not exist cannot
	// be alerted on - `absent()` fires, or the dashboard shows "no data" and everyone assumes
	// the panel is broken. Seeding zero makes the metric readable from the first scrape, which
	// is what "exists and stays at 0" asks for (ADR-0016).
	m.PanicRecoveredBy(context.Background(), "process", 0)
	if err := m.buildInfo(meter, cfg); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Metrics) instruments(meter metric.Meter) error {
	var err error
	if m.panicsRecovered, err = meter.Int64Counter(
		namespace+"_panics_recovered_total",
		metric.WithDescription("Panics caught by SafeGo or a recovery middleware. Target value: 0."),
	); err != nil {
		return fmt.Errorf("panic counter: %w", err)
	}
	if m.httpRequests, err = meter.Int64Counter(
		namespace+"_http_requests_total",
		metric.WithDescription("HTTP requests by route, method and status class."),
	); err != nil {
		return fmt.Errorf("request counter: %w", err)
	}
	if m.httpDuration, err = meter.Float64Histogram(
		namespace+"_http_request_duration_seconds",
		metric.WithDescription("HTTP request duration."),
		metric.WithUnit("s"),
		// Buckets around the targets from engineering-guidelines.md §4: P95 read < 200 ms,
		// write < 300 ms. Default buckets would put both in the same bucket.
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.3, 0.5, 1, 2.5, 5, 10),
	); err != nil {
		return fmt.Errorf("duration histogram: %w", err)
	}
	if m.inflightRequests, err = meter.Int64UpDownCounter(
		namespace+"_inflight_requests",
		metric.WithDescription("Requests currently being served, by role."),
	); err != nil {
		return fmt.Errorf("inflight gauge: %w", err)
	}
	if m.useCaseTotal, err = meter.Int64Counter(
		namespace+"_usecase_total",
		metric.WithDescription("Use case executions by outcome."),
	); err != nil {
		return fmt.Errorf("use case counter: %w", err)
	}
	if m.dependencyUp, err = meter.Int64Gauge(
		namespace+"_dependency_up",
		metric.WithDescription("Self-diagnosis as a time series: 1 up, 0 down."),
	); err != nil {
		return fmt.Errorf("dependency gauge: %w", err)
	}
	if m.degradedMode, err = meter.Int64Gauge(
		namespace+"_degraded_mode",
		metric.WithDescription("1 while a feature is restricted."),
	); err != nil {
		return fmt.Errorf("degradation gauge: %w", err)
	}
	if m.configInvalid, err = meter.Int64Counter(
		namespace+"_config_invalid_total",
		metric.WithDescription("Configuration rejected at startup, by variable."),
	); err != nil {
		return fmt.Errorf("config counter: %w", err)
	}
	if m.breakerState, err = meter.Int64Gauge(
		namespace+"_circuit_breaker_state",
		metric.WithDescription("0 closed, 1 half-open, 2 open, per guarded dependency."),
	); err != nil {
		return fmt.Errorf("breaker gauge: %w", err)
	}
	if m.outboundDuration, err = meter.Float64Histogram(
		namespace+"_outbound_http_duration_seconds",
		metric.WithDescription("Duration of outbound HTTP calls by target class."),
		metric.WithUnit("s"),
		// Wider than the inbound buckets: a third-party system that answers in five seconds is
		// unpleasant but real, and the inbound scale would put everything slow in one bucket.
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30),
	); err != nil {
		return fmt.Errorf("outbound histogram: %w", err)
	}
	if m.rateLimited, err = meter.Int64Counter(
		namespace+"_rate_limited_total",
		metric.WithDescription("Calls turned away by a limit: a rate limit, a bulkhead, or load shedding."),
	); err != nil {
		return fmt.Errorf("rate limit counter: %w", err)
	}
	return nil
}

// buildInfo is the gauge that is always 1 and whose labels are the point: it makes the version
// situation across a cluster visible, which is what a rolling update needs.
func (m *Metrics) buildInfo(meter metric.Meter, cfg env.Config) error {
	gauge, err := meter.Int64Gauge(
		namespace+"_build_info",
		metric.WithDescription("Always 1; the labels carry the build."),
	)
	if err != nil {
		return fmt.Errorf("build info: %w", err)
	}
	gauge.Record(context.Background(), 1, metric.WithAttributes(
		attribute.String("version", cfg.Version),
		attribute.String("commit", cfg.Commit),
		attribute.String("go_version", runtime.Version()),
	))
	return nil
}

// Handler serves the Prometheus endpoint. It belongs on the ops port, never on the public one
// (§3.2): the series say what runs, how much of it, and how slowly.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		// A scrape failure is a monitoring problem, not an application one: report it to the
		// scraper and keep serving.
		ErrorHandling: promhttp.ContinueOnError,
	})
}

// PanicRecovered counts a caught panic. component is the location, not the message: a panic
// message can contain anything, including user content (rule 10).
func (m *Metrics) PanicRecovered(ctx context.Context, component string) {
	m.PanicRecoveredBy(ctx, component, 1)
}

// PanicRecoveredBy exists for the zero seeding above; a delta of anything but 0 or 1 would be a
// misuse, and the counter is not the place to be clever.
func (m *Metrics) PanicRecoveredBy(ctx context.Context, component string, delta int64) {
	m.panicsRecovered.Add(ctx, delta, metric.WithAttributes(attribute.String("component", component)))
}

// HTTPRequest records one served request. route is the template (/items/{id}), never the
// resolved path - the resolved one carries identifiers and would be unbounded.
func (m *Metrics) HTTPRequest(ctx context.Context, route, method string, status int, seconds float64) {
	attrs := metric.WithAttributes(
		attribute.String("route", route),
		attribute.String("method", method),
		attribute.String("status_class", statusClass(status)),
	)
	m.httpRequests.Add(ctx, 1, attrs)
	m.httpDuration.Record(ctx, seconds,
		metric.WithAttributes(attribute.String("route", route), attribute.String("method", method)))
}

// InflightDelta moves the gauge of requests currently in flight.
func (m *Metrics) InflightDelta(ctx context.Context, role string, delta int64) {
	m.inflightRequests.Add(ctx, delta, metric.WithAttributes(attribute.String("role", role)))
}

// UseCase records the outcome of a use case. The result is the error category, not the message:
// ok, or one of the domain's categories in lower case (§4.1) - see ResultClass, which is the only
// thing that should be producing this value.
func (m *Metrics) UseCase(ctx context.Context, useCase, result string, tenant string) {
	attrs := []attribute.KeyValue{
		attribute.String("use_case", useCase),
		attribute.String("result", result),
	}
	// The tenant label is off unless the operator asks for it: in provider operation with
	// thousands of tenants it multiplies every series by the tenant count (§3.2).
	if m.tenantLabelActive && tenant != "" {
		attrs = append(attrs, attribute.String("tenant_id", tenant))
	}
	m.useCaseTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// DependencyUp mirrors the health report into a time series, so that a disruption has a
// beginning and an end in the same place as everything else.
func (m *Metrics) DependencyUp(ctx context.Context, dependency string, up bool) {
	value := int64(0)
	if up {
		value = 1
	}
	m.dependencyUp.Record(ctx, value, metric.WithAttributes(attribute.String("dependency", dependency)))
}

// DegradedMode reports a feature as restricted, which is what makes a partial outage visible
// without reading the health endpoint.
func (m *Metrics) DegradedMode(ctx context.Context, feature string, degraded bool) {
	value := int64(0)
	if degraded {
		value = 1
	}
	m.degradedMode.Record(ctx, value, metric.WithAttributes(attribute.String("feature", feature)))
}

// CircuitBreakerState publishes the state of one breaker: 0 closed, 1 half-open, 2 open. The
// values come from resilience.BreakerState.Level(), which owns them - the gauge only reports
// what it is handed, so that the dashboard's contract has a single source (§4).
//
// The composition root passes this as the breaker's OnStateChange hook. It takes the level
// rather than the state type, so that the metrics adapter does not have to know the resilience
// adapter: adapters do not know each other (project-structure.md §2).
func (m *Metrics) CircuitBreakerState(ctx context.Context, dependency string, level int64) {
	m.breakerState.Record(ctx, level, metric.WithAttributes(attribute.String("dependency", dependency)))
}

// OutboundHTTP records the duration of one outbound call. targetClass is a class, never a host:
// a webhook target is chosen by a tenant, and a label per URL would grow a series per customer
// integration (§3.2, rule 10).
func (m *Metrics) OutboundHTTP(ctx context.Context, targetClass string, seconds float64) {
	m.outboundDuration.Record(ctx, seconds,
		metric.WithAttributes(attribute.String("target_class", targetClass)))
}

// RateLimited counts a call turned away by a limit. scope says which limit: ip, token, tenant
// for the rate limiter, load_shed for the shedder, bulkhead:<compartment> for a full
// compartment. The set is written by hand and stays small - that is what keeps it a label.
func (m *Metrics) RateLimited(ctx context.Context, scope string) {
	m.rateLimited.Add(ctx, 1, metric.WithAttributes(attribute.String("scope", scope)))
}

// ConfigInvalid counts a rejected configuration variable - misconfiguration as a signal rather
// than as guesswork in a support conversation.
func (m *Metrics) ConfigInvalid(ctx context.Context, key string) {
	m.configInvalid.Add(ctx, 1, metric.WithAttributes(attribute.String("key", key)))
}

// Shutdown flushes the meter. Metrics missing from the last seconds of a pod's life are exactly
// the ones an incident review wants.
func (m *Metrics) Shutdown(ctx context.Context) error {
	return m.provider.Shutdown(ctx)
}

// statusClass reduces a status to its class. The exact code would give 5xx a series per code and
// tell an alert nothing more than the class does.
func statusClass(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	case status >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}
