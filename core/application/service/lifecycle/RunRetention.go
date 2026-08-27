// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// RunRetention removes what one tenant's periods say may go.
//
// Internal, and deliberately not in the use case catalogue. The catalogue is the list of things a
// person, an agent or a rule can ask for (arc42 §4), and this is not one of them: it is what the
// installation does on its own, and the way to influence it is to change the period rather than to
// call the sweep. Registering it would put a "delete everything that is due, now" action on three
// channels, which is a button nobody should be given.
//
// One pass per call. The queue's handler runs inside the transaction the runner opened
// (core/port/queue), so a run that looped would hold one transaction open across a whole deletion -
// and the batching data-retention.md §5 asks for is exactly the opposite of that. The job comes back
// for the next pass instead, which is also what makes a large deletion stoppable between passes
// rather than only by killing it.
type RunRetention struct {
	Policies repository.Policies
	Runs     repository.Runs
	Purger   Purger
	// History is the notification record's remover (C-09). A second kind rather than a second job,
	// because a tenant's periods are one thing to evaluate: two schedules would mean two leases,
	// two logs and two ways for one of them to quietly stop running.
	History NotificationHistory
	// Events is the outbox's own remover. Optional, like Rules and Sweeper below: an installation
	// wired without it sweeps exactly what it did before, which is what lets the two land in
	// separate releases.
	Events DispatchedEvents
	Clock  clock.Clock
	IDs    clock.IDGenerator
	// Signals is the observability slice. Optional: a run without it still runs, which is what keeps
	// a metrics adapter from being a dependency of the deletion path.
	Signals RetentionSignals
	// Rules and Sweeper are the rule-driven half of the engine (E-07). Optional together: an
	// installation wired without them sweeps the trash and the notification history exactly as
	// before, which is what makes the two halves independently deployable.
	Rules   repository.Rules
	Sweeper Sweeper
}

// RetentionSignals is the slice of the metrics adapter a run reports through
// (data-retention.md §5, observability-reliability.md §3.2).
//
// The labels are the closed sets this package defines - the data kind and the block reasons - so
// there is no way for an unbounded label to reach a metric from here.
type RetentionSignals interface {
	RetentionDeleted(ctx context.Context, dataKind string, count int64)
	RetentionBlocked(ctx context.Context, reason string, count int64)
	RetentionRun(ctx context.Context, dataKind string, seconds float64)
}

// NotificationHistory is the slice of the notification repository this run removes through.
//
// Declared here rather than imported, so that what the retention engine can do to notifications is
// visible in one place: it can count what is due and remove a batch of it, and it cannot read one,
// write one or send one.
type NotificationHistory interface {
	DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error)
	CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error)
}

// DispatchedEvents is the slice of the outbox this run removes through (G-02, ADR-0007's second
// countermeasure). The same two methods as the notification history, and deliberately the same
// shape: the engine treats a third kind exactly as it treats the second.
//
// What the interface does not offer is a way to remove an *undispatched* event. That guard lives
// in the query rather than in a parameter, because it is not a policy an engine could get wrong
// once and a tenant could configure away - a row nobody has consumed is never due.
type DispatchedEvents interface {
	DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error)
	CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error)
}

// Execute runs one pass for the tenant the transaction is bound to, and reports what it did.
//
// The whole of it inside the caller's transaction, which is the queue's contract: what the pass
// removed, the records it left and the log entry saying it happened commit together or not at all.
// A pass whose log was written separately could say it removed nothing while the rows were gone.
func (h RunRetention) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (Outcome, error) {
	started := h.Clock.Now()

	// The defaults for a tenant that has none. Here rather than in a migration: a migration covers
	// the tenants that existed when it ran, and this covers the one in front of it (ADR-0020).
	if err := h.Policies.Ensure(ctx, domain.DefaultPolicies()); err != nil {
		return Outcome{}, err
	}
	policy, err := h.Policies.Find(ctx, domain.KindTrash)
	if err != nil {
		return Outcome{}, err
	}

	runID := h.IDs.NewID()
	if err := h.Runs.Start(ctx, runID, domain.KindTrash, started); err != nil {
		return Outcome{}, err
	}

	outcome, sweepErr := h.Purger.Sweep(ctx, actor, Selection{
		// One reading of the clock decides the cutoff for the whole pass. Two would let a long batch
		// use two definitions of "expired", which is the kind of difference that shows up as one row
		// surviving a run for no reason anybody can reconstruct.
		Cutoff: policy.Cutoff(started),
		Reason: domain.DeletedByRetention,
		// The automatic path, so the offline window is a floor: an object may only disappear for good
		// once every known device has had the chance to learn of the deletion, and nobody asked for
		// this to happen today (offline-sync.md §7).
		ObserveTombstoneWindow: true,
	}, started)

	// The log is closed either way. A failed pass that left no trace would be indistinguishable from
	// one that never ran, which is the state this log exists to make visible.
	finished := h.Clock.Now()
	status := repository.RunSucceeded
	if sweepErr != nil {
		status = repository.RunFailed
	}
	if err := h.Runs.Finish(ctx, runID, repository.RunResult{
		Matched: outcome.Matched, Removed: outcome.Removed, Blocked: outcome.Blocked,
		Status: status, FinishedAt: finished,
	}); err != nil {
		return outcome, err
	}
	if sweepErr != nil {
		return outcome, sweepErr
	}

	h.report(ctx, domain.KindTrash, outcome, finished.Sub(started))

	// The trail gets one entry per pass rather than one per row: an audit that grew with every
	// deleted object would grow faster than the payload data it is about (data-retention.md §5).
	if outcome.Removed > 0 || len(outcome.Blocked) > 0 {
		if err := h.Purger.RecordAudit(ctx, actor, RetentionRunAction, trashTarget, runID,
			outcome, domain.DeletedByRetention, finished); err != nil {
			return outcome, err
		}
	}

	history, err := h.sweepHistory(ctx, started)
	if err != nil {
		return outcome, err
	}

	events, err := h.sweepEvents(ctx, started)
	if err != nil {
		return outcome, err
	}

	rules, err := h.sweepRules(ctx, actor, started)
	if err != nil {
		return outcome, err
	}

	// Reported as one outcome, so that the job's decision about coming back straight away covers
	// every kind: a pass that emptied the trash and left a full batch of notifications has not
	// finished, and a job that stopped there would leave them until the next long interval.
	outcome.Matched += history.Matched
	outcome.Removed += history.Removed
	outcome.Matched += events.Matched
	outcome.Removed += events.Removed
	outcome.add(rules)
	return outcome, nil
}

// sweepEvents removes one batch of dispatched outbox rows (G-02, data-retention.md §3).
//
// The table the outbox pattern leaves behind: an event's job is done the moment every consumer has
// had it, and until ADR-0007's second countermeasure existed nothing ever removed the row. Seven
// days by default, the shortest period in the catalogue, because this is a debugging aid rather
// than a record - the audit trail is the record.
//
// No tombstone window, no legal hold and no audit entry, on sweepHistory's reasoning exactly: an
// event is not an object a device holds, a hold is placed on tenants, containers and items, and an
// entry per pass per tenant per hour would bury the entries that matter.
//
// A missing wiring is skipped rather than refused, which is the one place this differs from the
// notification history - and the difference is which risk each carries. A notification history
// that silently stops being swept is personal data kept past its period (risk R-09); an outbox
// that silently stops being swept is a table that grows, which the backlog alert already reports.
func (h RunRetention) sweepEvents(ctx context.Context, started time.Time) (Outcome, error) {
	if h.Events == nil {
		return Outcome{}, nil
	}

	policy, err := h.Policies.Find(ctx, domain.KindOutboxEvent)
	if err != nil {
		return Outcome{}, err
	}

	runID := h.IDs.NewID()
	if err := h.Runs.Start(ctx, runID, domain.KindOutboxEvent, started); err != nil {
		return Outcome{}, err
	}

	cutoff := policy.Cutoff(started)
	matched, err := h.Events.CountExpired(ctx, cutoff, h.Purger.BatchSize)
	if err != nil {
		return Outcome{}, err
	}
	removed, sweepErr := h.Events.DeleteExpired(ctx, cutoff, h.Purger.BatchSize)

	finished := h.Clock.Now()
	status := repository.RunSucceeded
	if sweepErr != nil {
		status = repository.RunFailed
	}
	outcome := Outcome{Matched: matched, Removed: removed}
	if err := h.Runs.Finish(ctx, runID, repository.RunResult{
		Matched: outcome.Matched, Removed: outcome.Removed,
		Status: status, FinishedAt: finished,
	}); err != nil {
		return outcome, err
	}
	if sweepErr != nil {
		return outcome, sweepErr
	}

	h.report(ctx, domain.KindOutboxEvent, outcome, finished.Sub(started))
	return outcome, nil
}

// sweepHistory removes one batch of expired notification records (C-09, data-retention.md §3).
//
// No tombstone window and no legal hold, and neither is an omission. A notification is not an
// object a device holds and could recreate - offline-sync.md §4 defines no merge rule for one,
// because nothing syncs them - and a hold is placed on tenants, containers and items, which is the
// scope §4.1 names. What governs this history is its period and nothing else.
func (h RunRetention) sweepHistory(ctx context.Context, started time.Time) (Outcome, error) {
	if h.History == nil {
		// Refused rather than skipped. A retention engine that quietly sweeps one kind fewer than
		// it is configured for is exactly the overlooked derived data of risk R-09
		// (data-protection.md §5) - and it would look like a working installation for ninety days
		// before anybody could notice.
		return Outcome{}, shared.ErrInternal.WithDetail("lifecycle.history_not_wired")
	}

	policy, err := h.Policies.Find(ctx, domain.KindNotification)
	if err != nil {
		return Outcome{}, err
	}

	runID := h.IDs.NewID()
	if err := h.Runs.Start(ctx, runID, domain.KindNotification, started); err != nil {
		return Outcome{}, err
	}

	cutoff := policy.Cutoff(started)
	// Counted no higher than the batch, because what the caller needs is "is there more after
	// this" and not a count of the table. Matched is what decides whether the job comes back
	// straight away, so it has to be the number of rows that were due rather than the number that
	// went - the same reading Exhausted takes of the trash.
	matched, err := h.History.CountExpired(ctx, cutoff, h.Purger.BatchSize)
	if err != nil {
		return Outcome{}, err
	}
	removed, sweepErr := h.History.DeleteExpired(ctx, cutoff, h.Purger.BatchSize)

	finished := h.Clock.Now()
	status := repository.RunSucceeded
	if sweepErr != nil {
		status = repository.RunFailed
	}
	outcome := Outcome{Matched: matched, Removed: removed}
	if err := h.Runs.Finish(ctx, runID, repository.RunResult{
		Matched: outcome.Matched, Removed: outcome.Removed,
		Status: status, FinishedAt: finished,
	}); err != nil {
		return outcome, err
	}
	if sweepErr != nil {
		return outcome, sweepErr
	}

	h.report(ctx, domain.KindNotification, outcome, finished.Sub(started))
	// No audit entry. The trash's is there because a removal of somebody's work is a decision an
	// auditor looks for (audit.md §2); the expiry of the record that somebody was emailed ninety
	// days ago is the machinery doing exactly what the period says, and an entry per pass per
	// tenant per hour would bury the entries that matter.
	return outcome, nil
}

// sweepRules is the rule-driven half: the kinds a tenant configures rather than the two the
// installation always sweeps (E-07, data-retention.md §2, §5).
//
// The carry-over comes first and runs on every pass, which is what makes an upgrade need no
// migration over tenant data: a tenant whose period lives in the old table gets a tenant-wide rule
// written for it the first time the engine reaches it, and one that has since written a rule of its
// own is left alone.
func (h RunRetention) sweepRules(
	ctx context.Context, actor appshared.ActorContext, started time.Time,
) (Outcome, error) {
	if h.Rules == nil {
		return Outcome{}, nil
	}
	for _, policy := range domain.DefaultPolicies() {
		if err := h.Rules.CarryOver(ctx, h.IDs.NewID(), policy.DataKind, started); err != nil {
			return Outcome{}, err
		}
	}

	runID := h.IDs.NewID()
	if err := h.Runs.Start(ctx, runID, domain.KindCompletedItem, started); err != nil {
		return Outcome{}, err
	}

	outcome, sweepErr := h.Sweeper.Pass(ctx, actor)

	finished := h.Clock.Now()
	status := repository.RunSucceeded
	if sweepErr != nil {
		status = repository.RunFailed
	}
	if err := h.Runs.Finish(ctx, runID, repository.RunResult{
		Matched: outcome.Matched, Removed: outcome.Removed, Blocked: outcome.Blocked,
		Status: status, FinishedAt: finished,
	}); err != nil {
		return outcome, err
	}
	if sweepErr != nil {
		return outcome, sweepErr
	}

	h.report(ctx, domain.KindCompletedItem, outcome, finished.Sub(started))
	return outcome, nil
}

// report publishes what the pass did.
//
// Zero is published too, and on purpose: a counter that has never been written has no series, and an
// alert on a deletion run that never happens is an alert that reads "no data" and is believed
// (observability-reliability.md §4).
func (h RunRetention) report(
	ctx context.Context, dataKind domain.DataKind, outcome Outcome, took time.Duration,
) {
	if h.Signals == nil {
		return
	}
	kind := string(dataKind)

	h.Signals.RetentionRun(ctx, kind, took.Seconds())
	h.Signals.RetentionDeleted(ctx, kind, int64(outcome.Removed))

	// The reasons this kind can be blocked by, from the catalogue rather than from a list here
	// (E-07). A kind nothing can block reports no series at all - a zero that can never be anything
	// else is a line on a dashboard that means nothing - and a kind that gains a reason gains its
	// series without anybody remembering to add it.
	entry, known := domain.FindKind(dataKind)
	if !known {
		return
	}
	for _, reason := range entry.Blockable {
		h.Signals.RetentionBlocked(ctx, reason, int64(outcome.Blocked[reason]))
	}
}

// Exhausted reports whether this pass found less than a full batch, which is how the job knows there
// is nothing left to come back for straight away.
//
// Read off what was matched rather than off what was removed: a pass that matched a full batch and
// removed none of it - every one of them under a legal hold - has still not reached the end of the
// trash, and a job that stopped there would leave the rest until the next long interval.
func (h RunRetention) Exhausted(outcome Outcome) bool {
	return outcome.Matched < h.Purger.BatchSize
}

// RetentionRunAction is the audit code of a run that removed something.
const RetentionRunAction audit.Action = "retention.executed"
