// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package ai

import (
	"context"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/ai"
	health "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
)

// Probe reports the AI provider to the registry, by asking the provider what it can do and reading
// the breaker rather than calling the endpoint - the discipline the object storage and the mail
// probes already follow: a probe that called a target the breaker has cut off would undo the
// breaker's purpose and make /meta/health as slow as the outage it reports.
//
// Optional by contract (core/port/health), and this is the dependency the whole optional-dependency
// design was written for: without a provider the product is complete (QS-09), so a missing one is
// `disabled` and never `down`. The distinction is the difference between an operator who made a
// decision and an operator who has an outage, and reporting the first as the second is how an alert
// catalogue teaches people to ignore it.
//
// It asks the *pool* rather than one breaker, because a provider is per tenant (ai-first.md §2) and
// there is no single endpoint to watch. What it reports is the installation's view - "some
// configured provider is cut off" - which is what an operator reading /meta/health is asking, and
// which endpoint it was is deliberately not in the answer: an endpoint is a tenant's configuration
// and naming one here would put a customer into a health report (rule 10).
//
// What it reads is whether this process has *called* a provider, which is the pool being non-empty.
// It is deliberately not "has any workspace configured one": that question is a read across
// tenants, and nothing in this system enumerates tenants (multi-tenancy.md §2.1) - a health probe
// is not where that rule would be worth breaking. So a freshly started process reports `disabled`
// until the first call, which is a true statement about what it is doing rather than a guess about
// what its workspaces have configured, and the degradation table's concern - "AI suggestions
// disappear" - is about calls that fail rather than about calls nobody has made.
type Probe struct {
	pool *BreakerPool
}

// NewProbe takes the pool of endpoint breakers. Nil is an installation with no AI surface running.
func NewProbe(pool *BreakerPool) Probe {
	return Probe{pool: pool}
}

var _ health.Probe = Probe{}

func (p Probe) Name() string   { return port.Dependency }
func (p Probe) Required() bool { return false }

func (p Probe) Check(context.Context) health.Result {
	if p.pool == nil || p.pool.Size() == 0 {
		// Disabled is a configuration, not a fault: the registry leaves it out of the degradation
		// entirely and reports the dependency as up in the metrics. An installation that never
		// wanted AI must not report itself degraded for ever.
		return health.Result{Status: health.StatusDisabled}
	}

	open, since := p.pool.Open(func(breaker Breaker) (bool, time.Time) {
		stateful, holds := breaker.(interface {
			State() resilience.BreakerState
			Since() time.Time
		})
		if !holds {
			return false, time.Time{}
		}
		return stateful.State() != resilience.BreakerClosed, stateful.Since()
	})
	if open == 0 {
		return health.Result{Status: health.StatusOK, CircuitState: "closed"}
	}
	return health.Result{
		Status: health.StatusDown,
		// Since when the earliest cut-off endpoint has been cut off. A degradation without a
		// timestamp is one nobody can tell from an old one.
		Since:        since,
		CircuitState: "open",
		ErrorCode:    "dependency.unavailable",
		Impact:       []string{port.Feature},
	}
}
