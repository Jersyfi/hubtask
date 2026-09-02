// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package eventbus

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	health "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
)

// BusDependency is what the bus is called in /meta/health and in the degraded-mode metric. The set
// of dependency names is small and written by hand, which is what keeps it a label.
const BusDependency = "bus"

// BusFeature is what a person loses when the bus is gone, and the honest answer is: nothing they
// can see. The events keep being written, the webhooks keep going out, and the bus catches up when
// it returns - which is the row observability-reliability.md §7 has always carried for NATS, "no
// visible change". It is named anyway, because a report that says a dependency is down and lists
// no impact reads like an omission rather than like good news.
const BusFeature = "event_bus"

// Probe reports the bus's health by reading the breaker rather than dialling it. A probe that
// published a message on every scrape would put a message on somebody's stream every fifteen
// seconds, and one that called past a breaker that is open would undo the breaker.
type Probe struct {
	breaker *resilience.Breaker
	// configured is whether this installation has a bus at all. An installation that never set a
	// URL is not broken; reporting it as an outage would have an operator paged over a decision
	// they made by not making one.
	configured bool
}

// NewProbe takes the breaker the resilient bus trips, and whether a bus is configured.
func NewProbe(breaker *resilience.Breaker, configured bool) Probe {
	return Probe{breaker: breaker, configured: configured}
}

var _ health.Probe = Probe{}

func (p Probe) Name() string   { return BusDependency }
func (p Probe) Required() bool { return false }

func (p Probe) Check(context.Context) health.Result {
	if !p.configured {
		return health.Result{Status: health.StatusDisabled}
	}

	state := p.breaker.State()
	result := health.Result{
		Status:       health.StatusOK,
		Since:        p.breaker.Since(),
		CircuitState: state.String(),
	}
	if state != resilience.BreakerClosed {
		result.Status = health.StatusDown
		result.ErrorCode = "dependency.unavailable"
		result.Impact = []string{BusFeature}
	}
	return result
}

// Resilient composes the publisher with the breaker (ADR-0016).
//
// No bulkhead, unlike the mail sender: the publishes are already bounded by the worker pool that
// runs the jobs, and a second compartment inside the first would be a limit on a limit. What the
// breaker buys is what it buys everywhere - a bus that is down costs an immediate answer instead
// of every queued job spending the publish timeout finding out what the one before it knew.
type Resilient struct {
	inner   Bus
	breaker *resilience.Breaker
}

// NewResilient wraps the bus. One breaker per dependency, owned by the composition root - the
// probe above reads the same one.
func NewResilient(inner Bus, breaker *resilience.Breaker) Resilient {
	return Resilient{inner: inner, breaker: breaker}
}

var _ Bus = Resilient{}

func (r Resilient) Publish(ctx context.Context, tenantID shared.ID, eventType string, payload []byte) error {
	return r.breaker.Do(ctx, func(ctx context.Context) error {
		return r.inner.Publish(ctx, tenantID, eventType, payload)
	})
}
