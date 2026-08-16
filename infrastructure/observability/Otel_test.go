// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// The incoming traceparent has to be adopted even with tracing switched off - otherwise the log
// of a self-hosted installation cannot be lined up with the caller's trace, and switching
// tracing on becomes the only way to correlate anything (§3.3).
func TestTheIncomingTraceparentIsAdoptedWithTracingOff(t *testing.T) {
	tracing, err := NewTracing(context.Background(), env.Config{})
	if err != nil {
		t.Fatalf("building the tracing: %v", err)
	}
	t.Cleanup(func() { _ = tracing.Shutdown(context.Background()) })
	if tracing.Enabled {
		t.Fatal("tracing reports itself enabled although it is off by default")
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/items", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")

	ctx := otel.GetTextMapPropagator().Extract(req.Context(), propagation.HeaderCarrier(req.Header))

	sc := trace.SpanContextFromContext(ctx)
	if got := sc.TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("the trace ID was not adopted: %q", got)
	}
	// And the no-op tracer passes it through, which is what makes the log correlation work.
	_, span := tracing.Tracer("test").Start(ctx, "unit")
	defer span.End()
	if got := span.SpanContext().TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("the no-op tracer dropped the trace ID: %q", got)
	}
}

// The outgoing direction of the same rule: what we adopted has to be injectable again, or the
// chain ends at our boundary.
func TestTheTraceparentIsPassedOnToOutboundCalls(t *testing.T) {
	tracing, err := NewTracing(context.Background(), env.Config{})
	if err != nil {
		t.Fatalf("building the tracing: %v", err)
	}
	t.Cleanup(func() { _ = tracing.Shutdown(context.Background()) })

	incoming := http.Header{}
	incoming.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	ctx := otel.GetTextMapPropagator().Extract(context.Background(), propagation.HeaderCarrier(incoming))

	outgoing := http.Header{}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(outgoing))

	if got := outgoing.Get("traceparent"); got == "" {
		t.Fatal("no traceparent was written to the outbound headers")
	} else if got[3:35] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("the outbound traceparent carries a different trace: %q", got)
	}
}

func TestAnEndpointThatIsNotAURLIsRejected(t *testing.T) {
	_, err := NewTracing(context.Background(), env.Config{
		Tracing: env.TracingConfig{Enabled: true, Endpoint: "collector:4318"},
	})
	if err == nil {
		t.Fatal("an endpoint without a scheme was accepted")
	}
}

// The sampling policy of §3.3, tested through the processor rather than through a collector:
// errors and slow spans are kept whatever the ratio says.
func TestTheSamplingPolicyKeepsErrorsAndSlowSpans(t *testing.T) {
	cases := []struct {
		name     string
		ratio    float64
		status   codes.Code
		duration time.Duration
		want     bool
	}{
		{name: "an error at a ratio of zero", ratio: 0, status: codes.Error, duration: time.Millisecond, want: true},
		{name: "a slow span at a ratio of zero", ratio: 0, status: codes.Ok, duration: 2 * time.Second, want: true},
		{name: "an ordinary span at a ratio of zero", ratio: 0, status: codes.Ok, duration: time.Millisecond, want: false},
		{name: "an ordinary span at a ratio of one", ratio: 1, status: codes.Ok, duration: time.Millisecond, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(
				sdktrace.WithSampler(sdktrace.AlwaysSample()),
				sdktrace.WithSpanProcessor(retainingProcessor{
					next:  recorder,
					ratio: tc.ratio,
					slow:  slowSpanThreshold,
				}),
			)
			t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

			_, span := provider.Tracer("test").Start(context.Background(), "unit",
				trace.WithTimestamp(time.Now().Add(-tc.duration)))
			span.SetStatus(tc.status, "")
			span.End()

			if got := len(recorder.Ended()) == 1; got != tc.want {
				t.Errorf("retained = %v, want %v", got, tc.want)
			}
		})
	}
}

// A ratio decision drawn per span would tear a trace apart: the parent kept, the child dropped.
// Drawing it from the trace ID keeps a trace whole.
func TestTheRatioDecisionIsTheSameForEverySpanOfATrace(t *testing.T) {
	id, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("the fixture is not a trace ID: %v", err)
	}

	first := withinRatio(id, 0.5)
	for range 100 {
		if withinRatio(id, 0.5) != first {
			t.Fatal("the same trace ID produced two different decisions")
		}
	}
	if withinRatio(id, 0) {
		t.Error("a ratio of zero kept a span")
	}
	if !withinRatio(id, 1) {
		t.Error("a ratio of one dropped a span")
	}
}
