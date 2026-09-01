// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package quota

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// Signals is the metric slice the guard reports through: the approach ratio, which is what
// finally gives A-18 something to watch (observability-reliability.md §4).
type Signals interface {
	// QuotaUsage records used/limit for one quota of one workspace. The tenant travels as an
	// identifier and reaches the series only when the operator enabled the tenant label (§3.2).
	QuotaUsage(ctx context.Context, quota string, tenant string, ratio float64)
}

// Guard is the one capacity decision, asked from wherever a quota-bounded thing is created.
//
// Every check runs inside the caller's own transaction: the count the refusal is based on and
// the write it guards see the same database. The check is check-then-act and deliberately not
// serialised - two concurrent creates can both pass at limit-1 and land one row over the wall.
// A quota is a wall against runaway growth, not an invariant; making it one would cost a lock
// on every create for a race that self-corrects at the very next request.
type Guard struct {
	Store   repository.Store
	Usage   repository.Usage
	Meter   repository.Meter
	Signals Signals
	Tenancy env.TenancyMode
}

// limits resolves the transaction's tenant's table.
func (g Guard) limits(ctx context.Context) (Limits, error) {
	overrides, err := g.Store.Overrides(ctx)
	if err != nil {
		return Limits{}, err
	}
	return Resolve(overrides, g.Tenancy), nil
}

// check is every capacity quota's shape: resolve, count, report the ratio, refuse at the wall.
// `adding` is what the caller is about to create - the refusal happens before the wall is
// breached, not after.
func (g Guard) check(
	ctx context.Context, tenant, quota string, limit int64,
	count func(context.Context) (int64, error), adding int64,
) error {
	if limit == Unlimited {
		return nil
	}
	used, err := count(ctx)
	if err != nil {
		return err
	}
	if g.Signals != nil {
		g.Signals.QuotaUsage(ctx, quota, tenant, Ratio(limit, used))
	}
	if used+adding > limit {
		return Exceeded(quota, limit, used)
	}
	return nil
}

// Items refuses when the workspace's items would pass their ceiling.
func (g Guard) Items(ctx context.Context, tenant string, adding int64) error {
	limits, err := g.limits(ctx)
	if err != nil {
		return err
	}
	return g.check(ctx, tenant, Items, limits.Items, g.Usage.Items, adding)
}

// MediaBytes refuses when storing `adding` more bytes would pass the ceiling.
func (g Guard) MediaBytes(ctx context.Context, tenant string, adding int64) error {
	limits, err := g.limits(ctx)
	if err != nil {
		return err
	}
	return g.check(ctx, tenant, MediaBytes, limits.MediaBytes, g.Usage.MediaBytes, adding)
}

// WebhookTargets refuses when another subscription would pass the ceiling.
func (g Guard) WebhookTargets(ctx context.Context, tenant string) error {
	limits, err := g.limits(ctx)
	if err != nil {
		return err
	}
	return g.check(ctx, tenant, WebhookTargets, limits.WebhookTargets, g.Usage.WebhookTargets, 1)
}

// ExportJobs refuses when another live export would pass the ceiling.
func (g Guard) ExportJobs(ctx context.Context, tenant string) error {
	limits, err := g.limits(ctx)
	if err != nil {
		return err
	}
	return g.check(ctx, tenant, ExportJobs, limits.ExportJobs, g.Usage.LiveExports, 1)
}

// AutomationRuns reports whether the workspace's hourly budget still has room, and meters the
// run either way. Not a refusal error: the caller is the rule engine, and its vocabulary for
// "over budget" is a THROTTLED run row - visible, counted, never executed - rather than a
// problem document nobody is there to read.
func (g Guard) AutomationRuns(ctx context.Context, tenant string, now time.Time) (bool, error) {
	limits, err := g.limits(ctx)
	if err != nil {
		return false, err
	}
	if g.Meter != nil {
		// The billing ledger's daily tally (usage_record) - capacity planning's number, never
		// the enforcement's source: the enforcement counts live from the run log.
		if err := g.Meter.Add(ctx, AutomationRunsPerHour, now, 1); err != nil {
			return false, err
		}
	}
	if limits.AutomationRunsPerHour == Unlimited {
		return true, nil
	}
	ran, err := g.Usage.AutomationRunsSince(ctx, now.Add(-time.Hour))
	if err != nil {
		return false, err
	}
	if g.Signals != nil {
		g.Signals.QuotaUsage(ctx, AutomationRunsPerHour, tenant, Ratio(limits.AutomationRunsPerHour, ran))
	}
	return ran < limits.AutomationRunsPerHour, nil
}
