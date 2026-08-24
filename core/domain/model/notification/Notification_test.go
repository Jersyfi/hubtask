// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification_test

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	tenant    = shared.ID("0192f000-0000-7000-8000-00000000000a")
	recipient = shared.ID("0192f000-0000-7000-8000-00000000000b")
	actor     = shared.ID("0192f000-0000-7000-8000-00000000000c")
	item      = shared.ID("0192f000-0000-7000-8000-00000000000d")
	eventID   = shared.ID("0192f000-0000-7000-8000-00000000000e")
	writtenAt = time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
)

func input() notification.NewInput {
	return notification.NewInput{
		ID:          "0192f000-0000-7000-8000-000000000001",
		TenantID:    tenant,
		RecipientID: recipient,
		Category:    notification.CategoryComment,
		Channel:     notification.ChannelEmail,
		EventID:     eventID,
		ItemID:      item,
		ActorID:     actor,
		At:          writtenAt,
	}
}

func TestANewNotificationIsPendingAndKeepsItsReferences(t *testing.T) {
	written, err := notification.New(input())
	if err != nil {
		t.Fatalf("writing the record: %v", err)
	}
	if written.State != notification.StatePending || !written.Pending() {
		t.Errorf("a new record is %q - it has to be pending", written.State)
	}
	if written.Reason != "" || written.SentAt != nil || written.Attempts != 0 {
		t.Errorf("a new record already carries an outcome: %+v", written)
	}
	if written.EventID != eventID || written.ItemID != item || written.ActorID != actor {
		t.Errorf("the references did not survive: %+v", written)
	}
	if written.CreatedAt != writtenAt {
		t.Errorf("created at %v, want %v", written.CreatedAt, writtenAt)
	}
}

func TestARecordThatNothingCouldHonourIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*notification.NewInput)
		detail string
	}{
		{"no tenant", func(in *notification.NewInput) { in.TenantID = "" },
			"notifications.tenant_missing"},
		{"no recipient", func(in *notification.NewInput) { in.RecipientID = "" },
			"notifications.recipient_missing"},
		{"an unknown category", func(in *notification.NewInput) { in.Category = "GOSSIP" },
			"notifications.category_unknown"},
		{"an unknown channel", func(in *notification.NewInput) { in.Channel = "CARRIER_PIGEON" },
			"notifications.channel_unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := input()
			tc.change(&in)

			_, err := notification.New(in)
			if err == nil {
				t.Fatalf("%s was accepted", tc.name)
			}
			if got := shared.AsError(err).DetailCode; got != tc.detail {
				t.Errorf("detail %q, want %q", got, tc.detail)
			}
		})
	}
}

func TestTheOutcomesAreRecordedOnTheRow(t *testing.T) {
	written, err := notification.New(input())
	if err != nil {
		t.Fatalf("writing the record: %v", err)
	}

	sentAt := writtenAt.Add(time.Minute)
	sent := written.Sent(sentAt)
	if sent.State != notification.StateSent || sent.SentAt == nil || !sent.SentAt.Equal(sentAt) {
		t.Errorf("sent %+v", sent)
	}
	if sent.Pending() {
		t.Error("a sent notification still reads as pending")
	}

	suppressed := written.Suppress(notification.ReasonCategoryOff)
	if suppressed.State != notification.StateSuppressed ||
		suppressed.Reason != notification.ReasonCategoryOff {
		t.Errorf("suppressed %+v - the record has to say why", suppressed)
	}

	// The whole point of the record: the original is untouched, so a suppression is a new value
	// rather than a mutation somebody else is holding a stale copy of.
	if written.State != notification.StatePending {
		t.Errorf("the original was mutated: %+v", written)
	}
}

// A failure between two attempts leaves the record pending. An operator reading FAILED on something
// the queue is still going to retry would be chasing a working system.
func TestAFailureIsOnlyFinalWhenTheQueueSaysSo(t *testing.T) {
	written, err := notification.New(input())
	if err != nil {
		t.Fatalf("writing the record: %v", err)
	}

	retrying := written.Failed(false)
	if retrying.State != notification.StatePending || retrying.Attempts != 1 {
		t.Errorf("a retriable failure left %+v - it has to stay pending and count the attempt",
			retrying)
	}
	if retrying.Reason != "" {
		t.Errorf("a retriable failure already carries the reason %q", retrying.Reason)
	}

	final := retrying.Failed(true)
	if final.State != notification.StateFailed || final.Attempts != 2 {
		t.Errorf("the last attempt left %+v", final)
	}
	if final.Reason != notification.ReasonDeliveryFailed {
		t.Errorf("reason %q, want %q", final.Reason, notification.ReasonDeliveryFailed)
	}
}

func TestTheClosedSetsAreClosed(t *testing.T) {
	for _, category := range notification.Categories() {
		if !category.Valid() {
			t.Errorf("%s is in the set and reads as unknown", category)
		}
		if category.String() != string(category) {
			t.Errorf("%s does not print as itself", category)
		}
	}
	if notification.Category("GOSSIP").Valid() {
		t.Error("an invented category reads as known")
	}

	for _, channel := range notification.Channels() {
		if !channel.Valid() {
			t.Errorf("%s is in the set and reads as unknown", channel)
		}
		if channel.String() != string(channel) {
			t.Errorf("%s does not print as itself", channel)
		}
	}
	if notification.Channel("SMOKE_SIGNAL").Valid() {
		t.Error("an invented channel reads as known")
	}

	if notification.StatePending.String() != "PENDING" {
		t.Error("a state does not print as itself")
	}
}

// The invitation is the message that decides whether somebody can use the system at all. A
// preference switching it off would be a lock nobody can open - and the setting is behind the door
// it would lock.
func TestOnlyTheInvitationCannotBeSwitchedOff(t *testing.T) {
	if notification.CategoryInvitation.Suppressible() {
		t.Error("the invitation can be switched off - that locks somebody out of their workspace")
	}
	for _, category := range []notification.Category{
		notification.CategoryAssignment, notification.CategoryMembership,
		notification.CategoryComment,
	} {
		if !category.Suppressible() {
			t.Errorf("%s cannot be switched off, and data-protection.md §9 says it must be",
				category)
		}
	}
}
