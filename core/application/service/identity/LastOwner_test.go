// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
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
