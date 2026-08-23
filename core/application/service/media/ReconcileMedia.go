// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"log/slog"
	"time"

	lifecyclerepo "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

// mediaEntity is what the deletion journal and the tombstone call a media record. The table's own
// name, as the journal's other entries use theirs (data-retention.md §3).
const mediaEntity = "media_object"

// ReconcileMedia is the deletion path data-protection.md §5 promises, made real: reference counting
// that is checked rather than trusted, and the removal of what nothing points at.
//
// Internal, and deliberately not in the use case catalogue - for the reason RunRetention is not.
// The catalogue is the list of things a person, an agent or a rule can ask for; this is what the
// installation does on its own, and "delete every unreferenced file, now" is not a button anybody
// should be given.
//
// One pass per call, in three parts that are three parts on purpose:
//
//  1. Recount, and mark what nothing points at. Both in one transaction, because a count read in
//     one and acted on in another is a count something can move in between - and the whole reason
//     the recount exists is that the incremental counter can drift.
//  2. Delete the bytes, outside any transaction. A bucket is an external dependency, and a
//     transaction waiting on one holds a database connection for as long as somebody else's server
//     feels like taking (observability-reliability.md §8).
//  3. Journal the removals and drop the rows, in one transaction. The journal entry is written
//     first and in the same transaction as the row it is about, so a restore from backup can never
//     bring back a file this installation decided was gone (ADR-0020 §6).
//
// Safe to run twice, which is what lets it run outside the runner's transaction (queue.Detached):
// a recount is idempotent, marking what is already marked changes nothing, deleting bytes that are
// not there succeeds, and both the journal and the row removal are written on the row's identity.
type ReconcileMedia struct {
	Objects    repository.Objects
	Store      storage.ObjectStore
	Removals   lifecyclerepo.Removals
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	Config     env.MediaConfig
	// Retention is where the tombstone window comes from: how long the marker of a removal has to
	// outlive it, which is the maximum offline window (offline-sync.md §7).
	Retention env.RetentionConfig
	// Signals is the observability slice. Optional: a pass without it still runs, which is what
	// keeps a metrics adapter from being a dependency of the deletion path.
	Signals ReconciliationSignals
}

// ReconciliationSignals is the slice of the metrics adapter a pass reports through.
//
// The labels are this package's closed set - the outcome of a pass, never an identifier - so there
// is no way for an unbounded label to reach a metric from here
// (observability-reliability.md §3.2).
type ReconciliationSignals interface {
	MediaReclaimed(ctx context.Context, count int64)
	MediaReclaimFailed(ctx context.Context, count int64)
}

// Outcome is what one pass did.
type Outcome struct {
	// Marked is how many objects this pass decided nothing points at any more.
	Marked int
	// Reclaimed is how many rows went for good, bytes and all.
	Reclaimed int
	// Failed is how many orphans kept their bytes because storage would not let go. They stay
	// marked, and the next pass tries again - which is why a failure here is a number rather than
	// an error.
	Failed int
}

// Execute runs one pass for the tenant the actor names.
func (h ReconcileMedia) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (Outcome, error) {
	now := h.Clock.Now()

	outcome, orphans, err := h.plan(ctx, actor, now)
	if err != nil {
		return Outcome{}, err
	}

	// Outside any transaction. The pass is at its slowest here - one call to a bucket per object -
	// and this is exactly the stretch that must not be holding a connection.
	removed, failed := h.discard(ctx, orphans)
	outcome.Failed = failed

	if len(removed) > 0 {
		if err := h.forget(ctx, actor, removed, now); err != nil {
			// The bytes are gone and the rows are not. The next pass takes the same rows, finds no
			// bytes - which storage reports as success - and finishes the job. That is the whole
			// reason deleting what is not there is not an error (core/port/storage).
			return outcome, err
		}
		outcome.Reclaimed = len(removed)
	}

	// Published on every pass that got this far, zero included: a counter that has never been
	// written has no series, and an alert on a reclamation that never happens is one that reads
	// "no data" and is believed (observability-reliability.md §4).
	h.report(ctx, outcome)
	return outcome, nil
}

// Exhausted reports whether the pass reached the end of what there was to do. A pass that filled
// its batch has known work left, and the job comes back at once rather than at the interval.
func (h ReconcileMedia) Exhausted(outcome Outcome) bool {
	return outcome.Reclaimed+outcome.Failed < h.batchSize()
}

// plan makes the counters honest, marks what nothing points at, and reads the batch to reclaim -
// all in one transaction.
//
// One transaction for all three, because they are one decision. A count read in one transaction and
// acted on in another is a count something can move in between, and the recount exists precisely
// because the incremental counter can be wrong - reading it back outside the transaction that
// corrected it would be trusting the number this step was written to distrust.
func (h ReconcileMedia) plan(
	ctx context.Context, actor appshared.ActorContext, now time.Time,
) (Outcome, []repository.Orphan, error) {
	var (
		outcome Outcome
		orphans []repository.Orphan
	)

	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Objects.Recount(ctx); err != nil {
			return err
		}

		marked, err := h.Objects.MarkOrphans(ctx, now, now.Add(-h.stagingGrace()))
		if err != nil {
			return err
		}
		outcome.Marked = marked

		// Marked before the grace ended, which is the window in which a mistake is still a
		// mistake: an object that lost its last reference and gained a new one inside it is
		// recounted and unmarked by the pass above rather than removed by this one.
		orphans, err = h.Objects.TakeOrphans(ctx, now.Add(-h.orphanGrace()), h.batchSize())
		return err
	})
	if err != nil {
		return Outcome{}, nil, err
	}
	return outcome, orphans, nil
}

// discard removes the bytes and reports which rows may now follow them.
//
// An object whose bytes would not go keeps its row. The order is deliberate and it is the only
// order that is safe: a row removed before its bytes leaves a file in the bucket that nothing in
// this system knows about any more, and nothing would ever come back for it.
func (h ReconcileMedia) discard(
	ctx context.Context, orphans []repository.Orphan,
) ([]shared.ID, int) {
	removed := make([]shared.ID, 0, len(orphans))
	failed := 0

	for _, orphan := range orphans {
		if err := h.Store.Delete(ctx, orphan.StorageKey); err != nil {
			// Not the pass's error: the object stays marked and the next pass tries again, which
			// is what makes a bucket that is briefly unreachable a delay rather than a stuck
			// reclamation. The key is not logged - it carries the tenant.
			slog.WarnContext(ctx, "reclaiming the bytes of an orphaned media object failed",
				slog.String("media_id", orphan.ID.String()),
				slog.String("error", err.Error()))
			failed++
			continue
		}
		removed = append(removed, orphan.ID)
	}
	return removed, failed
}

// forget writes the records of the removal and drops the rows, in one transaction.
//
// The journal first and the tombstone with it, both before the rows go and all three in the same
// transaction: a journal entry without the removal is a file this installation would refuse to
// restore and still holds, and a removal without the journal entry is a file a restore from backup
// brings back (ADR-0020 §6, backup-restore.md §6).
func (h ReconcileMedia) forget(
	ctx context.Context, actor appshared.ActorContext, ids []shared.ID, now time.Time,
) error {
	removals := make([]lifecycle.Removal, 0, len(ids))
	for _, id := range ids {
		removals = append(removals, lifecycle.Removal{
			Entity: mediaEntity, EntityID: id,
			// The installation removing what nothing points at any more, on its own schedule.
			// Not USER: nobody asked for this object to go today, and an auditor reading the
			// journal has to be able to tell a person's deletion from the machinery's.
			Reason: lifecycle.DeletedByRetention,
		})
	}

	return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Removals.Record(ctx, removals, now, now.Add(h.Retention.TombstoneWindow)); err != nil {
			return err
		}
		_, err := h.Objects.RemoveRows(ctx, ids)
		return err
	})
}

// report publishes what the pass did.
func (h ReconcileMedia) report(ctx context.Context, outcome Outcome) {
	if h.Signals == nil {
		return
	}
	h.Signals.MediaReclaimed(ctx, int64(outcome.Reclaimed))
	h.Signals.MediaReclaimFailed(ctx, int64(outcome.Failed))
}

// The defaults a build without configuration falls back on, so that a pass never divides the world
// into zero-sized batches or reclaims a staging the moment it is written.
func (h ReconcileMedia) batchSize() int {
	if h.Config.BatchSize < 1 {
		return 100
	}
	return h.Config.BatchSize
}

func (h ReconcileMedia) stagingGrace() time.Duration {
	if h.Config.StagingGrace <= 0 {
		return 24 * time.Hour
	}
	return h.Config.StagingGrace
}

func (h ReconcileMedia) orphanGrace() time.Duration {
	if h.Config.OrphanGrace <= 0 {
		return time.Hour
	}
	return h.Config.OrphanGrace
}
