// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"context"

	health "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
)

// mediaFeature is what a person cares about when the bucket is gone: attachments, not S3
// (observability-reliability.md §7). The bare feature name the degradation report carries.
const mediaFeature = "media"

// Probe reports the object storage's health to the registry, by reading the breaker rather than
// dialling the endpoint: a probe that called a target the breaker has cut off would undo the
// breaker's whole purpose - and would make /meta/health as slow as the outage it reports.
//
// Optional by contract (core/port/health): the failure of the object storage restricts media and
// nothing else, and must never block the write path or readiness (QS-11).
type Probe struct {
	breaker *resilience.Breaker
}

// NewProbe takes the breaker the resilient store trips.
func NewProbe(breaker *resilience.Breaker) Probe {
	return Probe{breaker: breaker}
}

var _ health.Probe = Probe{}

func (p Probe) Name() string   { return s3Dependency }
func (p Probe) Required() bool { return false }

func (p Probe) Check(context.Context) health.Result {
	state := p.breaker.State()
	result := health.Result{
		Status:       health.StatusOK,
		Since:        p.breaker.Since(),
		CircuitState: state.String(),
	}
	if state != resilience.BreakerClosed {
		result.Status = health.StatusDown
		result.ErrorCode = "dependency.unavailable"
		result.Impact = []string{mediaFeature}
	}
	return result
}
