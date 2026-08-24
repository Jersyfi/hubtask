// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"context"

	health "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
)

// Probe reports the mail server's health to the registry by reading the breaker rather than
// dialling it - a probe that opened an SMTP session on every scrape would be a probe some servers
// rate-limit, and one that called a target the breaker has cut off would undo the breaker's whole
// purpose.
//
// Optional by contract (core/port/health), and that is the point of the whole design: SMTP down
// restricts notifications and nothing else. The write path stays open, the comment that caused the
// notification is committed, and the message waits (observability-reliability.md §7).
type Probe struct {
	breaker *resilience.Breaker
	// configured is whether this installation has a mail server at all. An installation that never
	// set HUBTASK_SMTP_HOST is not broken, it is one that does not send email - and reporting that
	// as an outage would have an operator paging themselves over a decision they made.
	configured bool
}

// NewProbe takes the breaker the resilient sender trips, and whether mail is configured.
func NewProbe(breaker *resilience.Breaker, configured bool) Probe {
	return Probe{breaker: breaker, configured: configured}
}

var _ health.Probe = Probe{}

func (p Probe) Name() string   { return Dependency }
func (p Probe) Required() bool { return false }

func (p Probe) Check(context.Context) health.Result {
	if !p.configured {
		// Disabled is a configuration, not a fault: the registry leaves it out of the degradation
		// entirely and reports the dependency as up in the metrics.
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
		result.Impact = []string{NotificationsFeature}
	}
	return result
}
