// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package quota holds the outbound ports of the §4 limits (multi-tenancy.md).
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
	// AiOwnTokensPerDay is the workspace's own budget for its own model. Written by the workspace,
	// never by the operator's quota write, and stored apart from the quotas key so that write cannot
	// drop it (SetOwnAiBudget).
	AiOwnTokensPerDay *int64
}

// Store reads and writes the overrides in the tenant's own settings document
// (`tenant.settings.quotas` - §4's home for per-tenant knobs).
type Store interface {
	// Overrides answers what the transaction's tenant configured. A workspace that configured
	// nothing answers the zero value, not an error.
	Overrides(ctx context.Context) (Overrides, error)

	// InstanceOverrides answers what the **installation** set, which every workspace that set
	// nothing of its own falls back to (ADR-0070 §2, the concept's §6.7).
	//
	// It is on this port rather than on a second one because it is the same question one level up:
	// "what limits apply here" has three answers now — the product's table, the installation's, and
	// the workspace's — and a caller that had to consult two ports would be a caller that
	// eventually consults one. `instance_setting` carries no row level security, so this read works
	// inside the tenant's own transaction.
	//
	// An installation that set nothing answers the zero value, not an error.
	InstanceOverrides(ctx context.Context) (Overrides, error)

	// InstanceLocks names the quotas the installation locked, by their contract name: a workspace's
	// screen says "set by the installation" there (UC-INS-09 check 5). Beside the overrides rather
	// than in them, because an override is a ceiling a level may leave unset and a lock is not.
	InstanceLocks(ctx context.Context) (map[string]bool, error)

	// SetOverrides replaces the quotas key of the settings document, guarded on the row
	// version. False means the version moved under the caller. AiOwnTokensPerDay is not written:
	// it is not the operator's.
	SetOverrides(ctx context.Context, overrides Overrides, expectedVersion int, now time.Time) (bool, error)

	// SetOwnAiBudget writes the workspace's own budget for its own AI model; nil removes it.
	SetOwnAiBudget(ctx context.Context, tokens *int64, now time.Time) error

	// TimeZone answers the workspace's time zone, the one its AI budget's day ends in. An empty
	// answer is UTC.
	TimeZone(ctx context.Context) (string, error)
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

	// MeteredSince sums one metered thing since the instant.
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
	// `since` names a calendar day by its date in the location it carries, as `Meter.Add` does.
	MeteredSince(ctx context.Context, metric string, since time.Time) (int64, error)
}

// Meter records usage into the billing ledger (`usage_record`) - daily tallies for capacity
// planning and the dashboards, never the enforcement's source: the enforcement counts live,
// because a ledger row can lag and a limit that lags is a limit that lies.
type Meter interface {
	// Add increases one metric's tally for the calendar day `at` falls on **in the location it
	// carries** - the caller decides whose day it is - creating the row on first use.
	Add(ctx context.Context, metric string, at time.Time, amount int64) error
}
