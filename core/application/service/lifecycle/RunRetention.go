// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
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
	Clock    clock.Clock
	IDs      clock.IDGenerator
	// Signals is the observability slice. Optional: a run without it still runs, which is what keeps
	// a metrics adapter from being a dependency of the deletion path.
	Signals RetentionSignals
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

	h.report(ctx, outcome, finished.Sub(started))

	// The trail gets one entry per pass rather than one per row: an audit that grew with every
	// deleted object would grow faster than the payload data it is about (data-retention.md §5).
	if outcome.Removed > 0 || len(outcome.Blocked) > 0 {
		if err := h.Purger.RecordAudit(ctx, actor, RetentionRunAction, trashTarget, runID,
			outcome, domain.DeletedByRetention, finished); err != nil {
			return outcome, err
		}
	}
	return outcome, nil
}

// report publishes what the pass did.
//
// Zero is published too, and on purpose: a counter that has never been written has no series, and an
// alert on a deletion run that never happens is an alert that reads "no data" and is believed
// (observability-reliability.md §4).
func (h RunRetention) report(ctx context.Context, outcome Outcome, took time.Duration) {
	if h.Signals == nil {
		return
	}
	kind := string(domain.KindTrash)

	h.Signals.RetentionRun(ctx, kind, took.Seconds())
	h.Signals.RetentionDeleted(ctx, kind, int64(outcome.Removed))
	for _, reason := range []string{BlockedByLegalHold, BlockedByTombstoneWindow} {
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
