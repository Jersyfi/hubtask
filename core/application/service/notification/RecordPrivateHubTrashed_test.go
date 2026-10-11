// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// UC-ID-16 check 6: each of the workspace's owners gets a membership message about nothing - no
// entry, no actor - and an owner who switched membership mail off gets a suppressed record.
func TestTheOwnersAreToldThatAPrivateHubWentWithoutItsName(t *testing.T) {
	notifications, preferences, jobs := newNotifications(), newPreferences(), &jobQueue{}
	preferences.switchOff(bert, domain.CategoryMembership)
	recorder := RecordPrivateHubTrashed{
		Notifications: notifications,
		Accounts: newAccounts(
			person(anna, "Anna", "anna@example.org", "en"),
			person(bert, "Bert", "bert@example.org", "de"),
		),
		Preferences: preferences, Jobs: jobs, Clock: clock.Fixed(now), IDs: &idSequence{},
	}

	if err := recorder.PrivateHubTrashed(t.Context(), tenant, []shared.ID{anna, bert}); err != nil {
		t.Fatalf("recording: %v", err)
	}

	written := notifications.written()
	if len(written) != 2 {
		t.Fatalf("%d records, want one per owner", len(written))
	}
	for _, record := range written {
		if !record.PrivateHubTrashed() || !record.ActorID.IsZero() {
			t.Errorf("record %+v names something", record)
		}
	}
	if len(jobs.requests) != 1 {
		t.Errorf("%d deliveries queued, want one - the owner who switched it off is not sent to",
			len(jobs.requests))
	}
}

// The message has its own sentence, no title and no actor, and links to the list of private
// hubs rather than to anything inside one.
func TestThePrivateHubMessageNamesNothing(t *testing.T) {
	record, err := domain.New(domain.NewInput{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e2"), TenantID: tenant,
		RecipientID: bert, Category: domain.CategoryMembership, Channel: domain.ChannelEmail, At: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := delivery(t, domain.CategoryMembership, true)
	if _, err := fixture.notifications.Insert(t.Context(), record); err != nil {
		t.Fatal(err)
	}

	if err := fixture.delivery.Execute(t.Context(), tenant, record.ID, false); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(fixture.mailbox.sent) != 1 {
		t.Fatalf("%d messages sent, want one", len(fixture.mailbox.sent))
	}
	message := fixture.mailbox.sent[0]
	if !strings.HasPrefix(message.Subject, subjectPrivateHubTrashed+" ") ||
		strings.Contains(message.Subject, "title=") || strings.Contains(message.Subject, "actor=") {
		t.Errorf("the subject is %q", message.Subject)
	}
	if !strings.HasPrefix(message.Body, bodyPrivateHubTrashed+" ") ||
		!strings.Contains(message.Body, "link="+baseURL+"/administration/private-hubs") ||
		strings.Contains(message.Body, "title=") || strings.Contains(message.Body, "actor=") {
		t.Errorf("the body is %q", message.Body)
	}
}
