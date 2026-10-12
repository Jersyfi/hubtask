// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"
	"time"

	lifecyclerepo "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
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
