// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package quota

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// Signals is the metric slice the guard reports through: the approach ratio, which is what
// alert A-18 watches (observability-reliability.md §4).
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
	instance, err := g.Store.InstanceOverrides(ctx)
	if err != nil {
		return Limits{}, err
	}
	return Resolve(overrides, instance, g.Tenancy), nil
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

// AiTokens reports whether the workspace's daily AI budget for one source still has room - the
// offered model's (`ai_tokens_per_day`) or, when `own`, the workspace's own model's
// (`ai_own_tokens_per_day`) - and MeterAi meters what a call cost against the same one.
//
// Two calls rather than one, and in that order: `AiTokens` before the provider is asked, `MeterAi`
// after it answers. A budget cannot be reserved up front because nobody knows what a call will
// cost until the provider says - so the ceiling is checked against what has been spent, and a call
// that goes over it is the last one rather than one that never happened. That is the right way
// round for a bound whose purpose is to stop a runaway loop rather than to bill exactly.
//
// Not a refusal error, for `AutomationRuns`' reason: the caller's vocabulary for "out of budget" is
// already `ai.unavailable` - the one refusal the provider port answers for every reason a provider is out
// of reach - and a second shape would be a second thing for every caller to handle.
func (g Guard) AiTokens(ctx context.Context, tenant string, own bool, now time.Time) (bool, error) {
	limits, err := g.limits(ctx)
	if err != nil {
		return false, err
	}
	quota := aiQuota(own)
	limit := limits.Of(quota)
	if limit == Unlimited {
		return true, nil
	}

	day, err := g.day(ctx, now)
	if err != nil {
		return false, err
	}
	spent, err := g.Usage.MeteredSince(ctx, quota, day)
	if err != nil {
		return false, err
	}
	if g.Signals != nil {
		g.Signals.QuotaUsage(ctx, quota, tenant, Ratio(limit, spent))
	}
	return spent < limit, nil
}

// MeterAi records what one call to one source cost, on the workspace's day. Called after the
// provider answered, with what it reported.
//
// Nothing is metered for a call that failed, and that is deliberate: a provider that refused
// charged nothing, and a budget that counted refusals would tighten on itself the way the
// automation bound would if it counted its own throttles.
func (g Guard) MeterAi(ctx context.Context, own bool, at time.Time, tokens int64) error {
	if g.Meter == nil || tokens <= 0 {
		return nil
	}
	day, err := g.day(ctx, at)
	if err != nil {
		return err
	}
	return g.Meter.Add(ctx, aiQuota(own), day, tokens)
}

// AiRenewal is the next midnight in the workspace's time zone after `now`: when the day a spent AI
// budget belongs to ends. Midnight of the next calendar day rather than 24 hours on, so a day that
// a daylight-saving change makes 23 or 25 hours long still renews at midnight.
func (g Guard) AiRenewal(ctx context.Context, now time.Time) (time.Time, error) {
	day, err := g.day(ctx, now)
	if err != nil {
		return time.Time{}, err
	}
	return day.AddDate(0, 0, 1), nil
}

func aiQuota(own bool) string {
	if own {
		return AiOwnTokensPerDay
	}
	return AiTokensPerDay
}

// day is the calendar day `now` falls on in the workspace's time zone, as midnight there
// (UC-AI-07 check 2: the budget renews at the workspace's midnight). The ledger's AI rows are
// dated by that day; every other metric keeps the UTC day.
func (g Guard) day(ctx context.Context, now time.Time) (time.Time, error) {
	zone, err := g.Store.TimeZone(ctx)
	if err != nil {
		return time.Time{}, err
	}
	return StartOfDay(now, zone), nil
}

// StartOfDay is midnight of the day `now` falls on in the named zone, carrying that zone. An
// unknown or empty zone is UTC: the workspace's zone is validated when it is set, and a budget that
// refused every call over a zone name would be the worse failure.
//
// A zone changed in the middle of a day takes effect from the next call: the call is dated by the
// day it falls on in the new zone, which may be a day that already carries spend, or a fresh one.
// The ledger never moves or splits a row.
func StartOfDay(now time.Time, zone string) time.Time {
	location, err := time.LoadLocation(zone)
	if zone == "" || err != nil {
		location = time.UTC
	}
	local := now.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
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
		if err := g.Meter.Add(ctx, AutomationRunsPerHour, now.UTC(), 1); err != nil {
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
