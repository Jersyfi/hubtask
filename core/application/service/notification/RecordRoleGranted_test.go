// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var (
	hubGranted = shared.ID("01936f2a-7c1e-7000-8000-0000000000f1")
	grantID    = shared.ID("01936f2a-7c1e-7000-8000-0000000000f2")
)

func grantOn(scope identity.Scope, account, group shared.ID) identity.Grant {
	return identity.Grant{
		ID: grantID, TenantID: tenant, AccountID: account, GroupID: group,
		Scope: scope, Role: identity.RoleMember,
	}
}

// UC-NOT-01 check 9 at the recording: one MEMBERSHIP record naming the grant for the person given
// the role, its delivery queued under its own kind - and the rules every other message keeps:
// nobody is told of their own act (check 2), nobody without an address is mailed and the record
// says why (check 6), and a group given a role mails nobody.
func TestARoleGivenIsRecordedForThePersonOnly(t *testing.T) {
	for _, tc := range []struct {
		name      string
		grant     identity.Grant
		actor     shared.ID
		records   int
		state     domain.State
		reason    string
		delivered bool
	}{
		{"a person", grantOn(identity.HubScope(hubGranted), bert, ""), anna, 1, domain.StatePending, "", true},
		{"oneself", grantOn(identity.HubScope(hubGranted), anna, ""), anna, 1, domain.StateSuppressed, domain.ReasonSelfCaused, false},
		{"no address", grantOn(identity.HubScope(hubGranted), carla, ""), anna, 1, domain.StateSuppressed, domain.ReasonNoAddress, false},
		{"a group", grantOn(identity.HubScope(hubGranted), "", shared.ID("01936f2a-7c1e-7000-8000-0000000000f3")), anna, 0, "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			notifications, jobs := newNotifications(), &jobQueue{}
			recorder := RecordRoleGranted{
				Notifications: notifications,
				Accounts: newAccounts(
					person(anna, "Anna", "anna@example.org", "en"),
					person(bert, "Bert", "bert@example.org", "de"),
					person(carla, "Carla", "", "en"),
				),
				Preferences: newPreferences(), Jobs: jobs, Clock: clock.Fixed(now), IDs: &idSequence{},
			}

			if err := recorder.RoleGranted(t.Context(), tc.grant, tc.actor); err != nil {
				t.Fatalf("recording: %v", err)
			}

			written := notifications.written()
			if len(written) != tc.records {
				t.Fatalf("%d records, want %d", len(written), tc.records)
			}
			if tc.records == 1 {
				record := written[0]
				if record.Category != domain.CategoryMembership || record.MembershipID != grantID ||
					record.RecipientID != tc.grant.AccountID || record.ActorID != tc.actor || !record.ItemID.IsZero() {
					t.Errorf("record %+v does not name the grant, the person and the actor", record)
				}
				if record.State != tc.state || record.Reason != tc.reason {
					t.Errorf("state %s %q, want %s %q", record.State, record.Reason, tc.state, tc.reason)
				}
			}
			if delivered := len(jobs.requests) == 1; delivered != tc.delivered {
				t.Fatalf("%d deliveries queued, want delivered=%v", len(jobs.requests), tc.delivered)
			}
			if tc.delivered {
				job := jobs.requests[0]
				if job.Kind != queue.KindRoleGrantedEmail || job.MaxAttempts != queue.RoleGrantedAttempts ||
					job.Payload["notification_id"] != written[0].ID.String() || len(job.Payload) != 1 {
					t.Errorf("queued %+v, want its own kind, budget and the record's identifier only", job)
				}
			}
		})
	}
}

// The person's own switch decides, as for being added to an entry: the category is MEMBERSHIP.
func TestARoleGivenHonoursTheMembershipSwitch(t *testing.T) {
	notifications, preferences, jobs := newNotifications(), newPreferences(), &jobQueue{}
	preferences.switchOff(bert, domain.CategoryMembership)
	recorder := RecordRoleGranted{
		Notifications: notifications, Accounts: newAccounts(person(bert, "Bert", "bert@example.org", "de")),
		Preferences: preferences, Jobs: jobs, Clock: clock.Fixed(now), IDs: &idSequence{},
	}

	if err := recorder.RoleGranted(t.Context(), grantOn(identity.TenantScope(), bert, ""), anna); err != nil {
		t.Fatalf("recording: %v", err)
	}
	written := notifications.written()
	if len(written) != 1 || written[0].Reason != domain.ReasonCategoryOff || len(jobs.requests) != 0 {
		t.Errorf("records %+v, jobs %d; want one suppressed for the switch and nothing queued", written, len(jobs.requests))
	}
}
