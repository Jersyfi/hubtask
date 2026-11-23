// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	lifecyclerepo "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

type oneHub struct{}

func (oneHub) ListPrivateHubs(context.Context) ([]repository.PrivateHubSummary, error) {
	return []repository.PrivateHubSummary{{ID: hubID, CreatedAt: now}}, nil
}

type noPolicies struct{ lifecyclerepo.Policies }

func (noPolicies) Find(context.Context, lifecycle.DataKind) (lifecycle.Policy, error) {
	return lifecycle.Policy{}, shared.ErrNotFound
}

func emergencyRow(t *testing.T, accesses *accessStore, owner bool, kind appshared.ActorKind) usecase.Output {
	t.Helper()
	registry, err := usecase.NewRegistry(nil, ListPrivateHubs{
		Hubs: oneHub{}, Policies: noPolicies{}, Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
		Accesses: accesses, Owners: &permitting{allow: owner}, Clock: clock.Fixed(now.Add(10 * time.Minute)),
	}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	who := actor()
	who.Kind = kind
	who.Scopes = append(who.Scopes, membersRead)
	out, err := registry.Invoke(context.Background(), ListPrivateHubsName, who, usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	row := out["data"].([]usecase.Output)[0]
	return row["emergency_access"].(usecase.Output)
}

// P-05: the list offers the emergency access exactly where OpenPrivateHub would accept it - the
// owner, a person, holding nothing in the hub, with none of theirs running - and says until when a
// running one lasts.
func TestTheListOffersTheEmergencyAccessWhereItWouldBeAccepted(t *testing.T) {
	if row := emergencyRow(t, &accessStore{members: []shared.ID{memberID}}, true, appshared.ActorUser); row["offered"] != true ||
		row["open_until"] != nil {
		t.Errorf("the owner outside the hub: %v, want offered", row)
	}
	if row := emergencyRow(t, &accessStore{}, false, appshared.ActorUser); row["offered"] != false {
		t.Errorf("an administrator: %v, want not offered", row)
	}
	if row := emergencyRow(t, &accessStore{}, true, appshared.ActorServiceAccount); row["offered"] != false {
		t.Errorf("a service account: %v, want not offered", row)
	}
	if row := emergencyRow(t, &accessStore{members: []shared.ID{accountID}}, true, appshared.ActorUser); row["offered"] != false {
		t.Errorf("the owner inside the hub: %v, want not offered", row)
	}
	running := &accessStore{accesses: []domain.EmergencyAccess{{
		ID: "a", HubID: hubID, OpenedBy: accountID, Reason: "x", OpenedAt: now, EndsAt: now.Add(time.Hour),
	}}}
	if row := emergencyRow(t, running, true, appshared.ActorUser); row["offered"] != false || row["open_until"] != now.Add(time.Hour) {
		t.Errorf("the owner with a running access: %v", row)
	}
}

// A person's own notices come from their records; nobody else has any.
func TestOnlyAPersonHasEmergencyNotices(t *testing.T) {
	store := &accessStore{accesses: []domain.EmergencyAccess{{
		ID: "a", HubID: hubID, OpenedBy: accountID, Reason: "Maria is in hospital", OpenedAt: now, EndsAt: now.Add(time.Hour),
	}}}
	registry, err := usecase.NewRegistry(nil, ListEmergencyNotices{
		Accesses: store, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	reader := actor()
	reader.Scopes = []string{containersRead}
	out, err := registry.Invoke(context.Background(), ListEmergencyNoticesName, reader, usecase.Input{})
	if err != nil || len(out["data"].([]usecase.Output)) != 1 {
		t.Fatalf("a member's notices: %v, %v", out, err)
	}
	reader.Kind = appshared.ActorServiceAccount
	out, err = registry.Invoke(context.Background(), ListEmergencyNoticesName, reader, usecase.Input{})
	if err != nil || len(out["data"].([]usecase.Output)) != 0 {
		t.Errorf("a service account's notices: %v, %v", out, err)
	}
}

// confirming is AcknowledgeEmergencyNotice and ListEmergencyNotices through the registry, over one
// access the owner (accountID) opened on hubID and told to memberID alone.
type confirming struct {
	registry *usecase.Registry
	store    *accessStore
	audit    *sink
	clock    *movingClock
}

var noticeAccess = shared.ID("0192f000-0000-7000-8000-0000000000e1")

func newConfirming(t *testing.T) *confirming {
	t.Helper()
	c := &confirming{
		store: &accessStore{
			accesses: []domain.EmergencyAccess{{
				ID: noticeAccess, TenantID: tenantID, HubID: hubID, OpenedBy: accountID,
				Reason: "Maria is in hospital", OpenedAt: now, EndsAt: now.Add(time.Hour),
			}},
			told: map[shared.ID][]shared.ID{noticeAccess: {memberID}},
		},
		audit: &sink{}, clock: &movingClock{at: now.Add(2 * time.Hour)},
	}
	registry, err := usecase.NewRegistry(nil,
		AcknowledgeEmergencyNotice{Accesses: c.store, Audit: c.audit, UnitOfWork: &unitOfWork{}, Clock: c.clock}.Descriptor(),
		ListEmergencyNotices{Accesses: c.store, UnitOfWork: &unitOfWork{}, Clock: c.clock}.Descriptor(),
	)
	if err != nil {
		t.Fatal(err)
	}
	c.registry = registry
	return c
}

func (c *confirming) confirm(who appshared.ActorContext) error {
	_, err := c.registry.Invoke(context.Background(), AcknowledgeEmergencyNoticeName, who,
		usecase.Input{"access_id": noticeAccess.String()})
	return err
}

func toldPerson(account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: account, AccountName: "Lena",
		Scopes: []string{containersRead, containersWrite},
	}
}

// UC-ID-16 check 10: a member who was told confirms once; the confirmation is stored for that
// member and is in the trail with the hub and the access, without the reason. Twice is one row and
// one entry.
func TestAMemberWhoWasToldConfirmsOnce(t *testing.T) {
	c := newConfirming(t)
	if err := c.confirm(toldPerson(memberID)); err != nil {
		t.Fatalf("the member's confirmation: %v", err)
	}
	c.clock.Set(now.Add(3 * time.Hour))
	if err := c.confirm(toldPerson(memberID)); err != nil {
		t.Fatalf("the second confirmation: %v", err)
	}
	if len(c.store.acknowledged) != 1 || !c.store.acknowledged[[2]shared.ID{noticeAccess, memberID}].Equal(now.Add(2*time.Hour)) {
		t.Errorf("stored %v, want the member's one confirmation at its first moment", c.store.acknowledged)
	}
	if len(c.audit.entries) != 1 {
		t.Fatalf("%d trail entries, want one", len(c.audit.entries))
	}
	entry := c.audit.entries[0]
	if entry.Action != EmergencyNoticeAcknowledgedAction || entry.ActorID != memberID ||
		entry.TargetType != containerTarget || entry.TargetID != hubID || entry.Severity != audit.SeverityInfo {
		t.Errorf("the entry is %+v", entry)
	}
	if len(entry.Changes) != 1 || entry.Changes["access_id"] == nil {
		t.Errorf("the entry's changes are %v, want the access's identifier alone", entry.Changes)
	}
	if strings.Contains(stringOf(entry.Changes), "hospital") || strings.Contains(fmt.Sprintf("%+v", entry), "hospital") {
		t.Errorf("the reason is in the trail: %+v", entry)
	}
}

// UC-ID-16 check 10: a confirmation by anybody who was not told answers "not found" - never
// "forbidden" - and writes nothing.
func TestAConfirmationByAnybodyNotToldIsNotFound(t *testing.T) {
	service := toldPerson(memberID)
	service.Kind = appshared.ActorServiceAccount
	robot := toldPerson(memberID)
	robot.Kind = appshared.ActorAIAgent
	for _, tc := range []struct {
		name string
		who  appshared.ActorContext
		at   time.Time
	}{
		{name: "the owner who opened it", who: toldPerson(accountID), at: now.Add(2 * time.Hour)},
		{name: "a member of another hub", who: toldPerson(shared.ID("0192f000-0000-7000-8000-0000000000e2")), at: now.Add(2 * time.Hour)},
		{name: "a service account", who: service, at: now.Add(2 * time.Hour)},
		{name: "an AI actor", who: robot, at: now.Add(2 * time.Hour)},
		{name: "the member after the 30 days", who: toldPerson(memberID), at: now.Add(EmergencyNoticeDays*24*time.Hour + time.Second)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfirming(t)
			c.clock.Set(tc.at)
			err := c.confirm(tc.who)
			if !errors.Is(err, shared.ErrNotFound) || shared.AsError(err).DetailCode != "containers.emergency_access_not_found" {
				t.Errorf("answered %v, want not found", err)
			}
			if len(c.store.acknowledged) != 0 || len(c.audit.entries) != 0 {
				t.Errorf("wrote %v and %d entries", c.store.acknowledged, len(c.audit.entries))
			}
		})
	}
	t.Run("a credential without the write scope", func(t *testing.T) {
		c := newConfirming(t)
		reader := toldPerson(memberID)
		reader.Scopes = []string{containersRead}
		if err := c.confirm(reader); !errors.Is(err, shared.ErrForbidden) {
			t.Errorf("answered %v, want the scope refused", err)
		}
	})
}

// UC-ID-16 check 10: the member's list says when they confirmed - null until then.
func TestTheNoticesSayWhenTheMemberConfirmed(t *testing.T) {
	c := newConfirming(t)
	acknowledgedAt := func() any {
		t.Helper()
		out, err := c.registry.Invoke(context.Background(), ListEmergencyNoticesName, toldPerson(memberID), usecase.Input{})
		if err != nil {
			t.Fatal(err)
		}
		rows := out["data"].([]usecase.Output)
		if len(rows) != 1 {
			t.Fatalf("%d notices, want one", len(rows))
		}
		value, held := rows[0]["acknowledged_at"]
		if !held {
			t.Fatal("the notice carries no acknowledged_at")
		}
		return value
	}
	if at := acknowledgedAt(); at != nil {
		t.Errorf("before confirming: %v, want null", at)
	}
	if err := c.confirm(toldPerson(memberID)); err != nil {
		t.Fatal(err)
	}
	if at := acknowledgedAt(); at != now.Add(2*time.Hour) {
		t.Errorf("after confirming: %v, want the moment", at)
	}
}
