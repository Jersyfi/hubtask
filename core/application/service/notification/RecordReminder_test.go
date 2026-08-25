// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

func reminderRecorder(
	preferences *preferenceStore,
) (RecordReminder, *notificationStore, *jobQueue) {
	notifications, jobs := newNotifications(), &jobQueue{}
	return RecordReminder{
		Notifications: notifications, Preferences: preferences,
		Accounts: newAccounts(
			person(anna, "Anna", "anna@example.org", "en"),
			person(bert, "Bert", "bert@example.org", "de"),
		),
		Jobs:  jobs,
		Clock: clock.Fixed(now), IDs: &idSequence{}, Signals: &signalLog{},
	}, notifications, jobs
}

// A reminder that fires is a record per recipient and the same delivery job every other
// notification uses - not a second path to an inbox.
func TestAFiredReminderBecomesARecordPerRecipient(t *testing.T) {
	service, notifications, jobs := reminderRecorder(newPreferences())

	if err := service.Execute(
		t.Context(), tenant, itemID, []shared.ID{anna, bert},
	); err != nil {
		t.Fatalf("recording the reminders: %v", err)
	}

	written := notifications.written()
	if len(written) != 2 {
		t.Fatalf("%d records written, want one per recipient", len(written))
	}
	for _, record := range written {
		if record.Category != domain.CategoryReminder || record.State != domain.StatePending {
			t.Errorf("record %+v", record)
		}
		if record.ItemID != itemID {
			t.Errorf("the record is about %s rather than the entry", record.ItemID)
		}
		// Nobody caused it, and no event announced it: the clock did. An actor here would be
		// somebody suppressing their own reminder through the self-caused rule.
		if !record.ActorID.IsZero() || !record.EventID.IsZero() {
			t.Errorf("the reminder carries references it has no business carrying: %+v", record)
		}
	}

	if len(jobs.requests) != 2 {
		t.Fatalf("queued %v", jobs.requests)
	}
	for i, request := range jobs.requests {
		if request.Kind != queue.KindNotificationDeliver {
			t.Errorf("queued %s", request.Kind)
		}
		if request.Payload["notification_id"] != written[i].ID.String() {
			t.Errorf("payload %v", request.Payload)
		}
	}
}

// Somebody who has switched reminders off gets the record that says so and no message. The record
// is the point: a person asking why they heard nothing deserves better than silence.
func TestAReminderSomebodySwitchedOffIsSuppressedAndRecorded(t *testing.T) {
	preferences := newPreferences()
	preferences.switchOff(bert, domain.CategoryReminder)
	service, notifications, jobs := reminderRecorder(preferences)

	if err := service.Execute(t.Context(), tenant, itemID, []shared.ID{bert}); err != nil {
		t.Fatalf("recording the reminder: %v", err)
	}

	written := notifications.written()
	if len(written) != 1 || written[0].State != domain.StateSuppressed {
		t.Fatalf("records %+v", written)
	}
	if written[0].Reason == "" {
		t.Error("the suppressed record does not say why")
	}
	if len(jobs.requests) != 0 {
		t.Errorf("a suppressed reminder queued %v", jobs.requests)
	}
}

// An account that has gone between the reminder and its moment is nowhere to send to, which is a
// state rather than a failure of the pass.
func TestAReminderForAnAccountThatIsGoneIsRecordedAsUnreachable(t *testing.T) {
	service, notifications, jobs := reminderRecorder(newPreferences())

	if err := service.Execute(t.Context(), tenant, itemID, []shared.ID{carla}); err != nil {
		t.Fatalf("recording the reminder: %v", err)
	}

	written := notifications.written()
	if len(written) != 1 || written[0].State != domain.StateSuppressed {
		t.Fatalf("records %+v", written)
	}
	if len(jobs.requests) != 0 {
		t.Errorf("an unreachable recipient queued %v", jobs.requests)
	}
}

// An empty recipient list is nobody to tell, not an error: an entry with no assignee and no
// members is a reminder somebody set and nobody is on.
func TestAReminderWithNobodyToTellWritesNothing(t *testing.T) {
	service, notifications, jobs := reminderRecorder(newPreferences())

	if err := service.Execute(t.Context(), tenant, itemID, nil); err != nil {
		t.Fatalf("recording the reminder: %v", err)
	}
	if len(notifications.written()) != 0 || len(jobs.requests) != 0 {
		t.Error("a reminder with no recipients wrote something")
	}
}
