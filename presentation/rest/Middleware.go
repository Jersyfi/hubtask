// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// RequestIDHeader is the header a client may send and always gets back. A load balancer or a
// browser extension that already assigns one keeps its value, so one identifier spans the whole
// hop chain (api-guidelines.md §6).
const RequestIDHeader = "X-Request-Id"

// routeUnmatched is the route label for a request that matched nothing. The raw path must never
// become the label: it carries identifiers, and an unbounded label is how a Prometheus dies
// (§3.2).
const routeUnmatched = "unmatched"

// maxRequestIDLength bounds an adopted request ID. It lands in a log line and in an error
// response, and an unbounded header value from outside belongs in neither.
const maxRequestIDLength = 64

// Router is what the observability middleware needs beyond serving: the route template, known
// before dispatch. *http.ServeMux satisfies it, and so will the generated router from A-06.
type Router interface {
	http.Handler
	// Handler resolves a request to its handler and the pattern that matched, without serving.
	Handler(r *http.Request) (http.Handler, string)
}

// MetricRecorder is the slice of the metrics adapter this middleware uses. An interface rather
// than the adapter, so the presentation layer keeps pointing inwards (project-structure.md §2).
type MetricRecorder interface {
	HTTPRequest(ctx context.Context, route, method string, status int, seconds float64)
	InflightDelta(ctx context.Context, role string, delta int64)
}

// Observed is the RED middleware of §4: a rate, an error class, and a duration for every
// request, plus the span and the request ID everything else correlates against.
//
// It is also the outermost panic guard of the request path. A handler that panics still owes
// the client an answer, and it must be an RFC 9457 problem rather than a dropped connection.
type Observed struct {
	// Router is served, and asked for the route template first.
	Router Router
	// Metrics may be nil in a test that only cares about the response.
	Metrics MetricRecorder
	// Tracer is never nil in practice: with tracing off it is the no-op tracer, which still
	// carries an incoming trace context through (§3.3).
	Tracer trace.Tracer
	// Role labels the inflight gauge. One process can serve several roles (ADR-0014); this is
	// the one whose port is being served.
	Role string
}

func (o Observed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()

	// The route template has to be known before dispatch, because it is a metric label and a
	// span name. Resolving it costs a second lookup; the alternative is the raw path.
	route := routeUnmatched
	if _, pattern := o.Router.Handler(r); pattern != "" {
		route = pattern
	}

	requestID := requestIDFrom(r)
	// The incoming traceparent is adopted here, which is what makes a chain of caller → API →
	// job one trace rather than three (§3.3).
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	ctx = correlation.ContextWithRequestID(ctx, requestID)

	ctx, span := o.Tracer.Start(ctx, r.Method+" "+route,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("http.request.method", r.Method),
			attribute.String("http.route", route),
			attribute.String("hubtask.request_id", requestID),
		))
	defer span.End()

	w.Header().Set(RequestIDHeader, requestID)
	recorder := &statusRecorder{ResponseWriter: w}

	if o.Metrics != nil {
		o.Metrics.InflightDelta(ctx, o.Role, 1)
		defer o.Metrics.InflightDelta(ctx, o.Role, -1)
	}

	served := false
	defer func() {
		if !served {
			// A panic is unwinding. Recovering here rather than through concurrency.Recover is
			// deliberate: recover() only works one frame deep, and this frame still owes the
			// client a response.
			if rec := recover(); rec != nil {
				concurrency.Report(ctx, "rest.request", rec)
				if !recorder.written {
					WriteProblem(recorder, shared.ErrInternal, requestID)
				}
			}
		}
		o.finish(ctx, span, route, r.Method, recorder.statusCode(), time.Since(started))
	}()

	o.Router.ServeHTTP(recorder, r.WithContext(ctx))
	served = true
}

func (o Observed) finish(ctx context.Context, span trace.Span, route, method string, status int, elapsed time.Duration) {
	span.SetAttributes(attribute.Int("http.response.status_code", status))
	if status >= http.StatusInternalServerError {
		// Only 5xx is our error. A 404 or a 422 is the API working as designed, and marking it
		// an error would make the error-sampled traces useless (§3.1 level policy).
		span.SetStatus(codes.Error, "")
	}
	if o.Metrics != nil {
		o.Metrics.HTTPRequest(ctx, route, method, status, elapsed.Seconds())
	}
}

// requestIDFrom adopts the client's request ID or mints one.
//
// The adopted value is validated, not trusted: it is written into a log line and into an error
// response, so an arbitrary header value from outside would be a log injection and a reflected
// payload in one (security.md §9).
func requestIDFrom(r *http.Request) string {
	incoming := r.Header.Get(RequestIDHeader)
	if isSafeRequestID(incoming) {
		return incoming
	}
	return newRequestID()
}

func isSafeRequestID(value string) bool {
	if value == "" || len(value) > maxRequestIDLength {
		return false
	}
	for _, c := range value {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

// newRequestID is 16 random bytes as hex. Not a UUID: this identifier never reaches the
// database, so it needs no ordering and no port - it needs to be unique in a log index and
// unguessable enough that it cannot be used to probe for another user's request.
func newRequestID() string {
	var buf [16]byte
	// crypto/rand.Read cannot fail on any supported platform; since Go 1.24 it panics rather
	// than returning an error, so there is nothing left to handle here.
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

// statusRecorder remembers the status for the metric. net/http does not expose it, and the
// alternative is every handler reporting its own - which is the same forgetting problem the
// middleware exists to solve.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

// statusCode is what net/http will have sent. A handler that writes nothing at all still
// produces a 200, and the metric has to agree with the wire rather than report a status of zero.
func (s *statusRecorder) statusCode() int {
	if !s.written {
		return http.StatusOK
	}
	return s.status
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.written {
		return
	}
	s.status, s.written = code, true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.written {
		// net/http implies 200 on the first write; the metric has to see the same thing.
		s.WriteHeader(http.StatusOK)
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the real writer, so flushing and deadline control
// keep working through the wrapper.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
