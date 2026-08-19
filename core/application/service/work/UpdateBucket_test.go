// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

var (
	doingBucket = shared.MustParseID("0192f000-0000-7000-8000-0000000000f1")
	doneBucket  = shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
)

// writerHarness is the bucket harness with the two change use cases wired to one writer, which is
// how the composition root builds them.
type writerHarness struct {
	*bucketHarness
	update  UpdateBucket
	reorder ReorderBucket
}

func newWriterHarness() *writerHarness {
	base := newBucketHarness()
	writer := BucketWriter{
		Buckets: base.buckets, Containers: base.containers, Authorizer: base.authorizer,
		Events: base.events, Changes: base.changes, Audit: base.audit, UnitOfWork: base.uow,
		Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	base.withCollection()
	return &writerHarness{
		bucketHarness: base,
		update:        UpdateBucket{Writer: writer},
		reorder:       ReorderBucket{Writer: writer},
	}
}

func updateCommand(attributes domain.BucketAttributes) UpdateBucketCommand {
	return UpdateBucketCommand{BucketID: doingBucket, Attributes: attributes}
}

// One change owes four things, and this is the test that says so.
func TestUpdatingABucketWritesTheRowTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newWriterHarness()
	h.withBucket(doingBucket, "Doing", "a0")
	ctx := correlation.ContextWithRequestID(context.Background(), "01J9REQUEST")

	name := "In progress"
	bucket, err := h.update.Execute(ctx, actor(), updateCommand(domain.BucketAttributes{Name: &name}))
	if err != nil {
		t.Fatalf("updating the bucket failed: %v", err)
	}

	if bucket.Name != "In progress" || bucket.Version != 2 {
		t.Errorf("unexpected bucket: %+v", bucket)
	}
	if len(h.buckets.written) != 1 || h.buckets.written[0].method != "attributes" {
		t.Fatalf("unexpected writes: %+v", h.buckets.written)
	}

	t.Run("the event carries a change set", func(t *testing.T) {
		if len(h.events.appended) != 1 {
			t.Fatalf("%d events, want 1", len(h.events.appended))
		}
		announcement := h.events.appended[0]
		if announcement.Type != event.BucketUpdated {
			t.Errorf("event type %s", announcement.Type)
		}
		changeSet, _ := announcement.Payload["change_set"].(map[string]any)
		if changeSet == nil || changeSet[domain.FieldName] == nil {
			t.Errorf("the change set does not name the field that moved: %+v", announcement.Payload)
		}
	})

	// One entry per field, each with its own clock reading: a device that renamed a column while
	// another set its limit has to keep both changes (offline-sync.md §4.2).
	t.Run("one change log entry per field", func(t *testing.T) {
		if len(h.changes.recorded) != 1 {
			t.Fatalf("%d changes, want 1", len(h.changes.recorded))
		}
		payload := h.changes.recorded[0].Payload
		if len(payload) != 1 || payload[domain.FieldName] != "In progress" {
			t.Errorf("the payload carries more than the field that moved: %+v", payload)
		}
		if h.changes.recorded[0].ContainerID != hubID {
			t.Errorf("the change is filed under %s, want the hub", h.changes.recorded[0].ContainerID)
		}
	})

	t.Run("the audit entry hides the name", func(t *testing.T) {
		if len(h.audit.entries) != 1 {
			t.Fatalf("%d audit entries, want 1", len(h.audit.entries))
		}
		entry := h.audit.entries[0]
		if entry.Action != BucketUpdatedAction {
			t.Errorf("action %s", entry.Action)
		}
		recorded, _ := entry.Changes[domain.FieldName].(map[string]any)
		if recorded == nil || recorded["changed"] != true {
			t.Fatalf("the name is not in the trail: %+v", entry.Changes)
		}
		if _, readable := recorded["to"]; readable {
			t.Errorf("the name is in the trail in clear text: %+v", recorded)
		}
	})
}

// Two fields moving produce two change log entries, each with its own clock reading. One entry
// covering both would give the pair a single HLC, and the merge would then decide them together -
// silently discarding whichever field a second device had written concurrently.
func TestEachFieldThatMovesGetsItsOwnEntry(t *testing.T) {
	h := newWriterHarness()
	h.withBucket(doingBucket, "Doing", "a0")

	name, limit := "In progress", 3
	if _, err := h.update.Execute(context.Background(), actor(), updateCommand(
		domain.BucketAttributes{Name: &name, WipLimit: &limit})); err != nil {
		t.Fatalf("updating the bucket failed: %v", err)
	}

	if len(h.changes.recorded) != 2 {
		t.Fatalf("%d change log entries, want one per field", len(h.changes.recorded))
	}
	first, second := h.changes.recorded[0], h.changes.recorded[1]
	if first.HLC.Compare(second.HLC) == 0 {
		t.Error("the two entries share a clock reading, so a merge would decide them together")
	}
}

// A cleared field travels as null rather than as the empty string: the API spells "not set" that
// way, and a client merging "" into a colour would be merging a value this system never renders.
func TestAClearedFieldTravelsAsNull(t *testing.T) {
	h := newWriterHarness()
	bucket := h.withBucket(doingBucket, "Doing", "a0")
	bucket.ColorToken = "surface.blue"
	h.buckets.stored[doingBucket] = bucket

	empty := ""
	if _, err := h.update.Execute(context.Background(), actor(), updateCommand(
		domain.BucketAttributes{ColorToken: &empty})); err != nil {
		t.Fatalf("clearing the colour failed: %v", err)
	}

	if len(h.changes.recorded) != 1 {
		t.Fatalf("%d changes, want 1", len(h.changes.recorded))
	}
	if value := h.changes.recorded[0].Payload[domain.FieldColorToken]; value != nil {
		t.Errorf("the cleared colour travels as %v, want null", value)
	}
}

// An update that asks for what is already stored writes nothing, spends no version and announces
// nothing - which is what makes a client that echoes the whole object back harmless.
func TestAnUpdateToAColumnThatChangesNothingWritesNothing(t *testing.T) {
	h := newWriterHarness()
	h.withBucket(doingBucket, "Doing", "a0")

	name := "Doing"
	bucket, err := h.update.Execute(context.Background(), actor(), updateCommand(
		domain.BucketAttributes{Name: &name}))
	if err != nil {
		t.Fatalf("the update failed: %v", err)
	}

	if bucket.Version != 1 {
		t.Errorf("the version was spent: %d", bucket.Version)
	}
	if len(h.buckets.written) != 0 || len(h.events.appended) != 0 || len(h.audit.entries) != 0 {
		t.Error("an update that moved nothing wrote something")
	}
}

// The If-Match is honoured even when the change would have been a no-op: the state the caller was
// reasoning about is not the state that is there.
func TestAStaleVersionIsRefusedEvenWhenNothingWouldMove(t *testing.T) {
	h := newWriterHarness()
	bucket := h.withBucket(doingBucket, "Doing", "a0")
	bucket.Version = 4
	h.buckets.stored[doingBucket] = bucket

	name := "Doing"
	cmd := updateCommand(domain.BucketAttributes{Name: &name})
	cmd.ExpectedVersion = 1

	_, err := h.update.Execute(context.Background(), actor(), cmd)
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale version was accepted: %v", err)
	}
}

// The version the caller read is what the update matches on, so that a concurrent write between
// the read and the write is caught.
func TestTheExpectedVersionReachesTheRepository(t *testing.T) {
	h := newWriterHarness()
	bucket := h.withBucket(doingBucket, "Doing", "a0")
	bucket.Version = 4
	h.buckets.stored[doingBucket] = bucket

	name := "In progress"
	cmd := updateCommand(domain.BucketAttributes{Name: &name})
	cmd.ExpectedVersion = 4

	if _, err := h.update.Execute(context.Background(), actor(), cmd); err != nil {
		t.Fatalf("the update failed: %v", err)
	}
	if len(h.buckets.written) != 1 || h.buckets.written[0].expected != 4 {
		t.Fatalf("the write did not carry the version: %+v", h.buckets.written)
	}
}

// I-C3: an archived collection is read-only, and so is one whose hub is archived. Checked inside
// the transaction rather than trusted from the read before it.
func TestAColumnOnAnArchivedCollectionIsReadOnly(t *testing.T) {
	h := newWriterHarness()
	h.withBucket(doingBucket, "Doing", "a0")

	collection := h.containers.stored[collectionID]
	archivedAt := now
	collection.ArchivedAt = &archivedAt
	h.containers.stored[collectionID] = collection

	name := "In progress"
	_, err := h.update.Execute(context.Background(), actor(), updateCommand(
		domain.BucketAttributes{Name: &name}))
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a column on an archived collection was changed: %v", err)
	}
}

func TestUpdatingAColumnThatIsNotThere(t *testing.T) {
	h := newWriterHarness()

	name := "In progress"
	_, err := h.update.Execute(context.Background(), actor(), updateCommand(
		domain.BucketAttributes{Name: &name}))
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("an unknown column was accepted: %v", err)
	}
	if shared.AsError(err).DetailCode != "buckets.not_found" {
		t.Errorf("detail code %s", shared.AsError(err).DetailCode)
	}
}

func TestUpdatingNeedsAColumn(t *testing.T) {
	h := newWriterHarness()

	name := "In progress"
	_, err := h.update.Execute(context.Background(), actor(), UpdateBucketCommand{
		Attributes: domain.BucketAttributes{Name: &name},
	})
	if shared.AsError(err).DetailCode != "buckets.bucket_id_required" {
		t.Fatalf("detail code %s", shared.AsError(err).DetailCode)
	}
}

func TestARefusedUpdateWritesNothing(t *testing.T) {
	h := newWriterHarness()
	h.withBucket(doingBucket, "Doing", "a0")
	h.authorizer.err = shared.ErrForbidden

	name := "In progress"
	_, err := h.update.Execute(context.Background(), actor(), updateCommand(
		domain.BucketAttributes{Name: &name}))
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the refusal did not come back: %v", err)
	}
	if len(h.buckets.written) != 0 || h.uow.writes != 0 {
		t.Error("a refused update wrote something")
	}
}

// A reorder is its own verb with its own event and its own audit entry: a rule that reacts to a
// column being renamed must not fire when somebody drags it one place to the left.
func TestReorderingAColumnMovesItAndAnnouncesItsOwnEvent(t *testing.T) {
	h := newWriterHarness()
	h.withBucket(doingBucket, "Doing", "a1")
	h.withBucket(doneBucket, "Done", "a2")
	first := h.withBucket(shared.MustParseID("0192f000-0000-7000-8000-0000000000f0"), "Todo", "a0")

	bucket, err := h.reorder.Execute(context.Background(), actor(), ReorderBucketCommand{
		BucketID: doneBucket, BeforeBucketID: first.ID,
	})
	if err != nil {
		t.Fatalf("reordering failed: %v", err)
	}

	if bucket.OrderKey >= "a0" {
		t.Errorf("the column ranks %q, want it before a0", bucket.OrderKey)
	}
	if len(h.buckets.written) != 1 || h.buckets.written[0].method != "order_key" {
		t.Fatalf("unexpected writes: %+v", h.buckets.written)
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.BucketReordered {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != BucketReorderedAction {
		t.Fatalf("unexpected audit entries: %+v", h.audit.entries)
	}

	// A rank is a key this server produced. There is no personal data in "a1", and an auditor
	// asking when a board was rearranged has no other way to answer it.
	recorded, _ := h.audit.entries[0].Changes[domain.FieldOrderKey].(map[string]any)
	if recorded == nil || recorded["to"] == nil {
		t.Errorf("the rank is not readable in the trail: %+v", h.audit.entries[0].Changes)
	}
}

// No anchor is the right hand end, which is a position like any other rather than a special case.
func TestReorderingWithoutAnAnchorMovesToTheEnd(t *testing.T) {
	h := newWriterHarness()
	first := h.withBucket(doingBucket, "Doing", "a0")
	h.withBucket(doneBucket, "Done", "a1")

	bucket, err := h.reorder.Execute(context.Background(), actor(), ReorderBucketCommand{
		BucketID: first.ID,
	})
	if err != nil {
		t.Fatalf("reordering failed: %v", err)
	}
	if bucket.OrderKey <= "a1" {
		t.Errorf("the column ranks %q, want it after a1", bucket.OrderKey)
	}
}

func TestReorderingRefusesAnAnchorItCannotUse(t *testing.T) {
	h := newWriterHarness()
	h.withBucket(doingBucket, "Doing", "a0")

	for _, c := range []struct {
		name       string
		before     shared.ID
		detailCode string
	}{
		{
			name:       "the column itself",
			before:     doingBucket,
			detailCode: "buckets.before_bucket_is_self",
		},
		{
			name:       "a column that is not on the board",
			before:     shared.MustParseID("0192f000-0000-7000-8000-0000000000f9"),
			detailCode: "buckets.before_bucket_not_on_board",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := h.reorder.Execute(context.Background(), actor(), ReorderBucketCommand{
				BucketID: doingBucket, BeforeBucketID: c.before,
			})
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("the anchor was accepted: %v", err)
			}
			if shared.AsError(err).DetailCode != c.detailCode {
				t.Errorf("detail code %s, want %s", shared.AsError(err).DetailCode, c.detailCode)
			}
		})
	}
}

// A deleted column is off the board, and every verb that changes one says so with the same answer.
func TestADeletedColumnRefusesEveryChange(t *testing.T) {
	h := newWriterHarness()
	bucket := h.withBucket(doingBucket, "Doing", "a0")
	deleted, _, err := bucket.Deleted(now)
	if err != nil {
		t.Fatalf("deleting the bucket: %v", err)
	}
	h.buckets.stored[doingBucket] = deleted

	name := "In progress"
	if _, err := h.update.Execute(context.Background(), actor(), updateCommand(
		domain.BucketAttributes{Name: &name})); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a deleted column was renamed: %v", err)
	}
	if _, err := h.reorder.Execute(context.Background(), actor(), ReorderBucketCommand{
		BucketID: doingBucket,
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a deleted column was reordered: %v", err)
	}
}

func TestTheChangeDescriptorsCarryWhatTheChannelsNeed(t *testing.T) {
	update := UpdateBucket{}.Descriptor()
	if update.Name != UpdateBucketName || !update.Audit.Required {
		t.Errorf("unexpected descriptor: %+v", update)
	}

	reorder := ReorderBucket{}.Descriptor()
	if reorder.Audit.Action != BucketReorderedAction {
		t.Errorf("the reorder writes the wrong audit action: %s", reorder.Audit.Action)
	}
}

// The untyped input becomes the typed command in one place, for all three channels - including the
// two fields whose value carries no "absent" spelling of its own.
func TestTheUpdateReadsPresenceRatherThanValue(t *testing.T) {
	h := newWriterHarness()
	bucket := h.withBucket(doingBucket, "Doing", "a0")
	limit := 3
	bucket.WipLimit, bucket.IsDoneBucket = &limit, true
	h.buckets.stored[doingBucket] = bucket

	t.Run("a field nobody sent is left alone", func(t *testing.T) {
		name := "In progress"
		out, err := h.update.invoke(context.Background(), actor(), map[string]any{
			"bucket_id": doingBucket.String(), "name": name,
		})
		if err != nil {
			t.Fatalf("the update failed: %v", err)
		}
		if out["wip_limit"] != 3 || out["is_done_bucket"] != true {
			t.Errorf("an untouched field moved: %+v", out)
		}
	})

	t.Run("zero clears the limit and false is a state somebody wanted", func(t *testing.T) {
		out, err := h.update.invoke(context.Background(), actor(), map[string]any{
			"bucket_id": doingBucket.String(), "wip_limit": 0, "is_done_bucket": false,
		})
		if err != nil {
			t.Fatalf("the update failed: %v", err)
		}
		if out["wip_limit"] != nil || out["is_done_bucket"] != false {
			t.Errorf("the fields did not move: %+v", out)
		}
	})

	t.Run("an update that asks for nothing is refused", func(t *testing.T) {
		_, err := h.update.invoke(context.Background(), actor(), map[string]any{
			"bucket_id": doingBucket.String(),
		})
		if shared.AsError(err).DetailCode != "buckets.update_empty" {
			t.Fatalf("detail code %s", shared.AsError(err).DetailCode)
		}
	})
}
