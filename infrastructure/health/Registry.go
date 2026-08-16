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

type Registry struct {
	mu      sync.RWMutex
	probes  []port.Probe
	started bool
	closing bool

	version  string
	roles    []string
	warnings []port.Warning
}

func NewRegistry(version string, roles []string) *Registry {
	return &Registry{version: version, roles: roles}
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
		switch {
		case res.Status == port.StatusDown && p.Required():
			rep.Status = port.StatusDown
		case res.Status == port.StatusDown && rep.Status == port.StatusOK:
			// Optional dependency gone: reduced functionality, the system keeps running.
			rep.Status = port.StatusDegraded
			for _, f := range res.Impact {
				rep.DegradedFeatures = append(rep.DegradedFeatures, port.DegradedFeature{
					Feature: f, ReasonCode: "dependency.unavailable", Since: res.Since,
				})
			}
		}
	}
	return rep
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
