// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"

	env "github.com/Jersyfi/hubtask/core/port/environment"
)

func logAndParse(t *testing.T, cfg env.Config, log func(*slog.Logger)) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	log(NewLogger(cfg, &buf))

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("the log line is not JSON (%v): %s", err, buf.String())
	}
	return entry
}

// The mandatory fields of §3.1 that connect a log line to a trace. Without them the two are
// unrelated pieces of evidence, and an incident is investigated twice.
func TestTheLogCarriesTheTraceAndTheRequest(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("the fixture is not a trace ID: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("the fixture is not a span ID: %v", err)
	}

	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled,
	}))
	ctx = ContextWithRequestID(ctx, "01936f2a-7c1e-7000-8000-00000000000a")
	ctx = ContextWithTenant(ctx, "01936f2a-7c1e-7000-8000-00000000000b")

	entry := logAndParse(t, env.Config{}, func(l *slog.Logger) {
		l.InfoContext(ctx, "something happened")
	})

	for field, want := range map[string]string{
		"trace_id":   "4bf92f3577b34da6a3ce929d0e0e4736",
		"span_id":    "00f067aa0ba902b7",
		"request_id": "01936f2a-7c1e-7000-8000-00000000000a",
		"tenant_id":  "01936f2a-7c1e-7000-8000-00000000000b",
	} {
		if got, _ := entry[field].(string); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
}

// Outside a request there is nothing to correlate, and an empty trace_id in every startup line
// is noise that makes the field useless as a filter.
func TestTheCorrelationFieldsAreAbsentOutsideARequest(t *testing.T) {
	entry := logAndParse(t, env.Config{}, func(l *slog.Logger) {
		l.Info("starting")
	})

	for _, field := range []string{"trace_id", "span_id", "request_id", "tenant_id"} {
		if _, ok := entry[field]; ok {
			t.Errorf("%s appears although there is no request", field)
		}
	}
}

// service, role and version are the constant part of the mandatory field set (§3.1). role is
// what tells two pods of the same image apart in a log query (ADR-0014).
func TestTheLogCarriesTheServiceIdentity(t *testing.T) {
	cfg := env.Config{Version: "1.2.3", Roles: []env.Role{env.RoleAPI, env.RoleWorker}}

	entry := logAndParse(t, cfg, func(l *slog.Logger) { l.Info("starting") })

	for field, want := range map[string]string{
		"service": "hubtask",
		"version": "1.2.3",
		"role":    "api,worker",
	} {
		if got, _ := entry[field].(string); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
}

// Correlation sits outside the redaction, so what it adds is redacted like anything else. The
// check that matters is the reverse of the usual one: the redaction must still bite when the
// correlating handler is in front of it.
func TestRedactionStillAppliesUnderneathTheCorrelation(t *testing.T) {
	ctx := ContextWithRequestID(context.Background(), "req-1")

	entry := logAndParse(t, env.Config{}, func(l *slog.Logger) {
		l.InfoContext(ctx, "connection failed",
			slog.String("dsn", "postgres://hubtask:hunter2@db:5432/hubtask"))
	})

	dsn, _ := entry["dsn"].(string)
	if dsn != Redacted {
		t.Errorf("dsn = %q, want the key itself to be redacted", dsn)
	}
	if got, _ := entry["request_id"].(string); got != "req-1" {
		t.Errorf("the request ID did not survive the redaction: %q", got)
	}
}
