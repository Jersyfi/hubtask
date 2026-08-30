// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The advance warning of data-retention.md §6 (R-1, answered in G-12): the people who can stop a
// retention rule are told before it acts.

// administratorStore answers who administers, and records what it was asked about - the path is
// the half of the resolution that decides whether a hub's administrator hears about a collection.
type administratorStore struct {
	byPath map[string][]shared.ID
	asked  [][]identity.Scope
}

func (s *administratorStore) Along(
	context.Context, shared.ID, []identity.Scope,
) ([]identity.Membership, error) {
	return nil, nil
}

func (s *administratorStore) SharedItemsIn(
	context.Context, shared.ID, shared.ID,
) ([]shared.ID, error) {
	return nil, nil
}

func (s *administratorStore) Administrators(
	_ context.Context, path []identity.Scope,
) ([]shared.ID, error) {
	s.asked = append(s.asked, path)
	key := "tenant"
	if len(path) > 1 {
		key = "path"
	}
	return s.byPath[key], nil
}

func warningRecorder(
	members []shared.ID, administrators map[string][]shared.ID,
) (RecordRetentionWarning, *notificationStore, *jobQueue, *administratorStore) {
	notifications, jobs := newNotifications(), &jobQueue{}
	memberships := &administratorStore{byPath: administrators}
	return RecordRetentionWarning{
		Notifications: notifications,
		Accounts: newAccounts(
			person(anna, "Anna", "anna@example.org", "en"),
			person(bert, "Bert", "bert@example.org", "en"),
			person(carla, "Carla", "carla@example.org", "de"),
		),
		Memberships: memberships,
		Members:     &memberStore{members: members},
		Preferences: newPreferences(),
		Jobs:        jobs,
		Clock:       clock.Fixed(now), IDs: &idSequence{}, Signals: &signalLog{},
	}, notifications, jobs, memberships
}

func warningFor(recipients ...lifecycle.Recipient) RetentionWarning {
	return RetentionWarning{
		TenantID: tenant, ItemID: itemID,
		Path: []identity.Scope{
			identity.TenantScope(), identity.HubScope(collection), identity.CollectionScope(collection),
		},
		Recipients: recipients,
	}
}

// The three audiences of §2's vocabulary, each resolved through the thing that knows it: the entry
// for its members, the role matrix for the administrators.
func TestTheWarningReachesEveryAudienceTheRuleNames(t *testing.T) {
	service, notifications, jobs, memberships := warningRecorder(
		[]shared.ID{anna},
		map[string][]shared.ID{"path": {bert}, "tenant": {carla}},
	)

	err := service.Warn(t.Context(), warningFor(
		lifecycle.RecipientItemMembers,
		lifecycle.RecipientCollectionAdmins,
		lifecycle.RecipientTenantAdmins,
	))
	if err != nil {
		t.Fatalf("warning: %v", err)
	}

	written := notifications.written()
	if len(written) != 3 {
		t.Fatalf("%d records, want one per person: %+v", len(written), written)
	}
	told := map[shared.ID]bool{}
	for _, record := range written {
		told[record.RecipientID] = true
		if record.Category != domain.CategoryRetention {
			t.Errorf("the warning was written under %q", record.Category)
		}
		if record.ItemID != itemID {
			t.Errorf("the warning is about %q", record.ItemID)
		}
	}
	for _, who := range []shared.ID{anna, bert, carla} {
		if !told[who] {
			t.Errorf("%s was not told", who)
		}
	}
	if len(jobs.requests) != 3 {
		t.Errorf("%d deliveries queued", len(jobs.requests))
	}

	// The collection's administrators are resolved along the entry's whole path, and the tenant's
	// along the tenant alone: a role held on the hub administers the collection under it, and a
	// rule that said TENANT_ADMINS did not ask for the hub's.
	if len(memberships.asked) != 2 {
		t.Fatalf("the resolution asked %d times", len(memberships.asked))
	}
	if len(memberships.asked[0]) != 3 || len(memberships.asked[1]) != 1 {
		t.Errorf("the resolution asked along %+v", memberships.asked)
	}
}

// One person who qualifies twice is told once. The record is per person and per entry, not per
// reason they are in the audience.
func TestSomebodyWhoQualifiesTwiceIsToldOnce(t *testing.T) {
	service, notifications, _, _ := warningRecorder(
		[]shared.ID{anna},
		map[string][]shared.ID{"path": {anna}, "tenant": {anna}},
	)

	err := service.Warn(t.Context(), warningFor(
		lifecycle.RecipientItemMembers,
		lifecycle.RecipientCollectionAdmins,
		lifecycle.RecipientTenantAdmins,
	))
	if err != nil {
		t.Fatalf("warning: %v", err)
	}

	if written := notifications.written(); len(written) != 1 {
		t.Errorf("%d records for one person, want one: %+v", len(written), written)
	}
}

// A preference switched off suppresses the record rather than dropping it: the trail of what was
// decided about somebody is the point of writing it at all (C-09).
func TestAWarningSomebodySwitchedOffIsSuppressedRatherThanLost(t *testing.T) {
	preferences := newPreferences()
	preferences.switchOff(anna, domain.CategoryRetention)

	notifications, jobs := newNotifications(), &jobQueue{}
	service := RecordRetentionWarning{
		Notifications: notifications,
		Accounts:      newAccounts(person(anna, "Anna", "anna@example.org", "en")),
		Memberships:   &administratorStore{byPath: map[string][]shared.ID{}},
		Members:       &memberStore{members: []shared.ID{anna}},
		Preferences:   preferences,
		Jobs:          jobs,
		Clock:         clock.Fixed(now), IDs: &idSequence{}, Signals: &signalLog{},
	}

	if err := service.Warn(t.Context(), warningFor(lifecycle.RecipientItemMembers)); err != nil {
		t.Fatalf("warning: %v", err)
	}

	written := notifications.written()
	if len(written) != 1 {
		t.Fatalf("%d records, want the suppressed one", len(written))
	}
	if written[0].State != domain.StateSuppressed {
		t.Errorf("the record is %s, want suppressed", written[0].State)
	}
	if len(jobs.requests) != 0 {
		t.Errorf("%d deliveries queued for a message nobody wants", len(jobs.requests))
	}
}

// A rule that names nobody warns nobody, and asks nothing of the repositories: `notify` is off by
// default, and an empty audience is not an audience of everybody.
func TestAWarningWithNoAudienceAsksNothing(t *testing.T) {
	service, notifications, _, memberships := warningRecorder(
		[]shared.ID{anna}, map[string][]shared.ID{"tenant": {bert}},
	)

	if err := service.Warn(t.Context(), warningFor()); err != nil {
		t.Fatalf("warning: %v", err)
	}
	if len(notifications.written()) != 0 || len(memberships.asked) != 0 {
		t.Error("a rule that named nobody still went looking for somebody")
	}
}
