// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"strings"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var (
	hubOpened = shared.ID("01936f2a-7c1e-7000-8000-0000000000d1")
	opening   = work.EmergencyAccess{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000d2"), TenantID: tenant, HubID: hubOpened,
		OpenedBy: anna, Reason: "Maria is in hospital", OpenedAt: now, EndsAt: now.Add(time.Hour),
	}
)

// accessStore answers the access a record names, with its hub's name.
type accessStore struct{ hubName string }

func (a accessStore) Find(_ context.Context, id shared.ID) (work.EmergencyAccess, string, error) {
	if id != opening.ID {
		return work.EmergencyAccess{}, "", shared.ErrNotFound
	}
	return opening, a.hubName, nil
}

// UC-ID-16 check 4: every member gets a record of their own in the opening's transaction - one
// without an address a suppressed one, which the web app still shows them - and the opener none.
// A member who switched the category off is sent it all the same: there is no silent variant.
func TestEveryMemberIsToldOfTheEmergencyAccess(t *testing.T) {
	notifications, preferences, jobs := newNotifications(), newPreferences(), &jobQueue{}
	preferences.switchOff(bert, domain.CategoryEmergencyAccess)
	child := person(carla, "Lena", "", "de")
	recorder := RecordEmergencyAccess{
		Notifications: notifications,
		Accounts: newAccounts(
			person(anna, "Paul", "paul@example.org", "en"),
			person(bert, "Maria", "maria@example.org", "de"),
			child,
		),
		Preferences: preferences, Jobs: jobs, Clock: clock.Fixed(now), IDs: &idSequence{},
	}

	if err := recorder.EmergencyAccessOpened(t.Context(), opening, []shared.ID{bert, carla, anna}); err != nil {
		t.Fatalf("recording: %v", err)
	}

	written := notifications.written()
	if len(written) != 2 {
		t.Fatalf("%d records, want one per member and none for the opener", len(written))
	}
	states := map[shared.ID]domain.Notification{}
	for _, record := range written {
		if record.Category != domain.CategoryEmergencyAccess || record.PrivateHubAccessID != opening.ID ||
			record.ActorID != anna {
			t.Errorf("record %+v does not name the access and its opener", record)
		}
		states[record.RecipientID] = record
	}
	if states[bert].State != domain.StatePending {
		t.Errorf("the member who switched it off: %s, want pending", states[bert].State)
	}
	if states[carla].State != domain.StateSuppressed || states[carla].Reason != domain.ReasonNoAddress {
		t.Errorf("the member without a mailbox: %s %s, want a suppressed record", states[carla].State, states[carla].Reason)
	}
	if len(jobs.requests) != 1 {
		t.Fatalf("%d deliveries queued, want one", len(jobs.requests))
	}
	if job := jobs.requests[0]; job.Kind != queue.KindEmergencyAccessEmail ||
		job.MaxAttempts != queue.EmergencyAccessAttempts {
		t.Errorf("queued %s with %d attempts, want its own kind and budget", job.Kind, job.MaxAttempts)
	}
}

// The mail says who opened which hub, from when until when in the reader's zone, and why - and
// leaves the hub's name out for somebody who keeps titles out of their mail, never the reason.
func TestTheEmergencyMailSaysWhoWhenAndWhy(t *testing.T) {
	for _, tc := range []struct {
		name         string
		includeTitle bool
	}{{"with the hub's name", true}, {"without titles", false}} {
		t.Run(tc.name, func(t *testing.T) {
			record, err := domain.New(domain.NewInput{
				ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e3"), TenantID: tenant, RecipientID: bert,
				Category: domain.CategoryEmergencyAccess, Channel: domain.ChannelEmail,
				PrivateHubAccessID: opening.ID, ActorID: anna, At: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			fixture := delivery(t, domain.CategoryEmergencyAccess, true)
			fixture.delivery.Accesses = accessStore{hubName: "Presents"}
			berlin := person(bert, "Maria", "maria@example.org", "en")
			berlin.TimeZone = "Europe/Berlin"
			fixture.delivery.Accounts = newAccounts(person(anna, "Paul", "paul@example.org", "en"), berlin)
			if !tc.includeTitle {
				quiet := domain.DefaultPreference(tenant, bert, domain.CategoryEmergencyAccess, domain.ChannelEmail)
				quiet.IncludeTitle = false
				_ = fixture.preferences.Save(t.Context(), quiet)
			}
			if _, err := fixture.notifications.Insert(t.Context(), record); err != nil {
				t.Fatal(err)
			}

			if err := fixture.delivery.Execute(t.Context(), tenant, record.ID, false); err != nil {
				t.Fatalf("delivering: %v", err)
			}
			if len(fixture.mailbox.sent) != 1 {
				t.Fatalf("%d messages sent, want one", len(fixture.mailbox.sent))
			}
			body := fixture.mailbox.sent[0].Body
			for _, want := range []string{
				"actor=Paul", "reason=Maria is in hospital",
				"when=2026-08-24 11:00 (Europe/Berlin)", "until=2026-08-24 12:00 (Europe/Berlin)",
				"link=" + baseURL + "/hubs/" + hubOpened.String(),
			} {
				if !strings.Contains(body, want) {
					t.Errorf("the body %q lacks %q", body, want)
				}
			}
			named := strings.Contains(body, "title=Presents")
			if named != tc.includeTitle {
				t.Errorf("the hub named: %v, want %v (%q)", named, tc.includeTitle, body)
			}
			wantCode := bodyEmergencyAccess
			if !tc.includeTitle {
				wantCode += withheldSuffix
			}
			if !strings.HasPrefix(body, wantCode+" ") {
				t.Errorf("the body %q is not %s", body, wantCode)
			}
		})
	}
}
