// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package observability

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/url"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// exportTimeout bounds a single push to the collector. A tracing backend that stops answering
// must cost the process nothing beyond this (rule 7, ADR-0016 §6 "timeouts everywhere").
const exportTimeout = 10 * time.Second

// slowSpanThreshold is the "slow request" of observability-reliability.md §3.3. A span at or
// beyond it is kept regardless of the sample ratio, because the slow ones are the whole reason
// anyone opens a trace.
const slowSpanThreshold = time.Second

// Tracing owns the tracer provider. It exists whether or not tracing is enabled: the W3C
// propagator is installed either way, so an incoming traceparent still reaches the log and still
// travels onwards to the next service (§3.3). Turning tracing off means "export nothing", not
// "lose the correlation".
type Tracing struct {
	provider trace.TracerProvider
	shutdown func(context.Context) error
	Enabled  bool
}

// NewTracing builds the tracer provider from the configuration and installs it globally, along
// with the W3C trace context and baggage propagators.
//
// Disabled is the default and the documented self-hosting profile (§13): a no-op provider, no
// exporter, no goroutine, and no cost per span - but the propagator is still there.
func NewTracing(ctx context.Context, cfg env.Config) (*Tracing, error) {
	// Composite rather than trace context alone: baggage is how a tenant or a job correlation
	// travels with the trace without every layer threading it through by hand.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if !cfg.Tracing.Enabled {
		provider := noop.NewTracerProvider()
		otel.SetTracerProvider(provider)
		return &Tracing{
			provider: provider,
			shutdown: func(context.Context) error { return nil },
		}, nil
	}

	endpoint, err := url.Parse(cfg.Tracing.Endpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		// The configuration validator only checks that the endpoint is present. A value that is
		// not an OTLP/HTTP URL would otherwise be discovered as a silently dropped span.
		return nil, fmt.Errorf("HUBTASK_TRACING_ENDPOINT is not an http(s) URL: %q", cfg.Tracing.Endpoint)
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(cfg.Tracing.Endpoint),
		otlptracehttp.WithTimeout(exportTimeout),
	)
	if err != nil {
		return nil, fmt.Errorf("trace exporter: %w", err)
	}

	res, err := traceResource(cfg)
	if err != nil {
		return nil, err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		// Every span is recorded; what leaves the process is decided when the span ends. See
		// retainingProcessor for why the decision cannot be taken at the start.
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(retainingProcessor{
			next:  sdktrace.NewBatchSpanProcessor(exporter),
			ratio: cfg.Tracing.SampleRatio,
			slow:  slowSpanThreshold,
		}),
	)
	otel.SetTracerProvider(provider)

	return &Tracing{provider: provider, shutdown: provider.Shutdown, Enabled: true}, nil
}

// traceResource describes the process behind every span. Deliberately no host name and no
// process owner: a resource attribute travels to a third-party backend, and the deployment
// topology is not something the trace needs to carry (ADR-0018).
func traceResource(cfg env.Config) (*resource.Resource, error) {
	roles := make([]string, 0, len(cfg.Roles))
	for _, r := range cfg.Roles {
		roles = append(roles, string(r))
	}

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(namespace),
		semconv.ServiceVersion(cfg.Version),
		attribute.StringSlice("hubtask.role", roles),
	))
	if err != nil {
		return nil, fmt.Errorf("trace resource: %w", err)
	}
	return res, nil
}

// Tracer hands out a named tracer. Named per component, so a backend can tell a span raised by
// the REST layer from one raised by a job.
func (t *Tracing) Tracer(name string) trace.Tracer {
	return t.provider.Tracer(namespace + "/" + name)
}

// Shutdown flushes what has not been exported yet. Without it the spans of the last seconds of
// a pod's life are lost - which are the ones an incident review wants.
func (t *Tracing) Shutdown(ctx context.Context) error { return t.shutdown(ctx) }

// retainingProcessor implements the sampling policy of §3.3: 100% of errors, 100% of spans
// slower than a second, and a configurable share of the rest.
//
// The policy cannot be a Sampler, because a Sampler decides when a span *starts* - and at that
// moment neither the outcome nor the duration is known. So every span is recorded and the
// decision moves to OnEnd. The price is that a recorded span costs memory even when it is
// dropped; the alternative is a 5% chance of seeing the trace of an error, which is no
// diagnosis at all.
//
// The share of ordinary spans is drawn from the trace ID rather than per span, so all spans of
// one trace share the decision and a kept trace is a whole trace. A kept error span whose parent
// was dropped is still a partial trace - that is the known limit of a per-process policy, and
// the reason a collector may tail-sample on top.
type retainingProcessor struct {
	next  sdktrace.SpanProcessor
	ratio float64
	slow  time.Duration
}

func (p retainingProcessor) OnStart(parent context.Context, s sdktrace.ReadWriteSpan) {
	p.next.OnStart(parent, s)
}

func (p retainingProcessor) OnEnd(s sdktrace.ReadOnlySpan) {
	if p.retain(s) {
		p.next.OnEnd(s)
	}
}

func (p retainingProcessor) Shutdown(ctx context.Context) error   { return p.next.Shutdown(ctx) }
func (p retainingProcessor) ForceFlush(ctx context.Context) error { return p.next.ForceFlush(ctx) }

func (p retainingProcessor) retain(s sdktrace.ReadOnlySpan) bool {
	if s.Status().Code == codes.Error {
		return true
	}
	if s.EndTime().Sub(s.StartTime()) >= p.slow {
		return true
	}
	// An upstream service that decided to sample this trace gets its decision honoured -
	// otherwise the chain it is following breaks at our boundary.
	if parent := s.Parent(); parent.IsRemote() && parent.IsSampled() {
		return true
	}
	return withinRatio(s.SpanContext().TraceID(), p.ratio)
}

// withinRatio is the threshold test of the OpenTelemetry ratio sampler, applied to the trace ID:
// deterministic, so the same trace is judged the same way in every process that sees it.
func withinRatio(id trace.TraceID, ratio float64) bool {
	if ratio <= 0 {
		return false
	}
	if ratio >= 1 {
		return true
	}
	// The lower 8 bytes are the random part of a W3C trace ID; the top bit is dropped so the
	// comparison stays inside the positive range.
	return binary.BigEndian.Uint64(id[8:16])>>1 < uint64(ratio*(1<<63))
}
