// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	groupGrantID = shared.ID("01936f2a-7c1e-7000-8000-0000000000e4")
	leaverID     = shared.ID("01936f2a-7c1e-7000-8000-0000000000e5")
	grantGroupID = shared.ID("01936f2a-7c1e-7000-8000-0000000000e6")
)

type grantLog struct{ changes []changelog.Change }

func (l *grantLog) Record(_ context.Context, change changelog.Change) error {
	l.changes = append(l.changes, change)
	return nil
}

type grantHLC struct{ counter uint32 }

func (h *grantHLC) Next() shared.HLC { h.counter++; return shared.HLC{Counter: h.counter} }

func offlineRoles(t *testing.T) (Roles, *roleStore, *grantLog) {
	t.Helper()
	store := newRoleStore(serviceLead(t))
	log := &grantLog{}
	roles := rolesWith(store, &authorizer{}, &auditSink{}, &heldBy{})
	roles.Changes, roles.HLC = log, &grantHLC{}
	return roles, store, log
}

// offline-sync.md §4.3: a grant given is filed for its holder's devices with what the walk hands
// out; a grant taken away is filed as a removal naming nobody.
func TestAGrantGivenAndTakenIsFiledForTheHoldersDevices(t *testing.T) {
	roles, store, log := offlineRoles(t)
	grant := GrantRole{
		Roles:  roles,
		Grants: GrantMembership{Accounts: newAccounts(invitedAccount(t)), Groups: newGroups(), Hubs: &hubLocks{}},
	}
	row, err := grant.Execute(t.Context(), admin(), GrantRoleCommand{
		Role: "service_lead", AccountID: invitedID, Scope: domain.HubScope(hubID),
	})
	if err != nil {
		t.Fatalf("granting: %v", err)
	}
	if len(log.changes) != 1 || log.changes[0].Entity != RoleGrantEntity || log.changes[0].Op != changelog.Upsert ||
		log.changes[0].Payload["account_id"] != invitedID.String() || log.changes[0].Payload["role"] != "service_lead" {
		t.Fatalf("filed %+v", log.changes)
	}
	if err := (RevokeRoleGrant{Roles: roles}).Execute(t.Context(), admin(), row.Grant.ID); err != nil {
		t.Fatalf("revoking: %v", err)
	}
	if len(store.grants) != 0 || len(log.changes) != 2 || log.changes[1].Op != changelog.Delete ||
		log.changes[1].EntityID != row.Grant.ID || log.changes[1].Payload != nil {
		t.Fatalf("filed %+v", log.changes)
	}
}

// A group's grant reaches who joins it and leaves who left: the removal names the leaver, so the
// members who stay keep it.
func TestAGroupsMembersMovingFilesItsGrantsForWhoJoinedAndLeft(t *testing.T) {
	roles, store, log := offlineRoles(t)
	store.grants[groupGrantID] = repository.CustomGrantRow{RoleKey: "service_lead", Grant: domain.CustomGrant{
		ID: groupGrantID, TenantID: tenant, GroupID: grantGroupID, Scope: domain.HubScope(hubID), RoleID: customRoleID,
	}}
	store.grants[otherGrantID] = repository.CustomGrantRow{RoleKey: "service_lead", Grant: domain.CustomGrant{
		ID: otherGrantID, TenantID: tenant, AccountID: invitedID, Scope: domain.HubScope(hubID), RoleID: customRoleID,
	}}
	if err := roles.GroupMembersMoved(t.Context(), admin(), grantGroupID, []shared.ID{invitedID}, []shared.ID{leaverID}); err != nil {
		t.Fatal(err)
	}
	if len(log.changes) != 2 {
		t.Fatalf("filed %+v, want the group's one grant for the joiner and the leaver", log.changes)
	}
	joined, left := log.changes[0], log.changes[1]
	if joined.Op != changelog.Upsert || joined.EntityID != groupGrantID || joined.Payload["group_id"] != grantGroupID.String() {
		t.Errorf("joined %+v", joined)
	}
	if left.Op != changelog.Delete || left.EntityID != groupGrantID || left.Payload["account_id"] != leaverID.String() {
		t.Errorf("left %+v", left)
	}

	log.changes = nil
	if err := roles.GroupDeleting(t.Context(), admin(), grantGroupID); err != nil {
		t.Fatal(err)
	}
	if len(log.changes) != 1 || log.changes[0].Op != changelog.Delete || log.changes[0].Payload != nil {
		t.Errorf("a group's deletion filed %+v, want its grant removed for every device", log.changes)
	}
}

func TestDeletingARoleFilesTheRemovalOfEachOfItsGrants(t *testing.T) {
	roles, store, log := offlineRoles(t)
	store.grants[membershipID] = repository.CustomGrantRow{RoleKey: "service_lead", Grant: domain.CustomGrant{
		ID: membershipID, TenantID: tenant, AccountID: invitedID, Scope: domain.HubScope(hubID), RoleID: customRoleID,
	}}
	if err := (DeleteRole{Roles: roles}).Execute(t.Context(), admin(), "service_lead"); err != nil {
		t.Fatal(err)
	}
	var removed []shared.ID
	for _, change := range log.changes {
		if change.Entity == RoleGrantEntity && change.Op == changelog.Delete {
			removed = append(removed, change.EntityID)
		}
	}
	if len(removed) != 1 || removed[0] != membershipID {
		t.Errorf("removed %v, want the role's grant", removed)
	}
}
