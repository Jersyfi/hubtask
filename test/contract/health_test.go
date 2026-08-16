// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/health"
	healthadapter "github.com/Jersyfi/hubtask/infrastructure/health"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// staticProbe reports a fixed state, so the report can be driven into each of its shapes without
// a real dependency to break.
type staticProbe struct {
	name      string
	required  bool
	status    port.Status
	impact    []string
	errorCode string
	circuit   string
}

func (p staticProbe) Name() string   { return p.name }
func (p staticProbe) Required() bool { return p.required }
func (p staticProbe) Check(context.Context) port.Result {
	return port.Result{
		Status:       p.status,
		Latency:      3 * time.Millisecond,
		ErrorCode:    p.errorCode,
		Since:        time.Unix(1_755_000_000, 0).UTC(),
		CircuitState: p.circuit,
		Impact:       p.impact,
	}
}

func fetchHealth(t *testing.T, registry *healthadapter.Registry) (int, []byte) {
	t.Helper()
	handler := rest.OpsController{Health: registry}.Routes()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(
		context.Background(), http.MethodGet, "/meta/health", nil))
	return rec.Code, rec.Body.Bytes()
}

func mustValidate(t *testing.T, body []byte) {
	t.Helper()
	spec, err := loadSpec()
	if err != nil {
		t.Fatalf("the specification could not be read: %v", err)
	}
	problems, err := spec.validateAgainst("HealthReport", body)
	if err != nil {
		t.Fatalf("validating against HealthReport: %v", err)
	}
	for _, p := range problems {
		t.Errorf("the response does not match HealthReport: %s", p)
	}
	if len(problems) > 0 {
		t.Logf("the response was:\n%s", body)
	}
}

// The acceptance criterion of A-04: /meta/health returns the schema from openapi.yaml. Checked
// against the specification itself, so the two cannot drift - the specification is the source,
// not the result (ADR-0004).
func TestTheHealthyReportMatchesTheSchema(t *testing.T) {
	registry := healthadapter.NewRegistry("0.1.0", []string{"api"})
	registry.Register(staticProbe{name: "postgres", required: true, status: port.StatusOK})
	registry.MarkStarted()

	status, body := fetchHealth(t, registry)

	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	mustValidate(t, body)
}

// The degraded shape is the one with the optional fields in it - since, last_error_code,
// circuit_state, impact and degraded_features only appear when something is wrong, so the
// healthy response alone would prove nothing about them.
func TestTheDegradedReportMatchesTheSchema(t *testing.T) {
	registry := healthadapter.NewRegistry("0.1.0", []string{"api", "worker"})
	registry.Register(staticProbe{name: "postgres", required: true, status: port.StatusOK})
	registry.Register(staticProbe{
		name:      "object_storage",
		status:    port.StatusDown,
		impact:    []string{"media.upload", "media.download"},
		errorCode: "storage.unreachable",
		circuit:   "open",
	})
	registry.Register(staticProbe{name: "ai_provider", status: port.StatusDisabled})
	registry.SetWarnings([]port.Warning{
		{Code: "config.backup_not_configured", Severity: "warn"},
		{Code: "config.smtp_without_tls", Severity: "warn", Params: map[string]string{"host": "mail.example.org"}},
	})
	registry.MarkStarted()

	status, body := fetchHealth(t, registry)

	// A degraded system is still a reachable endpoint: the HTTP status says whether the report
	// could be produced, the status field says how the system is (api-guidelines.md, the
	// getHealthReport description).
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200 for a degraded system", status)
	}
	mustValidate(t, body)
}

// A mandatory dependency down answers 503, so a status page needs to read no JSON to know
// (ADR-0016) - and the body still has to be a valid report.
func TestTheDownReportAnswers503AndStillMatchesTheSchema(t *testing.T) {
	registry := healthadapter.NewRegistry("0.1.0", []string{"api"})
	registry.Register(staticProbe{
		name: "postgres", required: true, status: port.StatusDown,
		errorCode: "postgres.unreachable", impact: []string{"all"},
	})
	registry.MarkStarted()

	status, body := fetchHealth(t, registry)

	if status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", status)
	}
	mustValidate(t, body)
}

// The other half of the acceptance criterion: config.backup_not_configured has to arrive as a
// warning code, because it is the signal a self-hosted installation with no Prometheus gets
// (§5).
func TestTheBackupWarningReachesTheResponse(t *testing.T) {
	registry := healthadapter.NewRegistry("0.1.0", []string{"api"})
	registry.SetWarnings([]port.Warning{{Code: "config.backup_not_configured", Severity: "warn"}})
	registry.MarkStarted()

	_, body := fetchHealth(t, registry)
	mustValidate(t, body)

	report := decode(t, body)
	warnings, _ := report["warnings"].([]any)
	if len(warnings) != 1 {
		t.Fatalf("expected one warning, got %v", report["warnings"])
	}
	warning, _ := warnings[0].(map[string]any)
	if warning["code"] != "config.backup_not_configured" {
		t.Errorf("the warning code is %v", warning["code"])
	}
	if warning["severity"] != "warn" {
		t.Errorf("the severity is %v", warning["severity"])
	}
}

// The endpoint is read by a status page and by support. Nothing about it may be cached, and it
// must not carry a driver message - only codes (security.md §9).
func TestTheReportIsNotCacheable(t *testing.T) {
	registry := healthadapter.NewRegistry("0.1.0", []string{"api"})
	registry.Register(staticProbe{name: "postgres", required: true, status: port.StatusOK})
	registry.MarkStarted()

	handler := rest.OpsController{Health: registry}.Routes()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(
		context.Background(), http.MethodGet, "/meta/health", nil))

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
}
