// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The read behind the members screen (F3-01), against the real boundary. Gate SG-3: ListAt has
// its cross-tenant negative below.

// grantedAt writes one membership for a fresh account at the scope and returns it.
func grantedAt(ctx context.Context, t *testing.T, tenant shared.ID, scope identity.Scope) identity.Grant {
	t.Helper()
	account, err := identity.Invite(freshID(t), tenant, freshEmail(t), "Anna")
	if err != nil {
		t.Fatalf("building the account: %v", err)
	}
	grant, err := identity.NewGrant(freshID(t), tenant, account.ID, "", scope, identity.RoleMember)
	if err != nil {
		t.Fatalf("building the grant: %v", err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		if err := postgres.NewAccountRepository().Insert(ctx, account); err != nil {
			return err
		}
		return postgres.NewMembershipGrantRepository(pageCursors()).Grant(ctx, grant)
	}); err != nil {
		t.Fatalf("granting: %v", err)
	}
	return grant
}

func membershipsAt(ctx context.Context, t *testing.T, tenant shared.ID, scope identity.Scope, page repository.Page) repository.GrantPage {
	t.Helper()
	var listed repository.GrantPage
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		listed, err = postgres.NewMembershipGrantRepository(pageCursors()).ListAt(ctx, scope, page)
		return err
	}); err != nil {
		t.Fatalf("listing the memberships: %v", err)
	}
	return listed
}

func grantIDs(grants []identity.Grant) map[shared.ID]bool {
	ids := make(map[shared.ID]bool, len(grants))
	for _, grant := range grants {
		ids[grant.ID] = true
	}
	return ids
}

// What is granted at the hub comes back, and what is granted at another hub or at the workspace
// does not - the list is the scope's own, not what is in force there.
func TestTheMembershipsAtAScopeAreListedAndNothingGrantedElsewhere(t *testing.T) {
	ctx := context.Background()
	hub := hubFor(ctx, t, tenantA, authorA)
	otherHub := hubFor(ctx, t, tenantA, authorA)

	here := grantedAt(ctx, t, tenantA, identity.HubScope(hub))
	elsewhere := grantedAt(ctx, t, tenantA, identity.HubScope(otherHub))
	workspace := grantedAt(ctx, t, tenantA, identity.TenantScope())

	listed := grantIDs(membershipsAt(ctx, t, tenantA, identity.HubScope(hub), repository.Page{Size: 50}).Grants)
	if !listed[here.ID] {
		t.Errorf("the grant at the hub is missing from %v", listed)
	}
	if listed[elsewhere.ID] || listed[workspace.ID] {
		t.Errorf("a grant made elsewhere was listed at the hub: %v", listed)
	}

	// The workspace scope is matched by its type alone, and it holds the tenant-wide grant.
	atWorkspace := grantIDs(membershipsAt(ctx, t, tenantA, identity.TenantScope(), repository.Page{Size: 200}).Grants)
	if !atWorkspace[workspace.ID] || atWorkspace[here.ID] {
		t.Errorf("the workspace scope listed %v", atWorkspace)
	}
}

// The walk: newest first, one page after another, every grant exactly once, and the last page
// says so.
func TestTheMembershipsAtAScopeArePagedNewestFirst(t *testing.T) {
	ctx := context.Background()
	hub := hubFor(ctx, t, tenantA, authorA)
	scope := identity.HubScope(hub)

	var granted []identity.Grant
	for range 3 {
		granted = append(granted, grantedAt(ctx, t, tenantA, scope))
	}

	var walked []identity.Grant
	cursor := ""
	for pages := 0; ; pages++ {
		page := membershipsAt(ctx, t, tenantA, scope, repository.Page{Cursor: cursor, Size: 2})
		walked = append(walked, page.Grants...)
		if !page.Info.HasMore {
			if page.Info.NextCursor != "" {
				t.Error("the last page carries a cursor")
			}
			break
		}
		if page.Info.NextCursor == "" || pages > 3 {
			t.Fatalf("the walk does not end: page %d has more and cursor %q", pages, page.Info.NextCursor)
		}
		cursor = page.Info.NextCursor
	}

	if len(walked) != len(granted) {
		t.Fatalf("walked %d grants, want %d", len(walked), len(granted))
	}
	for i := range granted {
		// Newest first: the last grant made is the first listed.
		if walked[i].ID != granted[len(granted)-1-i].ID {
			t.Errorf("position %d is %s, want %s", i, walked[i].ID, granted[len(granted)-1-i].ID)
		}
	}
}

// The cross-tenant negative for ListAt (gate SG-3): another tenant's grants at the same scope
// identifier are not there, and the identifier being known does not help.
func TestTheMembershipsOfAnotherTenantAreNotListed(t *testing.T) {
	ctx := context.Background()
	hub := hubFor(ctx, t, tenantA, authorA)
	grant := grantedAt(ctx, t, tenantA, identity.HubScope(hub))

	foreign := grantIDs(membershipsAt(ctx, t, tenantB, identity.HubScope(hub), repository.Page{Size: 50}).Grants)
	if len(foreign) != 0 {
		t.Errorf("tenant B listed %v at tenant A's hub", foreign)
	}
	atWorkspace := grantIDs(membershipsAt(ctx, t, tenantB, identity.TenantScope(), repository.Page{Size: 200}).Grants)
	if atWorkspace[grant.ID] {
		t.Error("tenant B listed tenant A's grant at its own workspace scope")
	}
}
