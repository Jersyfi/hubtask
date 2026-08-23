// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package service_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

var somebodyElse = shared.MustParseID("0192f000-0000-7000-8000-00000000000f")

// The per-entry half of the matrix in domain-model.md §3.2, read as a table. The two rows that
// matter are the ones no permission name can carry: a contributor changes only what is assigned to
// them, and a guest comments on what it may not change.
func TestThePerEntryMatrix(t *testing.T) {
	cases := []struct {
		role   identity.Role
		action service.ItemAction
		want   service.ItemAccess
	}{
		{identity.RoleOwner, service.ItemChange, service.AccessAll},
		{identity.RoleAdmin, service.ItemChange, service.AccessAll},
		{identity.RoleMember, service.ItemChange, service.AccessAll},

		{identity.RoleContributor, service.ItemRead, service.AccessAll},
		{identity.RoleContributor, service.ItemCreate, service.AccessAll},
		{identity.RoleContributor, service.ItemChange, service.AccessAssigned},
		{identity.RoleContributor, service.ItemComment, service.AccessAssigned},

		{identity.RoleViewer, service.ItemRead, service.AccessAll},
		{identity.RoleViewer, service.ItemCreate, service.AccessNone},
		{identity.RoleViewer, service.ItemChange, service.AccessNone},
		{identity.RoleViewer, service.ItemComment, service.AccessNone},

		{identity.RoleGuest, service.ItemRead, service.AccessAll},
		{identity.RoleGuest, service.ItemCreate, service.AccessNone},
		{identity.RoleGuest, service.ItemChange, service.AccessNone},
		{identity.RoleGuest, service.ItemComment, service.AccessAll},
	}

	for _, c := range cases {
		if got := service.ItemAccessOf(c.role, c.action); got != c.want {
			t.Errorf("%s on %s: %s, want %s", c.role, c.action, got, c.want)
		}
	}

	// Every defined role sees something, and every kind of access is answered for every role: a
	// missing cell would come back as AccessNone and look like a decision.
	for _, role := range identity.Roles() {
		for _, action := range service.ItemActions() {
			if service.ItemAccessOf(role, action) == "" {
				t.Errorf("%s has no answer for %s", role, action)
			}
		}
		if service.ItemAccessOf(role, service.ItemRead) != service.AccessAll {
			t.Errorf("%s cannot read an entry it holds a role on", role)
		}
	}

	// A role this build does not know reaches nothing, rather than everything.
	for _, action := range service.ItemActions() {
		if got := service.ItemAccessOf("ARCHITECT", action); got != service.AccessNone {
			t.Errorf("an unknown role may %s: %s", action, got)
		}
	}
}

// The narrowing itself: the same role, the same action, and two entries.
func TestAContributorReachesOnlyWhatIsAssignedToThem(t *testing.T) {
	cases := []struct {
		name     string
		action   service.ItemAction
		assignee shared.ID
		want     service.ItemVerdict
	}{
		{"their own entry", service.ItemChange, account, service.ItemPermitted},
		{"somebody else's", service.ItemChange, somebodyElse, service.ItemRefusedByAssignment},
		{"nobody's", service.ItemChange, shared.ID(""), service.ItemRefusedByAssignment},
		{"commenting on their own", service.ItemComment, account, service.ItemPermitted},
		{"commenting on another's", service.ItemComment, somebodyElse, service.ItemRefusedByAssignment},
		// Nothing is assigned yet, and the entry will be theirs: the create is the one action a
		// contributor performs on an entry that is nobody's.
		{"creating", service.ItemCreate, shared.ID(""), service.ItemPermitted},
		{"reading another's", service.ItemRead, somebodyElse, service.ItemPermitted},
	}

	for _, c := range cases {
		got := service.AllowsItemAction(identity.RoleContributor, c.action, account, c.assignee)
		if got != c.want {
			t.Errorf("%s: verdict %d, want %d", c.name, got, c.want)
		}
	}
}

// The refusals are told apart, because the trail records which one happened even though the client
// is told the same thing either way.
func TestARefusalSaysWhichNarrowingRefused(t *testing.T) {
	byRole := service.AllowsItemAction(identity.RoleGuest, service.ItemChange, account, account)
	if byRole != service.ItemRefusedByRole {
		t.Errorf("a guest changing an entry assigned to it: verdict %d, want refused by role", byRole)
	}

	byAssignment := service.AllowsItemAction(
		identity.RoleContributor, service.ItemChange, account, somebodyElse)
	if byAssignment != service.ItemRefusedByAssignment {
		t.Errorf("a contributor on another's entry: verdict %d, want refused by assignment", byAssignment)
	}

	// The cell the matrix widens: a guest carries no WRITE_ITEMS and comments all the same, which
	// is why the per-entry decision replaces the permission rather than sitting on top of it.
	if service.RoleAllows(identity.RoleGuest, service.PermissionWriteItems) {
		t.Fatal("a guest carries WRITE_ITEMS after all; the widening below would be redundant")
	}
	if got := service.AllowsItemAction(
		identity.RoleGuest, service.ItemComment, account, somebodyElse,
	); got != service.ItemPermitted {
		t.Errorf("a guest commenting: verdict %d, want permitted", got)
	}
}

// An entry's own scope is the bottom of the path, and it is what makes a share resolvable.
func TestAShareResolvesThroughTheEntrysOwnScope(t *testing.T) {
	item := shared.MustParseID("0192f000-0000-7000-8000-000000000010")
	neighbour := shared.MustParseID("0192f000-0000-7000-8000-000000000011")
	pathTo := func(id shared.ID) []identity.Scope {
		return append(append([]identity.Scope{}, collectionPth...), identity.ItemScope(id))
	}

	share := []identity.Membership{grant(identity.ItemScope(item), identity.RoleGuest)}
	role, found := service.EffectiveRole(share, pathTo(item))
	if !found || role != identity.RoleGuest {
		t.Errorf("a share on the entry resolves to %q (found %v), want GUEST", role, found)
	}

	// The same membership says nothing about the entry beside it: that is the whole of "shared
	// items only", and it needs no rule of its own.
	if _, found := service.EffectiveRole(share, pathTo(neighbour)); found {
		t.Error("a share on one entry reached another")
	}
}
