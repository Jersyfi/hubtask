// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build resilience

// Package resilience holds the RT series from observability-reliability.md §12: what the system
// does when something breaks.
//
// RT-1 is the rule that ADR-0016 calls the point of the whole degradation model: the failure of
// an optional dependency must not block the core write path, must be visible as a degraded
// feature, and must heal without a restart. It runs against the real building blocks - the
// breaker, the guarded client, the health registry, and the metrics - because the property is a
// property of their composition, not of any one of them.
//
// The optional dependency here is an HTTP service standing in for object storage. When the S3,
// SMTP, and AI adapters exist, RT-1 grows a container-backed sibling; the composition it checks
// is the same one.
package resilience

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	envport "github.com/Jersyfi/hubtask/core/port/environment"
	healthport "github.com/Jersyfi/hubtask/core/port/health"
	clientport "github.com/Jersyfi/hubtask/core/port/httpclient"
	healthadapter "github.com/Jersyfi/hubtask/infrastructure/health"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
	"github.com/Jersyfi/hubtask/infrastructure/observability"
	res "github.com/Jersyfi/hubtask/infrastructure/resilience"
)

// mediaFeature is what the optional dependency serves. It is a feature name, not a dependency
// name: a user cares that attachments are unavailable, not that a bucket is.
const mediaFeature = "media"

// clock is the injected time source, so that the breaker's cool-down can pass without the test
// waiting for it.
type clock struct{ now atomic.Int64 }

func newClock() *clock {
	c := &clock{}
	c.now.Store(time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC).UnixNano())
	return c
}

func (c *clock) Now() time.Time          { return time.Unix(0, c.now.Load()).UTC() }
func (c *clock) advance(d time.Duration) { c.now.Add(int64(d)) }

// objectStorage is the optional dependency: an HTTP service that can be switched off and on
// again without restarting anything.
type objectStorage struct {
	server *httptest.Server
	up     atomic.Bool
}

func newObjectStorage() *objectStorage {
	s := &objectStorage{}
	s.up.Store(true)
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !s.up.Load() {
			// A refused connection would be the more realistic outage, but a 500 keeps the
			// test's timing deterministic - the classification is the same either way.
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	return s
}

func (s *objectStorage) stop()  { s.up.Store(false) }
func (s *objectStorage) start() { s.up.Store(true) }
func (s *objectStorage) close() { s.server.Close() }

// mediaProbe reports the optional dependency for /meta/health. It reads the breaker rather than
// calling the dependency: a probe that dials a target the breaker has cut off would undo the
// breaker's whole purpose.
type mediaProbe struct {
	breaker *res.Breaker
}

func (p mediaProbe) Name() string   { return "object_storage" }
func (p mediaProbe) Required() bool { return false }

func (p mediaProbe) Check(context.Context) healthport.Result {
	state := p.breaker.State()
	result := healthport.Result{
		Status:       healthport.StatusOK,
		Since:        p.breaker.Since(),
		CircuitState: state.String(),
	}
	if state != res.BreakerClosed {
		result.Status = healthport.StatusDown
		result.ErrorCode = "dependency.unavailable"
		result.Impact = []string{mediaFeature}
	}
	return result
}

func TestRT1AnOptionalDependencyFailingDoesNotBlockTheWritePath(t *testing.T) {
	storage := newObjectStorage()
	defer storage.close()

	c := newClock()
	cfg := envport.OutboundConfig{
		Timeout:              500 * time.Millisecond,
		ConnectTimeout:       200 * time.Millisecond,
		MaxResponseBytes:     1 << 16,
		MaxRedirects:         1,
		AllowPrivateNetworks: true, // the test server listens on loopback
	}
	client := httpclient.NewGuardedClient(cfg, httpclient.NewGuard(cfg))

	metrics, err := observability.NewMetrics(envport.Config{Version: "test", Commit: "test"})
	if err != nil {
		t.Fatalf("building the metrics: %v", err)
	}
	defer func() { _ = metrics.Shutdown(context.Background()) }()

	breaker := res.NewBreaker(res.BreakerConfig{
		Dependency: "object_storage", FailureThreshold: 2, SuccessThreshold: 1,
		OpenFor: 30 * time.Second, Now: c.Now,
		OnStateChange: func(dependency string, state res.BreakerState) {
			metrics.CircuitBreakerState(context.Background(), dependency, state.Level())
		},
	})

	registry := healthadapter.NewRegistry("test", []string{"api"})
	registry.Register(mediaProbe{breaker: breaker})
	registry.SetSignals(metrics)
	registry.MarkStarted()

	// storeMedia is the optional path: everything that needs the object storage.
	storeMedia := func(ctx context.Context) error {
		return breaker.Do(ctx, func(callCtx context.Context) error {
			resp, err := client.Do(callCtx, clientport.Request{
				URL: storage.server.URL + "/media", TargetClass: "object_storage",
			})
			if err != nil {
				return err
			}
			if resp.Status >= 500 {
				// The target answered, so this is not a transport error - the adapter is the
				// one that decides a 5xx means the dependency is unwell (see port.Response).
				return shared.ErrUnavailable.
					WithDetail("dependency.unavailable").
					WithParams(map[string]string{"dependency": "object_storage"})
			}
			return nil
		})
	}

	// createTask stands in for the core write path. It touches nothing but the database, which
	// is the claim under test: no optional dependency sits between a user and their data. It is
	// a stub here because there is no use case yet - A-07 replaces it with CreateContainer
	// against a real PostgreSQL, and the assertions around it stay as they are.
	var written int
	createTask := func(context.Context) error {
		written++
		return nil
	}

	ctx := context.Background()

	// --- Everything up ---------------------------------------------------------------------
	if err := storeMedia(ctx); err != nil {
		t.Fatalf("the media path failed while the dependency was up: %v", err)
	}
	if report := registry.Report(ctx); report.Status != healthport.StatusOK {
		t.Fatalf("status = %s while everything is up, want ok", report.Status)
	}

	// --- The dependency goes down ----------------------------------------------------------
	storage.stop()
	for range 3 {
		if err := storeMedia(ctx); err == nil {
			t.Error("the media path succeeded although the dependency is down")
		}
	}
	if state := breaker.State(); state != res.BreakerOpen {
		t.Fatalf("breaker = %s after repeated failures, want open", state)
	}

	// The core write path stays open, and it stays fast: an open breaker answers without
	// dialling, so nothing about the outage reaches a user creating a task.
	start := time.Now()
	for range 20 {
		if err := createTask(ctx); err != nil {
			t.Fatalf("the write path was blocked by the outage: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("20 writes took %v during the outage - the write path is waiting on something", elapsed)
	}
	if written != 20 {
		t.Errorf("writes = %d, want 20", written)
	}

	// The outage is reported rather than hidden: degraded, not down, and with the feature named.
	report := registry.Report(ctx)
	if report.Status != healthport.StatusDegraded {
		t.Errorf("status = %s during the outage, want degraded", report.Status)
	}
	if !hasFeature(report.DegradedFeatures, mediaFeature) {
		t.Errorf("degraded features = %v, want %s", report.DegradedFeatures, mediaFeature)
	}
	if ready, reason := registry.Ready(ctx); !ready {
		t.Errorf("the process reported itself unready over an optional dependency: %s", reason)
	}

	body := scrape(t, metrics)
	for _, want := range []string{
		`hubtask_circuit_breaker_state{dependency="object_storage"} 2`,
		`hubtask_dependency_up{dependency="object_storage"} 0`,
		`hubtask_degraded_mode{feature="media"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the outage is missing from the metrics: %s", want)
		}
	}

	// --- Recovery, without a restart ---------------------------------------------------------
	storage.start()
	c.advance(31 * time.Second) // the cool-down passes

	if err := storeMedia(ctx); err != nil {
		t.Fatalf("the probe after the cool-down failed: %v", err)
	}
	if state := breaker.State(); state != res.BreakerClosed {
		t.Fatalf("breaker = %s after recovery, want closed", state)
	}

	report = registry.Report(ctx)
	if report.Status != healthport.StatusOK {
		t.Errorf("status = %s after recovery, want ok", report.Status)
	}
	if len(report.DegradedFeatures) != 0 {
		t.Errorf("degraded features = %v after recovery, want none", report.DegradedFeatures)
	}

	// A gauge that only ever goes up keeps showing an outage that ended hours ago.
	body = scrape(t, metrics)
	for _, want := range []string{
		`hubtask_circuit_breaker_state{dependency="object_storage"} 0`,
		`hubtask_dependency_up{dependency="object_storage"} 1`,
		`hubtask_degraded_mode{feature="media"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the recovery is missing from the metrics: %s", want)
		}
	}
}

func hasFeature(features []healthport.DegradedFeature, name string) bool {
	for _, f := range features {
		if f.Feature == name {
			return true
		}
	}
	return false
}

func scrape(t *testing.T, metrics *observability.Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec,
		httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("scrape returned %d", rec.Code)
	}
	return rec.Body.String()
}
