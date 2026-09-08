// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package ai

import (
	"context"

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
// The provider answers the question rather than a separate `configured` flag, unlike the mail
// probe's: a provider's abilities are its models', so "configured" and "can actually do something"
// are one question here and asking it twice would let the two answers drift.
type Probe struct {
	provider port.Provider
	breaker  *resilience.Breaker
}

// NewProbe takes the configured provider and the breaker its adapter trips. The breaker may be nil
// where the provider trips none - Noop calls nothing, so it can open nothing.
func NewProbe(provider port.Provider, breaker *resilience.Breaker) Probe {
	return Probe{provider: provider, breaker: breaker}
}

var _ health.Probe = Probe{}

func (p Probe) Name() string   { return port.Dependency }
func (p Probe) Required() bool { return false }

func (p Probe) Check(context.Context) health.Result {
	if p.provider == nil || !p.provider.Capabilities().Enabled() {
		// Disabled is a configuration, not a fault: the registry leaves it out of the degradation
		// entirely and reports the dependency as up in the metrics.
		return health.Result{Status: health.StatusDisabled}
	}
	if p.breaker == nil {
		return health.Result{Status: health.StatusOK}
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
		result.Impact = []string{port.Feature}
	}
	return result
}
