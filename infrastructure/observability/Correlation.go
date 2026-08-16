// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package observability

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// requestIDKey is the context key for the request ID. Unexported and of its own type, so no
// other package can collide with it - and so the only way in is ContextWithRequestID.
type contextKey int

const requestIDKey contextKey = iota

// ContextWithRequestID carries the request ID through the call chain. It is what a user quotes
// in a support request: the one handle that connects a response to a log entry
// (api-guidelines.md §6).
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFromContext returns the request ID, or the empty string outside a request.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// NewCorrelatingHandler adds trace_id, span_id and request_id from the context to every record.
//
// It is a handler rather than a rule every call site has to remember, because the call sites are
// many and the one that forgets is the one being debugged. The fields are mandatory in
// observability-reliability.md §3.1 for exactly that reason: without them a log line and a trace
// are two unrelated pieces of evidence.
//
// It belongs outside the redacting handler, so the attributes it adds pass the redaction like
// any other - a handler that writes straight to the sink would be a hole in T-18.
func NewCorrelatingHandler(inner slog.Handler) slog.Handler {
	return correlatingHandler{inner: inner}
}

type correlatingHandler struct {
	inner slog.Handler
}

func (h correlatingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h correlatingHandler) Handle(ctx context.Context, record slog.Record) error {
	// The span context is valid even when tracing is switched off: the propagator adopts an
	// incoming traceparent regardless, and the no-op tracer passes it through (§3.3).
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	if id := RequestIDFromContext(ctx); id != "" {
		record.AddAttrs(slog.String("request_id", id))
	}
	return h.inner.Handle(ctx, record)
}

func (h correlatingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return correlatingHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h correlatingHandler) WithGroup(name string) slog.Handler {
	return correlatingHandler{inner: h.inner.WithGroup(name)}
}
