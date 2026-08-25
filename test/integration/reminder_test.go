// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The statements the reminders run on, against a real database (D-02): the round trip through the
// two array columns, the bound, the recomputation a moved due date causes, the purge that takes
// the reminders with the entry, and a cross-tenant negative for every method (gate SG-3).

func reminderRepo() postgres.ReminderRepository { return postgres.NewReminderRepository() }

func reminderIn(tenant, item shared.ID, spec string, due *work.DueDate) work.Reminder {
	reminder, err := work.NewReminder(work.NewReminderInput{
		ID: shared.ID(""), TenantID: tenant, ItemID: item, OffsetSpec: spec, Due: due,
		Now: created,
	})
	if err != nil {
		panic(err)
	}
	return reminder
}

// seedReminder writes one reminder on the entry and returns it as written.
func seedReminder(
	ctx context.Context, t *testing.T, tenant, item shared.ID, spec string, due *work.DueDate,
) work.Reminder {
	t.Helper()

	reminder := reminderIn(tenant, item, spec, due)
	reminder.ID = freshID(t)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return reminderRepo().Insert(ctx, reminder)
	}); err != nil {
		t.Fatalf("writing the reminder: %v", err)
	}
	return reminder
}

func dueIn(t *testing.T, value string) *work.DueDate {
	t.Helper()

	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("the due date does not parse: %v", err)
	}
	due, err := work.NewDueDate(&at, false, "Europe/Berlin")
	if err != nil {
		t.Fatalf("the due date was refused: %v", err)
	}
	return due
}

func TestAReminderRoundTripsWholeIncludingItsLists(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	recipient := seedAccount(ctx, t, tenantA)

	due := dueIn(t, "2026-09-01T17:00:00Z")
	written := reminderIn(tenantA, task, "REL:-PT1H", due)
	written.ID = freshID(t)
	written.Recipients = []shared.ID{recipient}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return reminderRepo().Insert(ctx, written)
	}); err != nil {
		t.Fatalf("writing the reminder: %v", err)
	}

	var stored work.Reminder
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = reminderRepo().Find(ctx, written.ID)
		return err
	}); err != nil {
		t.Fatalf("reading the reminder: %v", err)
	}

	if stored.Offset.Spec != "REL:-PT1H" || !stored.Offset.Relative {
		t.Errorf("the offset came back as %+v", stored.Offset)
	}
	if work.ChannelList(stored.Channels) != "EMAIL" {
		t.Errorf("the channels came back as %v", stored.Channels)
	}
	if work.RecipientList(stored.Recipients) != recipient.String() {
		t.Errorf("the recipients came back as %v", stored.Recipients)
	}
	if stored.State != work.ReminderPending || stored.Version != 1 || stored.UpdatedAt != nil {
		t.Errorf("the reminder came back as %+v", stored)
	}
	if stored.FireAt == nil || !stored.FireAt.Equal(*written.FireAt) {
		t.Errorf("the moment came back as %v rather than %v", stored.FireAt, written.FireAt)
	}

	// The edit, under the lock, and the two array columns written whole.
	spec := "REL:-PT2H"
	none := []shared.ID{}
	edited, changes, err := stored.Patched(
		work.ReminderPatch{OffsetSpec: &spec, Recipients: &none}, due, created.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("the patch was refused: %v", err)
	}
	if len(changes) != 2 {
		t.Errorf("the patch reported %v", changes)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return reminderRepo().Update(ctx, edited, stored.Version)
	}); err != nil {
		t.Fatalf("writing the edit: %v", err)
	}

	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = reminderRepo().Find(ctx, written.ID)
		return err
	}); err != nil {
		t.Fatalf("re-reading the reminder: %v", err)
	}
	if stored.Offset.Spec != "REL:-PT2H" || len(stored.Recipients) != 0 ||
		stored.Version != 2 || stored.UpdatedAt == nil {
		t.Errorf("the edit left the reminder as %+v", stored)
	}

	// The lock: the version that was already spent buys nothing.
	conflict := write(ctx, t, tenantA, func(ctx context.Context) error {
		return reminderRepo().Update(ctx, edited, 1)
	})
	if !errors.Is(conflict, shared.ErrVersionConflict) {
		t.Errorf("a stale edit answered %v", conflict)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return reminderRepo().Delete(ctx, stored.ID, stored.Version)
	}); err != nil {
		t.Fatalf("deleting the reminder: %v", err)
	}
	err = read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := reminderRepo().Find(ctx, stored.ID)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the deleted reminder answered %v", err)
	}
}

// The list is the entry's own, oldest first, and the count is what the bound is asked against.
func TestTheListAndTheCountAnswerOneEntry(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	other := seedTask(ctx, t, tenantA, authorA, collection)

	first := seedReminder(ctx, t, tenantA, task, "ABS:2026-09-01T08:00:00Z", nil)
	second := seedReminder(ctx, t, tenantA, task, "ABS:2026-09-02T08:00:00Z", nil)
	seedReminder(ctx, t, tenantA, other, "ABS:2026-09-03T08:00:00Z", nil)

	var listed []work.Reminder
	var count int
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if listed, err = reminderRepo().ListForItem(ctx, task); err != nil {
			return err
		}
		count, err = reminderRepo().CountForItem(ctx, task)
		return err
	}); err != nil {
		t.Fatalf("listing the reminders: %v", err)
	}

	if len(listed) != 2 || count != 2 {
		t.Fatalf("the entry answered %d reminders and a count of %d", len(listed), count)
	}
	if listed[0].ID != first.ID || listed[1].ID != second.ID {
		t.Errorf("the list is not oldest first: %v", listed)
	}
}

// What a moved due date does: the pending relative reminders follow, and the write spends no
// version - a client's If-Match must not be invalidated by arithmetic nobody asked for.
func TestARescheduledReminderMovesWithoutSpendingAVersion(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	due := dueIn(t, "2026-09-01T17:00:00Z")
	relative := seedReminder(ctx, t, tenantA, task, "REL:-PT1H", due)
	absolute := seedReminder(ctx, t, tenantA, task, "ABS:2026-09-01T08:00:00Z", nil)

	var pending []work.Reminder
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		pending, err = reminderRepo().ListPendingForItem(ctx, task)
		return err
	}); err != nil {
		t.Fatalf("reading the pending reminders: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("the entry answered %d pending reminders", len(pending))
	}

	moved := dueIn(t, "2026-09-04T17:00:00Z")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, reminder := range pending {
			rescheduled, changed, err := reminder.Rescheduled(moved)
			if err != nil {
				return err
			}
			if !changed {
				continue
			}
			if err := reminderRepo().Reschedule(ctx, rescheduled); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("rescheduling: %v", err)
	}

	var followed, stayed work.Reminder
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if followed, err = reminderRepo().Find(ctx, relative.ID); err != nil {
			return err
		}
		stayed, err = reminderRepo().Find(ctx, absolute.ID)
		return err
	}); err != nil {
		t.Fatalf("re-reading the reminders: %v", err)
	}

	if got := followed.FireAt.UTC().Format(time.RFC3339); got != "2026-09-04T16:00:00Z" {
		t.Errorf("the relative reminder fires at %s", got)
	}
	if followed.Version != 1 || followed.UpdatedAt != nil {
		t.Errorf("rescheduling spent a version or left a stamp: %+v", followed)
	}
	if got := stayed.FireAt.UTC().Format(time.RFC3339); got != "2026-09-01T08:00:00Z" {
		t.Errorf("the absolute reminder moved to %s", got)
	}
}

// The data catalogue's deletion path, proved rather than promised: the reminders die with the
// entry, through the composite foreign key's CASCADE.
func TestPurgingTheEntryTakesItsReminders(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	reminder := seedReminder(ctx, t, tenantA, task, "ABS:2026-09-01T08:00:00Z", nil)

	if _, err := adminPool(ctx, t).Exec(ctx,
		`DELETE FROM work_item WHERE id = $1`, task.String()); err != nil {
		t.Fatalf("purging the entry: %v", err)
	}

	var remaining int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM reminder WHERE id = $1`, reminder.ID.String()).
		Scan(&remaining); err != nil {
		t.Fatalf("counting the reminders: %v", err)
	}
	if remaining != 0 {
		t.Error("the reminder outlived the entry it reminded about")
	}
}

// Gate SG-3: one negative per port method.
func TestRemindersAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	reminder := seedReminder(ctx, t, tenantA, task, "ABS:2026-09-01T08:00:00Z", nil)

	t.Run("find", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := reminderRepo().Find(ctx, reminder.ID)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("tenant B's find answered %v", err)
		}
	})

	t.Run("list", func(t *testing.T) {
		var listed []work.Reminder
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			listed, err = reminderRepo().ListForItem(ctx, task)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if len(listed) != 0 {
			t.Errorf("tenant B listed tenant A's reminders: %v", listed)
		}
	})

	t.Run("pending", func(t *testing.T) {
		var listed []work.Reminder
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			listed, err = reminderRepo().ListPendingForItem(ctx, task)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if len(listed) != 0 {
			t.Errorf("tenant B listed tenant A's pending reminders: %v", listed)
		}
	})

	t.Run("count", func(t *testing.T) {
		var count int
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			count, err = reminderRepo().CountForItem(ctx, task)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("tenant B counted %d of tenant A's reminders", count)
		}
	})

	t.Run("insert", func(t *testing.T) {
		// The insert writes current_tenant_id() and never the caller's claim, so a row naming
		// tenant A's entry does not land in tenant A - the composite foreign key finds no such
		// entry in tenant B, and the write fails rather than crossing.
		foreign := reminderIn(tenantA, task, "ABS:2026-09-01T08:00:00Z", nil)
		foreign.ID = freshID(t)
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return reminderRepo().Insert(ctx, foreign)
		}); err == nil {
			t.Error("tenant B wrote a reminder on tenant A's entry")
		}
	})

	t.Run("update", func(t *testing.T) {
		spec := "ABS:2026-09-09T08:00:00Z"
		edited, _, err := reminder.Patched(work.ReminderPatch{OffsetSpec: &spec}, nil, created)
		if err != nil {
			t.Fatal(err)
		}
		writeErr := write(ctx, t, tenantB, func(ctx context.Context) error {
			return reminderRepo().Update(ctx, edited, reminder.Version)
		})
		if !errors.Is(writeErr, shared.ErrVersionConflict) {
			t.Fatalf("tenant B's edit answered %v", writeErr)
		}
	})

	t.Run("reschedule", func(t *testing.T) {
		// No conflict to report - rescheduling matches what it finds - so the proof is that the
		// row did not move.
		moved := reminder
		later := created.Add(48 * time.Hour)
		moved.FireAt = &later
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return reminderRepo().Reschedule(ctx, moved)
		}); err != nil {
			t.Fatalf("tenant B's reschedule answered %v", err)
		}

		var stored work.Reminder
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			var err error
			stored, err = reminderRepo().Find(ctx, reminder.ID)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if !stored.FireAt.Equal(*reminder.FireAt) {
			t.Errorf("tenant B moved tenant A's reminder to %v", stored.FireAt)
		}
	})

	t.Run("delete", func(t *testing.T) {
		writeErr := write(ctx, t, tenantB, func(ctx context.Context) error {
			return reminderRepo().Delete(ctx, reminder.ID, reminder.Version)
		})
		if !errors.Is(writeErr, shared.ErrVersionConflict) {
			t.Fatalf("tenant B's delete answered %v", writeErr)
		}
	})
}

// The merge rule offline-sync.md §4.2 gains for a reminder, proved the way D-01 proved the due
// trio's: two devices editing two different fields converge to both, because each field is its own
// change log entry and the version predicate makes the loser re-read rather than overwrite.
//
// The second field is the recipients rather than the channels: this installation sends on one
// channel, so a channel list has nothing to change to, and the rule under test is per field rather
// than about any particular pair.
func TestTwoDevicesEditingAReminderConvergeToBothFields(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	recipient := seedAccount(ctx, t, tenantA)

	due := dueIn(t, "2026-09-01T17:00:00Z")
	seen := seedReminder(ctx, t, tenantA, task, "REL:-PT1H", due)

	// Device A moves the offset.
	spec := "REL:-PT2H"
	fromA, _, err := seen.Patched(work.ReminderPatch{OffsetSpec: &spec}, due, created.Add(time.Hour))
	if err != nil {
		t.Fatalf("device A's patch was refused: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return reminderRepo().Update(ctx, fromA, seen.Version)
	}); err != nil {
		t.Fatalf("device A's write: %v", err)
	}

	// Device B names a recipient against the state it read, and the version is what catches it.
	named := []shared.ID{recipient}
	fromB, _, err := seen.Patched(
		work.ReminderPatch{Recipients: &named}, due, created.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("device B's patch was refused: %v", err)
	}
	staleErr := write(ctx, t, tenantA, func(ctx context.Context) error {
		return reminderRepo().Update(ctx, fromB, seen.Version)
	})
	if !errors.Is(staleErr, shared.ErrVersionConflict) {
		t.Fatalf("the concurrent write answered %v, want a version conflict", staleErr)
	}

	// B re-reads and retries with its one field on top of what is now there - the per-field merge,
	// which is the whole client protocol.
	var current work.Reminder
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		current, err = reminderRepo().Find(ctx, seen.ID)
		return err
	}); err != nil {
		t.Fatalf("device B's re-read: %v", err)
	}
	retryB, _, err := current.Patched(
		work.ReminderPatch{Recipients: &named}, due, created.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("device B's retry was refused: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return reminderRepo().Update(ctx, retryB, current.Version)
	}); err != nil {
		t.Fatalf("device B's retry: %v", err)
	}

	var converged work.Reminder
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		converged, err = reminderRepo().Find(ctx, seen.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if converged.Offset.Spec != "REL:-PT2H" {
		t.Errorf("device A's offset was lost: %s", converged.Offset.Spec)
	}
	if work.RecipientList(converged.Recipients) != recipient.String() {
		t.Errorf("device B's recipient was lost: %v", converged.Recipients)
	}
	// And the moment follows the field that decides it, without either device having sent one.
	if got := converged.FireAt.UTC().Format(time.RFC3339); got != "2026-09-01T15:00:00Z" {
		t.Errorf("the converged reminder fires at %s", got)
	}
}
