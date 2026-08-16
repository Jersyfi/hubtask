// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package observability

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// NewCorrelatingHandler adds trace_id, span_id, request_id and tenant_id from the context to
// every record.
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
	if id := correlation.RequestIDFrom(ctx); id != "" {
		record.AddAttrs(slog.String("request_id", id))
	}
	if id := correlation.TenantFrom(ctx); id != "" {
		record.AddAttrs(slog.String("tenant_id", id))
	}
	return h.inner.Handle(ctx, record)
}

func (h correlatingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return correlatingHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h correlatingHandler) WithGroup(name string) slog.Handler {
	return correlatingHandler{inner: h.inner.WithGroup(name)}
}
