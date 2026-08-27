// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// removals is the deletion journal and the tombstone, as one port writes them.
type removals struct {
	recorded   []lifecycle.Removal
	deletedAt  time.Time
	purgeAfter time.Time
	err        error
}

func (r *removals) Record(
	_ context.Context, entries []lifecycle.Removal, deletedAt, purgeAfter time.Time,
) error {
	if r.err != nil {
		return r.err
	}
	r.recorded = append(r.recorded, entries...)
	r.deletedAt, r.purgeAfter = deletedAt, purgeAfter
	return nil
}

// signals records what a pass published.
type signals struct{ reclaimed, failed int64 }

func (s *signals) MediaReclaimed(_ context.Context, count int64)     { s.reclaimed += count }
func (s *signals) MediaReclaimFailed(_ context.Context, count int64) { s.failed += count }

// reconcilingObjects extends the shared fake with what a pass asks of it: the orphan queue, and a
// record of the calls that make the counters honest.
type reconcilingObjects struct {
	*objects
	orphans []repository.Orphan
	// recounted and the cuts record that the two halves of the plan ran, and against which
	// instants: the graces are the whole of what this pass decides, so a test that did not read
	// them back would be checking that the calls happened rather than what they asked for.
	recounted       int
	recountedAt     time.Time
	pendingCut      time.Time
	unreferencedCut time.Time
	markedBefore    time.Time
	takenBatch      int
	removedRows     []shared.ID
	removeErr       error
}

func newReconcilingObjects() *reconcilingObjects {
	return &reconcilingObjects{objects: newObjects()}
}

func (o *reconcilingObjects) Recount(_ context.Context, now time.Time) error {
	o.recounted++
	o.recountedAt = now
	return nil
}

func (o *reconcilingObjects) MarkOrphans(
	_ context.Context, _ time.Time, before repository.Thresholds,
) (int, error) {
	o.pendingCut, o.unreferencedCut = before.Pending, before.Unreferenced
	return len(o.orphans), nil
}

func (o *reconcilingObjects) TakeOrphans(
	_ context.Context, markedBefore time.Time, batch int,
) ([]repository.Orphan, error) {
	o.markedBefore, o.takenBatch = markedBefore, batch
	if len(o.orphans) > batch {
		return o.orphans[:batch], nil
	}
	return o.orphans, nil
}

func (o *reconcilingObjects) RemoveRows(_ context.Context, ids []shared.ID) (int, error) {
	if o.removeErr != nil {
		return 0, o.removeErr
	}
	o.removedRows = append(o.removedRows, ids...)
	return len(ids), nil
}

// unreachableStore refuses to let go of the bytes it is asked about.
type unreachableStore struct{ *store }

func (s unreachableStore) Delete(context.Context, string) error {
	return shared.ErrUnavailable.WithDetail("dependency.object_storage_unavailable")
}

func reconcilingHarness() (ReconcileMedia, *reconcilingObjects, *store, *removals, *signals) {
	records, bytes, journal, published := newReconcilingObjects(), newStore(), &removals{}, &signals{}
	return ReconcileMedia{
		Objects:    records,
		Store:      bytes,
		Removals:   journal,
		UnitOfWork: &unitOfWork{},
		Clock:      clock.Fixed(now),
		Config: env.MediaConfig{
			StagingGrace: 24 * time.Hour, UnreferencedGrace: 30 * time.Minute,
			OrphanGrace: time.Hour, BatchSize: 2,
		},
		Retention: env.RetentionConfig{TombstoneWindow: 90 * 24 * time.Hour},
		Signals:   published,
	}, records, bytes, journal, published
}

// systemActor is what the queue hands in: a tenant, and no account.
func systemActor() appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenantID}
}

func orphan(id shared.ID) repository.Orphan {
	return repository.Orphan{ID: id, StorageKey: "media/" + tenantID.String() + "/" + id.String()}
}

func TestAPassReclaimsTheBytesTheRowAndWritesTheJournal(t *testing.T) {
	handler, records, bytes, journal, published := reconcilingHarness()
	records.orphans = []repository.Orphan{orphan(mintedID)}
	bytes.content[records.orphans[0].StorageKey] = []byte("gone soon")

	outcome, err := handler.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	if outcome.Reclaimed != 1 || outcome.Failed != 0 {
		t.Fatalf("the outcome is %+v", outcome)
	}
	if records.recounted != 1 {
		t.Errorf("the counters were made honest %d times, want 1", records.recounted)
	}
	if _, left := bytes.content[records.orphans[0].StorageKey]; left {
		t.Error("the bytes are still in storage")
	}
	if len(records.removedRows) != 1 || records.removedRows[0] != mintedID {
		t.Errorf("the rows removed are %v", records.removedRows)
	}
	// The journal is what stops a restore from backup bringing the file back (ADR-0020 §6), and
	// the reason distinguishes the machinery from a person.
	if len(journal.recorded) != 1 {
		t.Fatalf("the journal is %+v", journal.recorded)
	}
	entry := journal.recorded[0]
	if entry.Entity != mediaEntity || entry.EntityID != mintedID {
		t.Errorf("the journal entry is %+v", entry)
	}
	if entry.Reason != lifecycle.DeletedByRetention {
		t.Errorf("the reason recorded is %s", entry.Reason)
	}
	// The tombstone outlives the removal by the maximum offline window, so a device that was away
	// cannot push the row back into existence (offline-sync.md §7).
	if want := now.Add(90 * 24 * time.Hour); journal.purgeAfter != want {
		t.Errorf("the tombstone runs to %v, want %v", journal.purgeAfter, want)
	}
	if published.reclaimed != 1 {
		t.Errorf("%d reclamations published, want 1", published.reclaimed)
	}
}

func TestThePlanUsesTheConfiguredGraces(t *testing.T) {
	handler, records, _, _, _ := reconcilingHarness()

	if _, err := handler.Execute(t.Context(), systemActor()); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	// A staging is abandoned only after its grace, so an upload still travelling up a slow line is
	// not mistaken for one.
	if want := now.Add(-24 * time.Hour); records.pendingCut != want {
		t.Errorf("stagings before %v are reclaimed, want %v", records.pendingCut, want)
	}
	// A confirmed object is an orphan only once nothing has pointed at it for its own grace. The
	// window between a confirmation and the first thing that uses the object is not evidence of
	// anything, and marking is where the loss begins: nothing can attach a marked object back.
	if want := now.Add(-30 * time.Minute); records.unreferencedCut != want {
		t.Errorf("objects unreferenced before %v are marked, want %v", records.unreferencedCut, want)
	}
	// The stamp the sweep marks against is written by the recount in the same transaction, and at
	// the same instant the marking is decided at.
	if records.recountedAt != now {
		t.Errorf("the recount stamped at %v, want %v", records.recountedAt, now)
	}
	// A marked object waits out its grace, which is the window in which an operator who notices a
	// mistaken removal still finds the bytes where they were.
	if want := now.Add(-time.Hour); records.markedBefore != want {
		t.Errorf("orphans marked before %v are taken, want %v", records.markedBefore, want)
	}
	if records.takenBatch != 2 {
		t.Errorf("the batch is %d, want 2", records.takenBatch)
	}
}

// The defaults exist because a build without configuration must not divide the world into
// zero-length graces: an unreferenced grace of nothing is exactly the defect that made a confirmed
// upload disappear between its confirmation and its attachment.
func TestAnUnconfiguredPassStillGivesAConfirmedObjectItsGrace(t *testing.T) {
	handler, records, _, _, _ := reconcilingHarness()
	handler.Config = env.MediaConfig{}

	if _, err := handler.Execute(t.Context(), systemActor()); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	if want := now.Add(-time.Hour); records.unreferencedCut != want {
		t.Errorf("objects unreferenced before %v are marked, want %v", records.unreferencedCut, want)
	}
	if want := now.Add(-24 * time.Hour); records.pendingCut != want {
		t.Errorf("stagings before %v are reclaimed, want %v", records.pendingCut, want)
	}
}

// The order that matters: the row only goes once its bytes have. The other way round leaves a file
// in the bucket that nothing in this system knows about any more.
func TestAnObjectWhoseBytesWillNotGoKeepsItsRow(t *testing.T) {
	handler, records, bytes, journal, published := reconcilingHarness()
	handler.Store = unreachableStore{bytes}
	records.orphans = []repository.Orphan{orphan(mintedID)}

	outcome, err := handler.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	// A number rather than an error: the object stays marked and the next pass tries again, which
	// is what makes a briefly unreachable bucket a delay rather than a stuck reclamation.
	if outcome.Failed != 1 || outcome.Reclaimed != 0 {
		t.Fatalf("the outcome is %+v", outcome)
	}
	if len(records.removedRows) != 0 {
		t.Error("a row went while its bytes stayed")
	}
	if len(journal.recorded) != 0 {
		t.Error("a removal was journalled that did not happen")
	}
	// Published all the same: a number that keeps rising is a bucket that is not letting go, and
	// that is exactly the case an operator needs to see.
	if published.failed != 1 {
		t.Errorf("%d failures published, want 1", published.failed)
	}
}

func TestAPassWithNothingToDoTouchesNothing(t *testing.T) {
	handler, records, _, journal, _ := reconcilingHarness()

	outcome, err := handler.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	if outcome.Reclaimed != 0 || len(journal.recorded) != 0 || len(records.removedRows) != 0 {
		t.Errorf("an empty pass did something: %+v", outcome)
	}
	// The counters are still made honest. That is the half of the pass that has to run whether or
	// not anything is due: a drifted count is what makes an object invisible to the sweep.
	if records.recounted != 1 {
		t.Errorf("the counters were made honest %d times, want 1", records.recounted)
	}
}

func TestAFullBatchMeansThePassIsNotExhausted(t *testing.T) {
	handler, records, bytes, _, _ := reconcilingHarness()
	records.orphans = []repository.Orphan{orphan(mintedID), orphan(strangerA), orphan(accountID)}
	for _, o := range records.orphans {
		bytes.content[o.StorageKey] = []byte("gone soon")
	}

	outcome, err := handler.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	if outcome.Reclaimed != 2 {
		t.Fatalf("the pass reclaimed %d, want the batch of 2", outcome.Reclaimed)
	}
	// Known work left, so the job comes back at once rather than at the interval.
	if handler.Exhausted(outcome) {
		t.Error("a pass that filled its batch reports itself exhausted")
	}
}

func TestAJournalThatCannotBeWrittenLeavesTheRows(t *testing.T) {
	handler, records, bytes, journal, _ := reconcilingHarness()
	records.orphans = []repository.Orphan{orphan(mintedID)}
	bytes.content[records.orphans[0].StorageKey] = []byte("gone soon")
	journal.err = shared.ErrUnavailable.WithDetail("postgres.query_failed")

	_, err := handler.Execute(t.Context(), systemActor())
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("error %v, want the dependency failure", err)
	}
	// The bytes are gone and the row is not. The next pass takes the same row, finds no bytes -
	// which storage reports as success - and finishes the job. That is why deleting what is not
	// there is not an error.
	if len(records.removedRows) != 0 {
		t.Error("the row went without its journal entry")
	}
}
