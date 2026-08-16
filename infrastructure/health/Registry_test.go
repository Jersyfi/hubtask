// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package health

import (
	"context"
	"testing"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/health"
)

// probe is a test double whose status a test can move, so a recovery can be exercised and not
// just an outage.
type probe struct {
	name     string
	required bool
	status   port.Status
	impact   []string
}

func (p *probe) Name() string   { return p.name }
func (p *probe) Required() bool { return p.required }
func (p *probe) Check(context.Context) port.Result {
	return port.Result{Status: p.status, Impact: p.impact, Since: time.Unix(0, 0).UTC()}
}

type recordedSignals struct {
	up       map[string]bool
	degraded map[string]bool
}

func (r *recordedSignals) DependencyUp(_ context.Context, dependency string, up bool) {
	r.up[dependency] = up
}

func (r *recordedSignals) DegradedMode(_ context.Context, feature string, degraded bool) {
	r.degraded[feature] = degraded
}

func newSignals() *recordedSignals {
	return &recordedSignals{up: map[string]bool{}, degraded: map[string]bool{}}
}

func startedRegistry(probes ...port.Probe) *Registry {
	r := NewRegistry("1.2.3", []string{"api"})
	for _, p := range probes {
		r.Register(p)
	}
	r.MarkStarted()
	return r
}

func TestAHealthyProcessReportsOK(t *testing.T) {
	registry := startedRegistry(&probe{name: "postgres", required: true, status: port.StatusOK})

	report := registry.Report(context.Background())

	if report.Status != port.StatusOK {
		t.Errorf("status = %q, want ok", report.Status)
	}
	if len(report.DegradedFeatures) != 0 {
		t.Errorf("a healthy process reports degraded features: %v", report.DegradedFeatures)
	}
}

// The failure of an optional dependency reduces functionality; it does not take the process out
// (ADR-0016, §7).
func TestAnOptionalDependencyDownIsDegradedNotDown(t *testing.T) {
	registry := startedRegistry(
		&probe{name: "postgres", required: true, status: port.StatusOK},
		&probe{name: "object_storage", status: port.StatusDown, impact: []string{"media"}},
	)

	report := registry.Report(context.Background())

	if report.Status != port.StatusDegraded {
		t.Errorf("status = %q, want degraded", report.Status)
	}
	if ok, _ := registry.Ready(context.Background()); !ok {
		t.Error("the process reports itself not ready because of an optional dependency")
	}
}

// The bug this test exists for: the second failed dependency reported no impact at all, because
// the report was no longer "ok" by the time its impact was examined - so a status page showed
// one broken feature out of two.
func TestEveryFailedDependencyContributesItsImpact(t *testing.T) {
	registry := startedRegistry(
		&probe{name: "object_storage", status: port.StatusDown, impact: []string{"media"}},
		&probe{name: "smtp", status: port.StatusDown, impact: []string{"notifications"}},
	)

	report := registry.Report(context.Background())

	seen := map[string]bool{}
	for _, f := range report.DegradedFeatures {
		seen[f.Feature] = true
		if f.ReasonCode != "dependency.unavailable" {
			t.Errorf("%s carries the reason code %q", f.Feature, f.ReasonCode)
		}
	}
	for _, want := range []string{"media", "notifications"} {
		if !seen[want] {
			t.Errorf("%s is missing from the degraded features: %v", want, report.DegradedFeatures)
		}
	}
}

// A mandatory dependency down is an outage, and an optional one examined afterwards must not
// talk it back down to "degraded".
func TestAMandatoryDependencyDownStaysDown(t *testing.T) {
	registry := startedRegistry(
		&probe{name: "postgres", required: true, status: port.StatusDown, impact: []string{"all"}},
		&probe{name: "smtp", status: port.StatusDown, impact: []string{"notifications"}},
	)

	report := registry.Report(context.Background())

	if report.Status != port.StatusDown {
		t.Errorf("status = %q, want down", report.Status)
	}
	if ok, reason := registry.Ready(context.Background()); ok {
		t.Error("the process reports itself ready without its database")
	} else if reason != "postgres:down" {
		t.Errorf("the readiness reason is %q", reason)
	}
}

// A dependency nobody configured is not a fault. Reporting an unconfigured AI provider as down
// would put a permanent red on a dashboard that is working exactly as intended.
func TestADisabledDependencyIsNotAnOutage(t *testing.T) {
	registry := startedRegistry(&probe{name: "ai_provider", status: port.StatusDisabled})
	signals := newSignals()
	registry.SetSignals(signals)

	report := registry.Report(context.Background())

	if report.Status != port.StatusOK {
		t.Errorf("status = %q, want ok", report.Status)
	}
	if !signals.up["ai_provider"] {
		t.Error("a disabled dependency was published as down")
	}
}

func TestTheReportIsMirroredIntoTheMetrics(t *testing.T) {
	storage := &probe{name: "object_storage", status: port.StatusDown, impact: []string{"media"}}
	registry := startedRegistry(&probe{name: "postgres", required: true, status: port.StatusOK}, storage)
	signals := newSignals()
	registry.SetSignals(signals)

	registry.Report(context.Background())

	if signals.up["postgres"] != true {
		t.Error("postgres was not published as up")
	}
	if signals.up["object_storage"] != false {
		t.Error("the failed dependency was not published as down")
	}
	if signals.degraded["media"] != true {
		t.Error("the degraded feature was not published")
	}

	// And the recovery: a gauge that is only ever set to 1 keeps showing an outage that ended.
	storage.status, storage.impact = port.StatusOK, nil
	registry.Report(context.Background())

	if signals.up["object_storage"] != true {
		t.Error("the recovered dependency was not published as up")
	}
	if signals.degraded["media"] != false {
		t.Error("the feature is still published as degraded after the recovery")
	}
}

// The configuration warnings are the half of the report that works without any monitoring at all
// (§5) - the private user with no Prometheus reads them here.
func TestTheWarningsReachTheReport(t *testing.T) {
	registry := startedRegistry()
	registry.SetWarnings([]port.Warning{{Code: "config.backup_not_configured", Severity: "warn"}})

	report := registry.Report(context.Background())

	if len(report.Warnings) != 1 || report.Warnings[0].Code != "config.backup_not_configured" {
		t.Errorf("the warnings did not reach the report: %v", report.Warnings)
	}
}

// Liveness never touches a dependency: a liveness probe that checks the database takes down
// every pod at once during a database outage (ADR-0016).
func TestLivenessSurvivesAFailedDependency(t *testing.T) {
	registry := startedRegistry(&probe{name: "postgres", required: true, status: port.StatusDown})

	if !registry.Live() {
		t.Error("the process reports itself dead because its database is")
	}
}

func TestReadinessFollowsTheLifecycle(t *testing.T) {
	registry := NewRegistry("1.2.3", []string{"api"})

	if ok, reason := registry.Ready(context.Background()); ok || reason != "starting" {
		t.Errorf("before the start: ok=%v reason=%q", ok, reason)
	}
	registry.MarkStarted()
	if ok, _ := registry.Ready(context.Background()); !ok {
		t.Error("a started process without dependencies is not ready")
	}
	// Deregistering first is what gives the load balancer time to stop sending traffic before
	// the in-flight requests are drained (§9).
	registry.MarkClosing()
	if ok, reason := registry.Ready(context.Background()); ok || reason != "shutting_down" {
		t.Errorf("while shutting down: ok=%v reason=%q", ok, reason)
	}
}
