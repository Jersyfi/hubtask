// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

func invitation(accounts *accountStore) (RecordInvitation, *notificationStore, *jobQueue) {
	notifications, jobs := newNotifications(), &jobQueue{}
	return RecordInvitation{
		Notifications: notifications, Accounts: accounts, Jobs: jobs,
		Clock: clock.Fixed(now), IDs: &idSequence{}, Signals: &signalLog{},
	}, notifications, jobs
}

func TestAnInvitationBecomesARecordAndADelivery(t *testing.T) {
	service, notifications, jobs := invitation(newAccounts(
		person(anna, "Anna", "anna@example.org", "en"),
		person(bert, "Bert", "bert@example.org", "en"),
	))

	if err := service.Execute(t.Context(), tenant, bert, anna); err != nil {
		t.Fatalf("recording the invitation: %v", err)
	}

	written := notifications.written()
	if len(written) != 1 {
		t.Fatalf("%d records written, want one", len(written))
	}
	record := written[0]
	if record.Category != domain.CategoryInvitation || record.State != domain.StatePending {
		t.Errorf("record %+v", record)
	}
	if record.RecipientID != bert || record.ActorID != anna {
		t.Errorf("record %+v", record)
	}
	// No event, because this one is not announced by the outbox.
	if !record.EventID.IsZero() || !record.ItemID.IsZero() {
		t.Errorf("the invitation carries references it has no business carrying: %+v", record)
	}

	// The same delivery job as everything else: one place renders an email, one place sends it.
	if len(jobs.requests) != 1 || jobs.requests[0].Kind != queue.KindNotificationDeliver {
		t.Fatalf("queued %v", jobs.requests)
	}
	if jobs.requests[0].Payload["notification_id"] != record.ID.String() {
		t.Errorf("payload %v", jobs.requests[0].Payload)
	}
}

// A seat created by the control plane has nobody behind it, and a message that names nobody is
// still an invitation.
func TestAnInvitationFromNobodyIsStillSent(t *testing.T) {
	service, notifications, jobs := invitation(newAccounts(
		person(bert, "Bert", "bert@example.org", "en"),
	))

	if err := service.Execute(t.Context(), tenant, bert, ""); err != nil {
		t.Fatalf("recording the invitation: %v", err)
	}
	if len(jobs.requests) != 1 {
		t.Fatalf("queued %v", jobs.requests)
	}
	if !notifications.written()[0].ActorID.IsZero() {
		t.Error("an actor was invented")
	}
}

// The account went between the invitation and the job. Finished business, not a failure - a job
// that failed here would retry until the dead letter over somebody who was removed.
func TestAnInvitationToSomebodyWhoIsGoneIsFinished(t *testing.T) {
	service, notifications, jobs := invitation(newAccounts())

	if err := service.Execute(t.Context(), tenant, bert, anna); err != nil {
		t.Fatalf("recording the invitation: %v", err)
	}
	if len(notifications.written()) != 0 || len(jobs.requests) != 0 {
		t.Errorf("records %v, jobs %v", notifications.written(), jobs.requests)
	}
}

// An invited account with no address cannot exist through InviteAccount. Recorded rather than
// assumed away: a record that says why is the point of the table.
func TestAnInvitationWithNowhereToSendSaysSo(t *testing.T) {
	service, notifications, jobs := invitation(newAccounts(
		identity.Account{ID: bert, TenantID: tenant, DisplayName: "Bert"},
	))

	if err := service.Execute(t.Context(), tenant, bert, anna); err != nil {
		t.Fatalf("recording the invitation: %v", err)
	}

	written := notifications.written()
	if len(written) != 1 {
		t.Fatalf("%d records written, want one", len(written))
	}
	if written[0].State != domain.StateSuppressed || written[0].Reason != domain.ReasonNoAddress {
		t.Errorf("record %+v", written[0])
	}
	if len(jobs.requests) != 0 {
		t.Errorf("a message was queued to nowhere: %v", jobs.requests)
	}
}
