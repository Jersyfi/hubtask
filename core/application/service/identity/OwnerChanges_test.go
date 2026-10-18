// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ownerChangesFake counts what the platform was told.
type ownerChangesFake struct{ told []event.Actor }

func (o *ownerChangesFake) OwnersChanged(_ context.Context, actor event.Actor, _ time.Time) error {
	o.told = append(o.told, actor)
	return nil
}

// An OWNER grant on the workspace is an owner changed; a lesser role, or OWNER of a hub, is not
// (UC-INS-14).
func TestAnOwnerGrantedIsTold(t *testing.T) {
	cases := []struct {
		name  string
		scope domain.Scope
		role  domain.Role
		told  int
	}{
		{"OWNER of the workspace", domain.TenantScope(), domain.RoleOwner, 1},
		{"a member of the workspace", domain.TenantScope(), domain.RoleMember, 0},
		{"OWNER of a hub", domain.HubScope(hubID), domain.RoleOwner, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			platform := &ownerChangesFake{}
			handler := grantHandler(newGrants(), newAccounts(invitedAccount(t)), newGroups(), &authorizer{}, &auditSink{})
			handler.StepUp, handler.Platform = &stepUpFake{satisfied: true}, platform
			if _, err := handler.Execute(t.Context(), admin(), GrantMembershipCommand{
				AccountID: invitedID, Scope: tc.scope, Role: tc.role, StepUpToken: "hbt_sup_x",
			}); err != nil {
				t.Fatalf("granting: %v", err)
			}
			if len(platform.told) != tc.told {
				t.Errorf("told %d times, want %d", len(platform.told), tc.told)
			}
			if tc.told == 1 && platform.told[0].ID != admin().AccountID {
				t.Errorf("told as %+v", platform.told[0])
			}
		})
	}
}

func TestAnOwnerRevokedIsTold(t *testing.T) {
	platform := &ownerChangesFake{}
	owners := &ownersFake{paths: []repository.OwnerPath{
		{MembershipID: ownerGrant, AccountID: ownerA}, {MembershipID: groupOwner, AccountID: ownerB},
	}}
	handler := revokeGuarded(newGrants(workspaceOwnerGrant(t, ownerGrant, ownerA, "")), owners, &stepUpFake{satisfied: true})
	handler.Platform = platform
	if err := handler.Execute(t.Context(), admin(), RevokeMembershipCommand{MembershipID: ownerGrant, StepUpToken: "hbt_sup_x"}); err != nil {
		t.Fatalf("revoking: %v", err)
	}
	if len(platform.told) != 1 {
		t.Errorf("told %d times, want once", len(platform.told))
	}

	member := revokeHandler(newGrants(existingGrant(t)), &authorizer{}, &auditSink{})
	member.Platform = platform
	if err := member.Execute(t.Context(), admin(), RevokeMembershipCommand{MembershipID: membershipID}); err != nil {
		t.Fatal(err)
	}
	if len(platform.told) != 1 {
		t.Errorf("a member's revocation was told as an owner changed")
	}
}

// Who is in a group holding OWNER on the workspace is who the owners are: a changed member list and
// the group's deletion are told, an unchanged list is not.
func TestAGroupHoldingOwnerChangedIsTold(t *testing.T) {
	groups, grants, owners := ownersGroupWorld(t)
	owners.paths = append(owners.paths, repository.OwnerPath{MembershipID: ownerGrant, AccountID: ownerB})
	platform := &ownerChangesFake{}
	accounts := newAccounts(invitedAccount(t), activePerson(ownerA), activePerson(ownerB))
	update := updateHandler(groups, accounts, &authorizer{}, &auditSink{})
	update.Grants, update.Owners, update.StepUp, update.Platform = grants, owners, &stepUpFake{satisfied: true}, platform

	same := []shared.ID{ownerA}
	if _, err := update.Execute(t.Context(), admin(), UpdateGroupCommand{
		GroupID: groupID, Members: same, ReplaceMembers: true, StepUpToken: "hbt_sup_x",
	}); err != nil {
		t.Fatalf("resending the members: %v", err)
	}
	if len(platform.told) != 0 {
		t.Fatalf("an unchanged member list was told as an owner changed")
	}
	if _, err := update.Execute(t.Context(), admin(), UpdateGroupCommand{
		GroupID: groupID, Members: []shared.ID{ownerA, ownerB}, ReplaceMembers: true, StepUpToken: "hbt_sup_x",
	}); err != nil {
		t.Fatalf("changing the members: %v", err)
	}
	if len(platform.told) != 1 {
		t.Fatalf("a changed member list told %d times, want once", len(platform.told))
	}

	remove := deleteHandler(groups, &authorizer{}, &auditSink{})
	remove.Grants, remove.Owners, remove.StepUp, remove.Platform = grants, owners, &stepUpFake{satisfied: true}, platform
	if err := remove.Execute(t.Context(), admin(), DeleteGroupCommand{GroupID: groupID, StepUpToken: "hbt_sup_x"}); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if len(platform.told) != 2 {
		t.Errorf("deleting the owners' group told %d times in all, want 2", len(platform.told))
	}
}

// A managed account placed as the workspace's owner is an owner changed too: its creation records
// its grant through the grant's own path. One placed lower is not.
func TestAManagedOwnerIsTold(t *testing.T) {
	f := newManagedFixture(t)
	in := lenaInput()
	in["role"], in["scope_type"], in["step_up_token"] = "OWNER", "TENANT", "hbt_sup_x"
	delete(in, "scope_id")
	if _, err := f.create(t, admin(), in); err != nil {
		t.Fatalf("creating: %v", err)
	}
	if len(f.platform.told) != 1 {
		t.Errorf("a managed owner was told %d times, want once", len(f.platform.told))
	}

	lower := newManagedFixture(t)
	if _, err := lower.create(t, admin(), lenaInput()); err != nil {
		t.Fatal(err)
	}
	if len(lower.platform.told) != 0 {
		t.Error("a managed contributor was told as an owner changed")
	}
}

// A group's OWNER grant on the workspace changes nobody's role while the group is empty.
func TestAnEmptyGroupMadeOwnerIsNotTold(t *testing.T) {
	platform := &ownerChangesFake{}
	groups := newGroups(settledGroup(t))
	handler := grantHandler(newGrants(), newAccounts(), groups, &authorizer{}, &auditSink{})
	handler.StepUp, handler.Platform = &stepUpFake{satisfied: true}, platform
	if _, err := handler.Execute(t.Context(), admin(), GrantMembershipCommand{
		GroupID: groupID, Scope: domain.TenantScope(), Role: domain.RoleOwner, StepUpToken: "hbt_sup_x",
	}); err != nil {
		t.Fatalf("granting: %v", err)
	}
	if len(platform.told) != 0 {
		t.Error("an empty group's grant was told as an owner changed")
	}
}
