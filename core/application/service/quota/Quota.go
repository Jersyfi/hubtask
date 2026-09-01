// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package quota turns multi-tenancy.md §4's table into enforcement (H-08): per-tenant limits
// from `tenant.settings.quotas` with the mode's defaults, one resolution, one refusal shape.
//
// The refusal split the backlog fixes: a *rate* is a 429 with Retry-After, because waiting
// helps; a *capacity* quota is a 422-shaped refusal naming the quota and the ceiling, because
// waiting does not. Every capacity code is `capacity.<quota>`, documented in
// api-guidelines.md §6.
package quota

import (
	"strconv"

	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// The quota names, exactly the §4 rows this milestone enforces per tenant. They are the
// contract's enum, the settings document's keys, the capacity codes' suffixes and the metric's
// `quota` label - one vocabulary, spelled once.
const (
	APIRequestsPerMinute  = "api_requests_per_minute"
	Items                 = "items"
	MediaBytes            = "media_bytes"
	AutomationRunsPerHour = "automation_runs_per_hour"
	WebhookTargets        = "webhook_targets"
	ExportJobs            = "export_jobs"
)

// Names is the vocabulary in the contract's order.
func Names() []string {
	return []string{
		APIRequestsPerMinute, Items, MediaBytes, AutomationRunsPerHour, WebhookTargets, ExportJobs,
	}
}

// Unlimited is the ceiling that never engages. It is a documented value, not an absence: a
// workspace may be *configured* unlimited, which survives a change of the mode's default.
const Unlimited int64 = 0

// Limits is the resolved table: every quota, one ceiling each, 0 = unlimited.
type Limits struct {
	APIRequestsPerMinute  int64
	Items                 int64
	MediaBytes            int64
	AutomationRunsPerHour int64
	WebhookTargets        int64
	ExportJobs            int64
}

// Defaults answers §4's two columns. Single mode's defaults stay effectively unlimited - the
// enforcement exists everywhere, the numbers differ, one code path (§1). "Plan-bound" rows
// default to unlimited in both modes: there is no plan system, so a bound is the operator's own
// write.
func Defaults(mode env.TenancyMode) Limits {
	if mode == env.TenancyMulti {
		return Limits{
			APIRequestsPerMinute:  600,
			Items:                 Unlimited,
			MediaBytes:            Unlimited,
			AutomationRunsPerHour: 1_000,
			WebhookTargets:        50,
			ExportJobs:            2,
		}
	}
	return Limits{
		APIRequestsPerMinute:  6_000,
		Items:                 Unlimited,
		MediaBytes:            Unlimited,
		AutomationRunsPerHour: 100_000,
		WebhookTargets:        Unlimited,
		ExportJobs:            5,
	}
}

// Resolve lays the workspace's overrides over the mode's defaults.
func Resolve(overrides repository.Overrides, mode env.TenancyMode) Limits {
	limits := Defaults(mode)
	apply := func(target *int64, override *int64) {
		if override != nil {
			*target = *override
		}
	}
	apply(&limits.APIRequestsPerMinute, overrides.APIRequestsPerMinute)
	apply(&limits.Items, overrides.Items)
	apply(&limits.MediaBytes, overrides.MediaBytes)
	apply(&limits.AutomationRunsPerHour, overrides.AutomationRunsPerHour)
	apply(&limits.WebhookTargets, overrides.WebhookTargets)
	apply(&limits.ExportJobs, overrides.ExportJobs)
	return limits
}

// Of answers one quota's ceiling by name.
func (l Limits) Of(quota string) int64 {
	switch quota {
	case APIRequestsPerMinute:
		return l.APIRequestsPerMinute
	case Items:
		return l.Items
	case MediaBytes:
		return l.MediaBytes
	case AutomationRunsPerHour:
		return l.AutomationRunsPerHour
	case WebhookTargets:
		return l.WebhookTargets
	case ExportJobs:
		return l.ExportJobs
	default:
		return Unlimited
	}
}

// Exceeded is the capacity refusal: 422-shaped, naming the quota and the ceiling, because
// waiting does not help - what helps is removing something, or the operator moving the wall.
func Exceeded(quota string, limit, used int64) error {
	return shared.ErrValidation.
		WithDetail("capacity." + quota).
		WithParams(map[string]string{
			"quota": quota,
			"limit": strconv.FormatInt(limit, 10),
			"used":  strconv.FormatInt(used, 10),
		})
}

// Ratio is used/limit, the number the metric reports; 0 while the limit is unlimited.
func Ratio(limit, used int64) float64 {
	if limit <= 0 {
		return 0
	}
	return float64(used) / float64(limit)
}
