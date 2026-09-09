// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package quota holds the outbound ports of the §4 limits (multi-tenancy.md, H-08).
//
// No method takes a tenant: every read and write is bounded by the transaction it runs in, like
// every repository (ADR-0010). The quota vocabulary itself - names, defaults, refusals - lives
// in the service package; this is only what the database is asked.
package quota

import (
	"context"
	"time"
)

// Overrides is what the workspace configured, key by key: nil is "the mode's default applies",
// a value is the workspace's own ceiling, and 0 is "unlimited" - three states, which is why
// these are pointers and not numbers.
type Overrides struct {
	APIRequestsPerMinute  *int64
	Items                 *int64
	MediaBytes            *int64
	AutomationRunsPerHour *int64
	WebhookTargets        *int64
	ExportJobs            *int64
	AiTokensPerDay        *int64
}

// Store reads and writes the overrides in the tenant's own settings document
// (`tenant.settings.quotas` - §4's home for per-tenant knobs).
type Store interface {
	// Overrides answers what the transaction's tenant configured. A workspace that configured
	// nothing answers the zero value, not an error.
	Overrides(ctx context.Context) (Overrides, error)

	// SetOverrides replaces the quotas key of the settings document, guarded on the row
	// version. False means the version moved under the caller.
	SetOverrides(ctx context.Context, overrides Overrides, expectedVersion int, now time.Time) (bool, error)
}

// Usage answers the live counts the capacity quotas are measured against. Each is one bounded
// query under the transaction's own tenant.
type Usage interface {
	// Items counts the workspace's work items, trash included: a row occupies its place until
	// the retention machinery lets it go.
	Items(ctx context.Context) (int64, error)

	// MediaBytes sums the stored objects' sizes, soft-deleted ones excluded - their bytes are
	// the reconciliation job's to reclaim, not the workspace's to answer for.
	MediaBytes(ctx context.Context) (int64, error)

	// WebhookTargets counts the live subscriptions.
	WebhookTargets(ctx context.Context) (int64, error)

	// AutomationRunsSince counts the runs that actually ran since the instant - THROTTLED ones
	// excluded, CountRunsSince's reasoning: counting the refusals would make the bound tighten
	// on itself.
	AutomationRunsSince(ctx context.Context, since time.Time) (int64, error)

	// LiveExports counts the pending and running export jobs.
	LiveExports(ctx context.Context) (int64, error)

	// MeteredSince sums one metered thing since the instant (J-15).
	//
	// **Read from the billing ledger, which every other quota here refuses to do** - and the
	// exception is the point rather than an oversight. The ledger is barred as an enforcement
	// source because it is a lagging copy of something countable: items are in `work_item`, media
	// in `media_object`, and a count that read the tally could lag behind the rows. A token has no
	// row. It was spent on somebody else's machine and the tally *is* the record, so there is
	// nothing more authoritative to count - and a budget that refused to read the only record of
	// what it bounds would not be a budget.
	// Generic in the metric, like `Meter.Add` beside it: the vocabulary is the application
	// layer's - `quota.Names()` - and an adapter that spelled one of those names would be a second
	// place the spelling lives.
	MeteredSince(ctx context.Context, metric string, since time.Time) (int64, error)
}

// Meter records usage into the billing ledger (`usage_record`) - daily tallies for capacity
// planning and the dashboards, never the enforcement's source: the enforcement counts live,
// because a ledger row can lag and a limit that lags is a limit that lies.
type Meter interface {
	// Add increases one metric's tally for the period holding `at`, creating the row on first
	// use.
	Add(ctx context.Context, metric string, at time.Time, amount int64) error
}
