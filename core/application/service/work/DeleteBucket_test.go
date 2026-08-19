// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

func newDeleteHarness() (*bucketHarness, DeleteBucket) {
	base := newBucketHarness()
	base.withCollection()
	return base, DeleteBucket{Writer: BucketWriter{
		Buckets: base.buckets, Containers: base.containers, Authorizer: base.authorizer,
		Events: base.events, Changes: base.changes, Audit: base.audit, UnitOfWork: base.uow,
		Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}}
}

// The acceptance criterion of B-09: deleting a column moves its entries to the one that is left
// rather than orphaning them. Which one is the leftmost remaining column, derived rather than read
// from a stored default that nothing keeps up to date.
func TestDeletingAColumnMovesItsEntriesToTheLeftmostRemainingOne(t *testing.T) {
	h, deleter := newDeleteHarness()
	first := h.withBucket(shared.MustParseID("0192f000-0000-7000-8000-0000000000f0"), "Todo", "a0")
	h.withBucket(doingBucket, "Doing", "a1")
	ctx := correlation.ContextWithRequestID(context.Background(), "01J9REQUEST")

	deletion, err := deleter.Execute(ctx, actor(), DeleteBucketCommand{BucketID: doingBucket})
	if err != nil {
		t.Fatalf("deleting the column failed: %v", err)
	}

	if deletion.BucketID != doingBucket || deletion.TargetBucketID != first.ID {
		t.Fatalf("unexpected deletion: %+v", deletion)
	}
	if len(h.buckets.moved) != 1 {
		t.Fatalf("%d moves, want 1", len(h.buckets.moved))
	}
	if move := h.buckets.moved[0]; move.source != doingBucket || move.target != first.ID {
		t.Errorf("the entries went from %s to %s", move.source, move.target)
	}
	if stored := h.buckets.stored[doingBucket]; !stored.IsDeleted() {
		t.Error("the column is still on the board")
	}

	t.Run("the event says where the entries went", func(t *testing.T) {
		if len(h.events.appended) != 1 {
			t.Fatalf("%d events, want 1", len(h.events.appended))
		}
		announcement := h.events.appended[0]
		if announcement.Type != event.BucketDeleted {
			t.Errorf("event type %s", announcement.Type)
		}
		if announcement.Payload["target_bucket_id"] != first.ID.String() {
			t.Errorf("the event names %v as the destination", announcement.Payload["target_bucket_id"])
		}
		if announcement.Payload["moved_items"] == nil {
			t.Error("the event does not say how many entries moved")
		}
	})

	// A deletion rather than an upsert, and it carries no payload: there is nothing left to
	// describe, and a tombstone with content would be a copy of the deleted object in the log.
	t.Run("the change log entry is a deletion", func(t *testing.T) {
		if len(h.changes.recorded) != 1 {
			t.Fatalf("%d changes, want 1", len(h.changes.recorded))
		}
		change := h.changes.recorded[0]
		if change.Op != changelog.Delete {
			t.Errorf("the operation is %s", change.Op)
		}
		if change.Payload != nil {
			t.Errorf("the deletion carries a payload: %+v", change.Payload)
		}
		if change.ContainerID != hubID {
			t.Errorf("the change is filed under %s, want the hub", change.ContainerID)
		}
	})

	// "How many entries did this move" is precisely what an auditor asking about a deletion needs,
	// and there is nothing personal in a count or in an identifier this installation produced.
	t.Run("the audit entry says what it cost", func(t *testing.T) {
		if len(h.audit.entries) != 1 {
			t.Fatalf("%d audit entries, want 1", len(h.audit.entries))
		}
		entry := h.audit.entries[0]
		if entry.Action != BucketDeletedAction || entry.TargetID != doingBucket {
			t.Errorf("unexpected entry: %+v", entry)
		}
		if entry.Changes["moved_items"] == nil || entry.Changes["target_bucket_id"] == nil {
			t.Errorf("the trail does not say what the deletion cost: %+v", entry.Changes)
		}
	})
}

// The last column of a board has nowhere to send its entries, and that is not a failure: they then
// carry none, which is the state the collection was in before anybody made a board.
func TestDeletingTheLastColumnLeavesTheEntriesWithoutOne(t *testing.T) {
	h, deleter := newDeleteHarness()
	h.withBucket(doingBucket, "Doing", "a0")

	deletion, err := deleter.Execute(context.Background(), actor(), DeleteBucketCommand{
		BucketID: doingBucket,
	})
	if err != nil {
		t.Fatalf("deleting the last column failed: %v", err)
	}

	if !deletion.TargetBucketID.IsZero() {
		t.Errorf("the entries went to %s, want no column at all", deletion.TargetBucketID)
	}
	if len(h.buckets.moved) != 1 || !h.buckets.moved[0].target.IsZero() {
		t.Fatalf("unexpected move: %+v", h.buckets.moved)
	}
	if h.events.appended[0].Payload["target_bucket_id"] != nil {
		t.Errorf("the event names a destination: %v", h.events.appended[0].Payload["target_bucket_id"])
	}
}

// Deleting a column that is already gone succeeds and moves nothing. Moving its entries a second
// time would take them off whatever column somebody has since put them on.
func TestDeletingAColumnTwiceMovesNothing(t *testing.T) {
	h, deleter := newDeleteHarness()
	h.withBucket(shared.MustParseID("0192f000-0000-7000-8000-0000000000f0"), "Todo", "a0")
	h.withBucket(doingBucket, "Doing", "a1")

	if _, err := deleter.Execute(context.Background(), actor(), DeleteBucketCommand{
		BucketID: doingBucket,
	}); err != nil {
		t.Fatalf("the first deletion failed: %v", err)
	}

	deletion, err := deleter.Execute(context.Background(), actor(), DeleteBucketCommand{
		BucketID: doingBucket,
	})
	if err != nil {
		t.Fatalf("the second deletion failed: %v", err)
	}

	if len(h.buckets.moved) != 1 {
		t.Errorf("%d moves, want the first one only", len(h.buckets.moved))
	}
	if len(h.events.appended) != 1 || len(h.audit.entries) != 1 {
		t.Error("the second deletion announced something")
	}
	if !deletion.TargetBucketID.IsZero() || deletion.MovedItems != 0 {
		t.Errorf("the second deletion claims to have moved something: %+v", deletion)
	}
}

// The If-Match is honoured even when the deletion would have been a no-op: the state the caller was
// reasoning about is not the state that is there.
func TestDeletingHonoursTheExpectedVersion(t *testing.T) {
	h, deleter := newDeleteHarness()
	bucket := h.withBucket(doingBucket, "Doing", "a0")
	bucket.Version = 4
	h.buckets.stored[doingBucket] = bucket

	t.Run("the version reaches the repository", func(t *testing.T) {
		if _, err := deleter.Execute(context.Background(), actor(), DeleteBucketCommand{
			BucketID: doingBucket, ExpectedVersion: 4,
		}); err != nil {
			t.Fatalf("the deletion failed: %v", err)
		}
		if len(h.buckets.written) != 1 || h.buckets.written[0].expected != 4 {
			t.Fatalf("the write did not carry the version: %+v", h.buckets.written)
		}
	})

	t.Run("a stale version is refused on a column that is already gone", func(t *testing.T) {
		_, err := deleter.Execute(context.Background(), actor(), DeleteBucketCommand{
			BucketID: doingBucket, ExpectedVersion: 1,
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Fatalf("a stale version was accepted: %v", err)
		}
	})
}

func TestARefusedDeletionMovesNothing(t *testing.T) {
	h, deleter := newDeleteHarness()
	h.withBucket(doingBucket, "Doing", "a0")
	h.authorizer.err = shared.ErrForbidden

	_, err := deleter.Execute(context.Background(), actor(), DeleteBucketCommand{
		BucketID: doingBucket,
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the refusal did not come back: %v", err)
	}
	if len(h.buckets.moved) != 0 || h.uow.writes != 0 {
		t.Error("a refused deletion moved something")
	}
}

// I-C3: an archived collection is read-only, and so is one whose hub is archived.
func TestAColumnOnAnArchivedCollectionCannotBeDeleted(t *testing.T) {
	h, deleter := newDeleteHarness()
	h.withBucket(doingBucket, "Doing", "a0")

	collection := h.containers.stored[collectionID]
	archivedAt := now
	collection.ArchivedAt = &archivedAt
	h.containers.stored[collectionID] = collection

	_, err := deleter.Execute(context.Background(), actor(), DeleteBucketCommand{
		BucketID: doingBucket,
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a column on an archived collection was deleted: %v", err)
	}
	if len(h.buckets.moved) != 0 {
		t.Error("the entries were moved anyway")
	}
}

func TestDeletingNeedsAColumnThatExists(t *testing.T) {
	h, deleter := newDeleteHarness()
	_ = h

	t.Run("no column named", func(t *testing.T) {
		_, err := deleter.Execute(context.Background(), actor(), DeleteBucketCommand{})
		if shared.AsError(err).DetailCode != "buckets.bucket_id_required" {
			t.Fatalf("detail code %s", shared.AsError(err).DetailCode)
		}
	})

	t.Run("a column nobody has", func(t *testing.T) {
		_, err := deleter.Execute(context.Background(), actor(), DeleteBucketCommand{
			BucketID: doingBucket,
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("an unknown column was accepted: %v", err)
		}
	})
}

func TestTheDeleteDescriptorCarriesWhatTheChannelsNeed(t *testing.T) {
	descriptor := DeleteBucket{}.Descriptor()

	if descriptor.Name != DeleteBucketName || descriptor.TokenScope != bucketsWrite {
		t.Errorf("unexpected descriptor: %+v", descriptor)
	}
	if !descriptor.Audit.Required || descriptor.Audit.Action != BucketDeletedAction {
		t.Errorf("a deletion that writes nothing to the trail: %+v", descriptor.Audit)
	}
}

// The catalogue's answer names what became of the entries, so that every channel reports it alike.
func TestTheDeletionOutputNamesWhatBecameOfTheEntries(t *testing.T) {
	h, deleter := newDeleteHarness()
	first := h.withBucket(shared.MustParseID("0192f000-0000-7000-8000-0000000000f0"), "Todo", "a0")
	h.withBucket(doingBucket, "Doing", "a1")

	out, err := deleter.invoke(context.Background(), actor(), map[string]any{
		"bucket_id": doingBucket.String(),
	})
	if err != nil {
		t.Fatalf("the deletion failed: %v", err)
	}

	if out["bucket_id"] != doingBucket.String() {
		t.Errorf("bucket_id is %v", out["bucket_id"])
	}
	if out["target_bucket_id"] != first.ID.String() {
		t.Errorf("target_bucket_id is %v", out["target_bucket_id"])
	}
	if out["moved_items"] == nil {
		t.Error("the answer does not say how many entries moved")
	}
}
