// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package health implements the health port.
package health

import (
	"context"
	"sync"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/health"
)

// SignalSink mirrors the health model into the metrics, so that a disruption has a beginning and
// an end in the same place as everything else (§4: hubtask_dependency_up, hubtask_degraded_mode).
//
// An interface rather than the metrics adapter itself: the registry is checked by tests that
// have no meter, and the health model should not depend on how it is exported.
type SignalSink interface {
	DependencyUp(ctx context.Context, dependency string, up bool)
	DegradedMode(ctx context.Context, feature string, degraded bool)
}

type Registry struct {
	mu      sync.RWMutex
	probes  []port.Probe
	started bool
	closing bool

	version  string
	roles    []string
	warnings []port.Warning
	signals  SignalSink
	// degradedSeen remembers which features have ever been reported as degraded, so that
	// recovery can be published as a zero. A gauge that is only ever set to 1 never comes back
	// down, and the dashboard keeps showing an outage that ended hours ago.
	degradedSeen map[string]bool
}

func NewRegistry(version string, roles []string) *Registry {
	return &Registry{version: version, roles: roles, degradedSeen: map[string]bool{}}
}

// SetSignals wires the report into the metrics. Optional: a process without a meter still
// answers /readyz and /meta/health.
func (r *Registry) SetSignals(sink SignalSink) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signals = sink
}

func (r *Registry) Register(p port.Probe) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.probes = append(r.probes, p)
}

func (r *Registry) SetWarnings(w []port.Warning) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.warnings = w
}

func (r *Registry) MarkStarted() { r.mu.Lock(); r.started = true; r.mu.Unlock() }
func (r *Registry) MarkClosing() { r.mu.Lock(); r.closing = true; r.mu.Unlock() }

func (r *Registry) Started() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.started
}

// Live checks only whether the process responds - NEVER dependencies.
// A liveness probe that checks the database takes down every pod at once during a database
// outage, turning a disruption into a total outage (ADR-0016).
func (r *Registry) Live() bool { return true }

// Ready checks the mandatory dependencies and the shutdown state.
func (r *Registry) Ready(ctx context.Context) (bool, string) {
	r.mu.RLock()
	closing, started := r.closing, r.started
	probes := append([]port.Probe(nil), r.probes...)
	r.mu.RUnlock()

	if closing {
		return false, "shutting_down"
	}
	if !started {
		return false, "starting"
	}
	for _, p := range probes {
		if !p.Required() {
			continue
		}
		if res := p.Check(ctx); res.Status != port.StatusOK {
			return false, p.Name() + ":" + string(res.Status)
		}
	}
	return true, ""
}

// Report produces the deep self-diagnosis for /api/v1/meta/health.
func (r *Registry) Report(ctx context.Context) port.Report {
	r.mu.RLock()
	probes := append([]port.Probe(nil), r.probes...)
	rep := port.Report{Version: r.version, Roles: r.roles, Warnings: r.warnings}
	r.mu.RUnlock()

	rep.Status = port.StatusOK
	for _, p := range probes {
		res := p.Check(ctx)
		rep.Dependencies = append(rep.Dependencies, port.DependencyReport{
			Name: p.Name(), Required: p.Required(), Result: res,
		})

		// The status aggregation and the impact list are two separate questions. Deciding them
		// in one switch meant a second failed dependency reported no impact at all, because the
		// report was no longer "ok" by the time it was examined.
		switch res.Status {
		case port.StatusDown:
			if p.Required() {
				// A mandatory dependency down is not a degradation, it is an outage - and it
				// must not be talked back down to "degraded" by an optional one after it.
				rep.Status = port.StatusDown
			} else if rep.Status == port.StatusOK {
				// An optional dependency gone means reduced functionality; the system keeps
				// running and the write path stays open (ADR-0016).
				rep.Status = port.StatusDegraded
			}
		case port.StatusDegraded:
			if rep.Status == port.StatusOK {
				rep.Status = port.StatusDegraded
			}
		case port.StatusOK, port.StatusDisabled:
			// Nothing to report. Disabled is a configuration, not a fault.
			continue
		}

		for _, f := range res.Impact {
			rep.DegradedFeatures = append(rep.DegradedFeatures, port.DegradedFeature{
				Feature: f, ReasonCode: "dependency.unavailable", Since: res.Since,
			})
		}
	}

	r.publish(ctx, rep)
	return rep
}

// publish mirrors the report into the metrics.
//
// It runs whenever the report is produced rather than on a timer of its own: /readyz is polled
// continuously by the readiness probe and /meta/health by whoever is watching, and a second loop
// would only mean the dependencies get pinged twice.
func (r *Registry) publish(ctx context.Context, rep port.Report) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.signals == nil {
		return
	}

	for _, d := range rep.Dependencies {
		// Disabled is not down: an AI provider nobody configured should not read as an outage.
		r.signals.DependencyUp(ctx, d.Name, d.Status == port.StatusOK || d.Status == port.StatusDisabled)
	}

	degraded := make(map[string]bool, len(rep.DegradedFeatures))
	for _, f := range rep.DegradedFeatures {
		degraded[f.Feature] = true
		r.degradedSeen[f.Feature] = true
	}
	for feature := range r.degradedSeen {
		if !degraded[feature] {
			r.signals.DegradedMode(ctx, feature, false)
		}
	}
	for feature := range degraded {
		r.signals.DegradedMode(ctx, feature, true)
	}
}

// StaticProbe is a placeholder until the real adapters exist.
type StaticProbe struct {
	ProbeName  string
	IsRequired bool
	Fixed      port.Status
	Effects    []string
}

func (s StaticProbe) Name() string   { return s.ProbeName }
func (s StaticProbe) Required() bool { return s.IsRequired }
func (s StaticProbe) Check(context.Context) port.Result {
	return port.Result{Status: s.Fixed, Since: time.Time{}, Impact: s.Effects}
}
