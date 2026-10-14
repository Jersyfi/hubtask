// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	ownerA     = shared.ID("01936f2a-7c1e-7000-8000-0000000001a1")
	ownerB     = shared.ID("01936f2a-7c1e-7000-8000-0000000001a2")
	ownerGrant = shared.ID("01936f2a-7c1e-7000-8000-0000000001a3")
	groupOwner = shared.ID("01936f2a-7c1e-7000-8000-0000000001a4")
)

// ownersFake is the workspace's OWNER paths, and whether the lock was taken before they were read.
type ownersFake struct {
	paths  []repository.OwnerPath
	locked bool
	reads  []bool
}

func (o *ownersFake) LockOwners(context.Context) error {
	o.locked = true
	return nil
}

func (o *ownersFake) OwnerPaths(context.Context) ([]repository.OwnerPath, error) {
	o.reads = append(o.reads, o.locked)
	return o.paths, nil
}

func workspaceOwnerGrant(t *testing.T, id, account, group shared.ID) domain.Grant {
	t.Helper()
	grant, err := domain.NewGrant(id, tenant, account, group, domain.TenantScope(), domain.RoleOwner)
	if err != nil {
		t.Fatalf("preparing the grant: %v", err)
	}
	return grant
}

func revokeGuarded(grants *grantStore, owners *ownersFake, proof *stepUpFake) RevokeMembership {
	handler := revokeHandler(grants, &authorizer{}, &auditSink{})
	handler.Owners = owners
	handler.StepUp = proof
	return handler
}

// #1429 (domain-model.md §3.2): the workspace keeps an owner. Revoking the last owner's only OWNER
// path - their own grant or the grant of the one group they hold it through - is refused before
// the proof is spent, and nothing is removed.
func TestTheLastOwnerIsNotRevoked(t *testing.T) {
	cases := []struct {
		name  string
		grant domain.Grant
		paths []repository.OwnerPath
	}{
		{
			name:  "their own grant",
			grant: workspaceOwnerGrant(t, ownerGrant, ownerA, ""),
			paths: []repository.OwnerPath{{MembershipID: ownerGrant, AccountID: ownerA}},
		},
		{
			name:  "the group's grant that every owner holds it through",
			grant: workspaceOwnerGrant(t, ownerGrant, "", groupID),
			paths: []repository.OwnerPath{
				{MembershipID: ownerGrant, AccountID: ownerA, GroupID: groupID},
				{MembershipID: ownerGrant, AccountID: ownerB, GroupID: groupID},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			grants, proof := newGrants(tc.grant), &stepUpFake{satisfied: true}
			err := revokeGuarded(grants, &ownersFake{paths: tc.paths}, proof).
				Execute(t.Context(), admin(), RevokeMembershipCommand{MembershipID: ownerGrant, StepUpToken: "hbt_sup_x"})

			if shared.AsError(err).DetailCode != "memberships.last_owner" || !errors.Is(err, shared.ErrConflict) {
				t.Fatalf("error %v, want memberships.last_owner", err)
			}
			if len(grants.revoked) != 0 {
				t.Errorf("revoked %v, want nothing", grants.revoked)
			}
			if len(proof.presented) != 0 {
				t.Errorf("the proof was spent on a refused revocation: %v", proof.presented)
			}
		})
	}
}

// Another owner left - in their own right, through a group, or the same person through a group -
// lets the revocation through, decided again under the owners' lock in the write.
func TestAnOwnerIsRevokedWhileAnotherRemains(t *testing.T) {
	cases := []struct {
		name  string
		paths []repository.OwnerPath
	}{
		{"another owner in their own right", []repository.OwnerPath{
			{MembershipID: ownerGrant, AccountID: ownerA}, {MembershipID: groupOwner, AccountID: ownerB},
		}},
		{"another owner through a group", []repository.OwnerPath{
			{MembershipID: ownerGrant, AccountID: ownerA}, {MembershipID: groupOwner, AccountID: ownerB, GroupID: groupID},
		}},
		{"the same person through a group", []repository.OwnerPath{
			{MembershipID: ownerGrant, AccountID: ownerA}, {MembershipID: groupOwner, AccountID: ownerA, GroupID: groupID},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			grants, owners := newGrants(workspaceOwnerGrant(t, ownerGrant, ownerA, "")), &ownersFake{paths: tc.paths}
			err := revokeGuarded(grants, owners, &stepUpFake{satisfied: true}).
				Execute(t.Context(), admin(), RevokeMembershipCommand{MembershipID: ownerGrant, StepUpToken: "hbt_sup_x"})
			if err != nil {
				t.Fatalf("revoking: %v", err)
			}
			if len(grants.revoked) != 1 {
				t.Errorf("revoked %v, want the grant", grants.revoked)
			}
			if len(owners.reads) != 2 || owners.reads[0] || !owners.reads[1] {
				t.Errorf("owners read %v, want once unlocked before the proof and once under the lock", owners.reads)
			}
		})
	}
}

// A workspace that had no owner who counts loses nothing a person held: a service account holding
// OWNER revokes it without the guard standing in the way.
func TestAWorkspaceWithoutACountingOwnerIsNotFrozen(t *testing.T) {
	grants := newGrants(workspaceOwnerGrant(t, ownerGrant, ownerA, ""))
	err := revokeGuarded(grants, &ownersFake{}, &stepUpFake{satisfied: true}).
		Execute(t.Context(), admin(), RevokeMembershipCommand{MembershipID: ownerGrant, StepUpToken: "hbt_sup_x"})
	if err != nil || len(grants.revoked) != 1 {
		t.Fatalf("revoking answered %v and removed %v, want the grant gone", err, grants.revoked)
	}
}

// Only OWNER on the workspace is guarded: a hub's owner and a workspace member are revoked without
// the owners being read, and an installation wired without the port still revokes them.
func TestOnlyTheWorkspacesOwnerIsGuarded(t *testing.T) {
	hubOwner, err := domain.NewGrant(ownerGrant, tenant, ownerA, "", domain.HubScope(hubID), domain.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	for _, grant := range []domain.Grant{hubOwner, existingGrant(t)} {
		grants := newGrants(grant)
		handler := revokeGuarded(grants, nil, &stepUpFake{satisfied: true})
		handler.Owners = nil
		if err := handler.Execute(t.Context(), admin(), RevokeMembershipCommand{MembershipID: grant.ID, StepUpToken: "hbt_sup_x"}); err != nil {
			t.Errorf("revoking %s at %s: %v", grant.Role, grant.Scope.Type, err)
		}
	}

	grants := newGrants(workspaceOwnerGrant(t, ownerGrant, ownerA, ""))
	handler := revokeGuarded(grants, nil, &stepUpFake{satisfied: true})
	handler.Owners = nil
	err = handler.Execute(t.Context(), admin(), RevokeMembershipCommand{MembershipID: ownerGrant, StepUpToken: "hbt_sup_x"})
	if shared.AsError(err).DetailCode != "memberships.owners_missing" || len(grants.revoked) != 0 {
		t.Errorf("unguarded workspace owner revocation answered %v, want memberships.owners_missing", err)
	}
}

func activePerson(id shared.ID) domain.Account {
	return domain.Account{ID: id, TenantID: tenant, Kind: domain.AccountUser, Status: domain.AccountActive}
}

// The owners' group: groupID holds OWNER on the workspace through membershipID, ownerA in it.
func ownersGroupWorld(t *testing.T) (*groupStore, *grantStore, *ownersFake) {
	t.Helper()
	groups := newGroups(settledGroup(t))
	groups.members[groupID] = []shared.ID{ownerA}
	owners := &ownersFake{paths: []repository.OwnerPath{{MembershipID: membershipID, AccountID: ownerA, GroupID: groupID}}}
	return groups, groupHolding(t, domain.RoleOwner), owners
}

// #1429 at the group doors: taking the last owners out of the group that holds OWNER on the
// workspace, without putting a counting person in, is refused; a handover to a person who counts
// passes; an invited account does not count.
func TestTheLastOwnersAreNotTakenOutOfTheirGroup(t *testing.T) {
	cases := []struct {
		name    string
		members []shared.ID
		refused bool
	}{
		{"emptied", nil, true},
		{"handed to an invitation", []shared.ID{invitedID}, true},
		{"handed to a person", []shared.ID{ownerB}, false},
		{"another person joins", []shared.ID{ownerA, ownerB}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			groups, grants, owners := ownersGroupWorld(t)
			handler := updateHandler(groups, newAccounts(invitedAccount(t), activePerson(ownerA), activePerson(ownerB)), &authorizer{}, &auditSink{})
			handler.Grants, handler.Owners, handler.StepUp = grants, owners, &stepUpFake{satisfied: true}

			_, err := handler.Execute(t.Context(), admin(), UpdateGroupCommand{
				GroupID: groupID, Members: tc.members, ReplaceMembers: true, StepUpToken: "hbt_sup_x",
			})

			if tc.refused {
				if shared.AsError(err).DetailCode != "memberships.last_owner" {
					t.Fatalf("error %v, want memberships.last_owner", err)
				}
				if !slices.Equal(groups.members[groupID], []shared.ID{ownerA}) {
					t.Errorf("members %v, want the group unchanged", groups.members[groupID])
				}
				return
			}
			if err != nil {
				t.Fatalf("changing the members: %v", err)
			}
			if len(owners.reads) != 2 || !owners.reads[1] {
				t.Errorf("owners read %v, want the decision under the lock", owners.reads)
			}
		})
	}
}

// Deleting the group the last owners hold OWNER through is refused; with another owner left it
// goes.
func TestTheLastOwnersGroupIsNotDeleted(t *testing.T) {
	groups, grants, owners := ownersGroupWorld(t)
	handler := deleteHandler(groups, &authorizer{}, &auditSink{})
	proof := &stepUpFake{satisfied: true}
	handler.Grants, handler.Owners, handler.StepUp = grants, owners, proof

	err := handler.Execute(t.Context(), admin(), DeleteGroupCommand{GroupID: groupID, StepUpToken: "hbt_sup_x"})
	if shared.AsError(err).DetailCode != "memberships.last_owner" || len(groups.deleted) != 0 {
		t.Fatalf("deleting answered %v and deleted %v, want memberships.last_owner and nothing", err, groups.deleted)
	}
	if len(proof.presented) != 0 {
		t.Errorf("the proof was spent on a refused deletion: %v", proof.presented)
	}

	owners.paths = append(owners.paths, repository.OwnerPath{MembershipID: ownerGrant, AccountID: ownerB})
	if err := handler.Execute(t.Context(), admin(), DeleteGroupCommand{GroupID: groupID, StepUpToken: "hbt_sup_x"}); err != nil {
		t.Fatalf("deleting with another owner left: %v", err)
	}
	if len(groups.deleted) != 1 {
		t.Errorf("deleted %v, want the group", groups.deleted)
	}
}

// A group holding OWNER on a hub only is not the workspace's owners: no count is read.
func TestAGroupOwningAHubIsNotCountedAsTheWorkspacesOwners(t *testing.T) {
	held, err := domain.NewGrant(membershipID, tenant, "", groupID, domain.HubScope(hubID), domain.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	groups := newGroups(settledGroup(t))
	groups.members[groupID] = []shared.ID{ownerA}
	handler := deleteHandler(groups, &authorizer{}, &auditSink{})
	handler.Grants, handler.StepUp = newGrants(held), &stepUpFake{satisfied: true}
	if err := handler.Execute(t.Context(), admin(), DeleteGroupCommand{GroupID: groupID, StepUpToken: "hbt_sup_x"}); err != nil {
		t.Fatalf("deleting a hub's owners' group: %v", err)
	}
}

// #1430 (identity.md §16.3): changing who is in a group that holds OWNER anywhere, or deleting it,
// changes who holds OWNER, and demands the fresh proof - after the ceiling and the guard, before
// anything is written. A rename, an unchanged list and a group without OWNER ask none.
func TestAGroupHoldingOwnerAsksAProof(t *testing.T) {
	hubOwner, err := domain.NewGrant(membershipID, tenant, "", groupID, domain.HubScope(hubID), domain.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	world := func(held domain.Grant) (*groupStore, *grantStore, *ownersFake) {
		groups := newGroups(settledGroup(t))
		groups.members[groupID] = []shared.ID{ownerA}
		owners := &ownersFake{paths: []repository.OwnerPath{{MembershipID: ownerGrant, AccountID: ownerB}}}
		return groups, newGrants(held), owners
	}
	accounts := newAccounts(activePerson(ownerA), activePerson(ownerB))
	rename := "Owners"

	for _, held := range []domain.Grant{workspaceOwnerGrant(t, membershipID, "", groupID), hubOwner} {
		t.Run(string(held.Scope.Type), func(t *testing.T) {
			groups, grants, owners := world(held)
			update := updateHandler(groups, accounts, &authorizer{}, &auditSink{})
			update.Grants, update.Owners, update.StepUp = grants, owners, &stepUpFake{}
			change := UpdateGroupCommand{GroupID: groupID, Members: []shared.ID{ownerA, ownerB}, ReplaceMembers: true}

			_, err := update.Execute(t.Context(), admin(), change)
			if shared.AsError(err).DetailCode != "auth.step_up_required" {
				t.Fatalf("a member change without a proof answered %v, want auth.step_up_required", err)
			}
			if len(groups.members[groupID]) != 1 {
				t.Errorf("members %v, want nothing written", groups.members[groupID])
			}

			proof := &stepUpFake{satisfied: true}
			update.StepUp = proof
			change.StepUpToken = "hbt_sup_x"
			if _, err := update.Execute(t.Context(), admin(), change); err != nil {
				t.Fatalf("a member change with the proof: %v", err)
			}
			if !slices.Equal(proof.presented, []string{"hbt_sup_x"}) {
				t.Errorf("proofs presented %v, want the one sent", proof.presented)
			}

			// The list now unchanged, beside a rename: nothing placed, no proof.
			update.StepUp = &stepUpFake{}
			change = UpdateGroupCommand{GroupID: groupID, Name: &rename, Members: []shared.ID{ownerA, ownerB}, ReplaceMembers: true}
			if _, err := update.Execute(t.Context(), admin(), change); err != nil {
				t.Errorf("an unchanged list beside a rename asked: %v", err)
			}

			remove := deleteHandler(groups, &authorizer{}, &auditSink{})
			remove.Grants, remove.Owners, remove.StepUp = grants, owners, &stepUpFake{}
			err = remove.Execute(t.Context(), admin(), DeleteGroupCommand{GroupID: groupID})
			if shared.AsError(err).DetailCode != "auth.step_up_required" || len(groups.deleted) != 0 {
				t.Fatalf("a deletion without a proof answered %v, want auth.step_up_required and nothing deleted", err)
			}
		})
	}

	// A group holding less than OWNER asks no proof.
	groups, grants, owners := world(func() domain.Grant {
		g, err := domain.NewGrant(membershipID, tenant, "", groupID, domain.TenantScope(), domain.RoleAdmin)
		if err != nil {
			t.Fatal(err)
		}
		return g
	}())
	update := updateHandler(groups, accounts, &authorizer{}, &auditSink{})
	update.Grants, update.Owners, update.StepUp = grants, owners, &stepUpFake{}
	if _, err := update.Execute(t.Context(), admin(), UpdateGroupCommand{
		GroupID: groupID, Members: []shared.ID{ownerB}, ReplaceMembers: true,
	}); err != nil {
		t.Errorf("a member change of a group holding ADMIN asked: %v", err)
	}
}
