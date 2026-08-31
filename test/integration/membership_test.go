// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

var (
	membershipHub   = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000b1")
	membershipGroup = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000b9")
	tenantGrantA    = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000d1")
	hubGrantA       = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000d2")
	tenantGrantB    = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000d3")
)

// seedMemberships gives Anna an administrator role at the tenant, Bert the same in his own
// tenant, and - through a group - the owner role on one hub.
func seedMemberships(ctx context.Context, t *testing.T) {
	t.Helper()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)

	group := membershipGroup
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account_group (id, tenant_id, name) VALUES ($1, $2, 'Leads')
		  ON CONFLICT (id) DO NOTHING`, []any{group.String(), tenantA.String()}},
		{`INSERT INTO account_group_member (tenant_id, group_id, account_id) VALUES ($1, $2, $3)
		  ON CONFLICT DO NOTHING`, []any{tenantA.String(), group.String(), authorA.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		  VALUES ($1, $2, $3, 'TENANT', 'ADMIN') ON CONFLICT (id) DO NOTHING`,
			[]any{tenantGrantA.String(), tenantA.String(), authorA.String()}},
		{`INSERT INTO membership (id, tenant_id, group_id, scope_type, scope_id, role)
		  VALUES ($1, $2, $3, 'HUB', $4, 'OWNER') ON CONFLICT (id) DO NOTHING`,
			[]any{hubGrantA.String(), tenantA.String(), group.String(), membershipHub.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		  VALUES ($1, $2, $3, 'TENANT', 'OWNER') ON CONFLICT (id) DO NOTHING`,
			[]any{tenantGrantB.String(), tenantB.String(), authorB.String()}},
	}

	for _, statement := range statements {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding memberships: %v", err)
		}
	}
}

func membershipsAlong(ctx context.Context, t *testing.T, tenant, account shared.ID, path []identity.Scope) []identity.Membership {
	t.Helper()

	var memberships []identity.Membership
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		memberships, err = postgres.NewMembershipRepository().Along(ctx, account, path)
		return err
	}); err != nil {
		t.Fatalf("reading the memberships: %v", err)
	}
	return memberships
}

func TestARoleAtTheTenantIsFound(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	path := []identity.Scope{identity.TenantScope(), identity.HubScope(membershipHub)}
	memberships := membershipsAlong(ctx, t, tenantA, authorA, path)

	role, found := service.EffectiveRole(memberships, path)
	if !found {
		t.Fatalf("no role at all: %+v", memberships)
	}
	// The group grants OWNER on the hub, which outranks the tenant-wide ADMIN.
	if role != identity.RoleOwner {
		t.Errorf("role %s, want OWNER - a right held through a group is not a lesser right", role)
	}
	if !service.Allows(memberships, path, service.PermissionDeleteContainer) {
		t.Error("the owner may not delete a container")
	}
}

// A right held through a group is a right of the account: whether it was granted directly is not
// a distinction the resolution makes.
func TestAGroupMembershipReachesItsMembers(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	path := []identity.Scope{identity.HubScope(membershipHub)}
	memberships := membershipsAlong(ctx, t, tenantA, authorA, path)

	if len(memberships) == 0 {
		t.Fatal("the group's membership did not reach its member")
	}
	for _, membership := range memberships {
		if membership.Scope.Type == identity.ScopeHub && membership.Role != identity.RoleOwner {
			t.Errorf("the hub membership came back as %s", membership.Role)
		}
	}
}

// The cross-tenant negative test for Along (gate SG-3): another tenant's memberships are not
// visible, even when the account identifier is known.
func TestMembershipsOfAnotherTenantAreInvisible(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	path := []identity.Scope{identity.TenantScope(), identity.HubScope(membershipHub)}

	// Tenant B asks about tenant A's account - the identifier is no secret, and it must not help.
	memberships := membershipsAlong(ctx, t, tenantB, authorA, path)
	if len(memberships) != 0 {
		t.Errorf("tenant B read %d of tenant A's memberships: %+v", len(memberships), memberships)
	}

	// And the resolution then refuses, rather than falling back to some default.
	if service.Allows(memberships, path, service.PermissionRead) {
		t.Error("an account with no visible membership was allowed to read")
	}
}

// The query is bounded by the path: a membership somewhere else in the tenant is not returned,
// which is what keeps a permission check cheap for an account with many of them.
func TestOnlyWhatCouldApplyToThePathComesBack(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	elsewhere := freshID(t)
	path := []identity.Scope{identity.HubScope(elsewhere)}

	for _, membership := range membershipsAlong(ctx, t, tenantA, authorA, path) {
		if membership.Scope.Type == identity.ScopeHub {
			t.Errorf("a membership on another hub came back: %+v", membership)
		}
	}
}

// The read half of C-04: a membership on an entry is what "shared with me" means, and the query
// answers which entries of one collection an account holds one on.
func TestSharedEntriesOfACollectionComeBack(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)

	sharedItem, unsharedItem := freshID(t), freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		items := itemRepo()
		if err := items.Insert(ctx, taskIn(tenantA, authorA, collection, sharedItem, "Shared", "a0")); err != nil {
			return err
		}
		return items.Insert(ctx, taskIn(tenantA, authorA, collection, unsharedItem, "Not shared", "a1"))
	}); err != nil {
		t.Fatalf("seeding the entries: %v", err)
	}
	shareItem(ctx, t, tenantA, authorA, sharedItem)

	shares := sharedItemsIn(ctx, t, tenantA, authorA, collection)
	if len(shares) != 1 || shares[0] != sharedItem {
		t.Fatalf("the shares came back as %+v, want only the entry that was shared", shares)
	}

	// The entry beside it is not in the answer, which is the whole of "shared items only": it needs
	// no rule, only the scope the membership was granted at.
	for _, id := range shares {
		if id == unsharedItem {
			t.Error("an entry nobody was given reached the answer")
		}
	}
}

// The query is bounded to the collection it was asked about: a share elsewhere in the tenant is
// not part of this level.
func TestASharedEntryInAnotherCollectionIsNotInTheLevel(t *testing.T) {
	ctx := context.Background()
	here := collectionFor(ctx, t, tenantA, authorA)
	elsewhere := collectionFor(ctx, t, tenantA, authorA)

	item := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, taskIn(tenantA, authorA, elsewhere, item, "Over there", "a0"))
	}); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}
	shareItem(ctx, t, tenantA, authorA, item)

	if shares := sharedItemsIn(ctx, t, tenantA, authorA, here); len(shares) != 0 {
		t.Errorf("a share in another collection reached this level: %+v", shares)
	}
}

// The cross-tenant negative test for SharedItemsIn (gate SG-3): another tenant's shares are not
// visible, even when both identifiers are known.
func TestSharedEntriesOfAnotherTenantAreInvisible(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)

	item := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, taskIn(tenantA, authorA, collection, item, "Shared", "a0"))
	}); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}
	shareItem(ctx, t, tenantA, authorA, item)

	// Tenant B asks about tenant A's account and tenant A's collection - neither identifier is a
	// secret, and neither must help.
	if shares := sharedItemsIn(ctx, t, tenantB, authorA, collection); len(shares) != 0 {
		t.Errorf("tenant B read %d of tenant A's shares: %+v", len(shares), shares)
	}
}

// shareItem grants the account a guest role on one entry - the membership that is a share.
func shareItem(ctx context.Context, t *testing.T, tenant, account, item shared.ID) {
	t.Helper()

	_, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		 VALUES ($1, $2, $3, 'ITEM', $4, 'GUEST') ON CONFLICT (id) DO NOTHING`,
		freshID(t).String(), tenant.String(), account.String(), item.String())
	if err != nil {
		t.Fatalf("sharing the entry: %v", err)
	}
}

func sharedItemsIn(
	ctx context.Context, t *testing.T, tenant, account, collection shared.ID,
) []shared.ID {
	t.Helper()

	var shares []shared.ID
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		shares, err = postgres.NewMembershipRepository().SharedItemsIn(ctx, account, collection)
		return err
	}); err != nil {
		t.Fatalf("reading the shared entries: %v", err)
	}
	return shares
}

// administratorsAlong is the reverse question against a real database (R-1, G-12).
func administratorsAlong(
	ctx context.Context, t *testing.T, tenant shared.ID, path []identity.Scope,
) []shared.ID {
	t.Helper()

	var administrators []shared.ID
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		administrators, err = postgres.NewMembershipRepository().Administrators(ctx, path)
		return err
	}); err != nil {
		t.Fatalf("reading the administrators: %v", err)
	}
	return administrators
}

// Who administers, directly and through a group: the retention warning's audience, resolved the
// way a permission check resolves what one account holds.
func TestTheAdministratorsOfAPathAreFound(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	// Anna holds ADMIN at the tenant directly, and OWNER on the hub through the group she is in.
	// Both are administrators, and she is one person.
	administrators := administratorsAlong(ctx, t, tenantA,
		[]identity.Scope{identity.TenantScope(), identity.HubScope(membershipHub)})

	found := 0
	for _, id := range administrators {
		if id == authorA {
			found++
		}
	}
	if found != 1 {
		t.Errorf("Anna appears %d times in %v, want once", found, administrators)
	}
}

// The cross-tenant negative test for Administrators (gate SG-3): one tenant's administrators are
// not another tenant's, and the answer is empty rather than somebody else's.
func TestTheAdministratorsOfAnotherTenantAreInvisible(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	// Tenant B has one owner of its own and must see no more than that.
	administrators := administratorsAlong(ctx, t, tenantB,
		[]identity.Scope{identity.TenantScope(), identity.HubScope(membershipHub)})

	for _, id := range administrators {
		if id == authorA {
			t.Errorf("tenant B read tenant A's administrator: %v", administrators)
		}
	}
	if len(administrators) != 1 || administrators[0] != authorB {
		t.Errorf("tenant B's administrators are %v, want its own owner", administrators)
	}
}

// A role that is not an administrator is not in the answer. Who may stop a retention rule is the
// role matrix's decision, and a resolution that returned viewers would be warning everybody.
func TestOnlyAdministeringRolesAreInTheAnswer(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	viewer := freshID(t)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Vera Viewer')
		 ON CONFLICT (id) DO NOTHING`, viewer.String(), tenantA.String()); err != nil {
		t.Fatalf("seeding the viewer: %v", err)
	}
	if _, err := admin.Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		 VALUES ($1, $2, $3, 'TENANT', 'VIEWER')`,
		freshID(t).String(), tenantA.String(), viewer.String()); err != nil {
		t.Fatalf("granting the viewer role: %v", err)
	}

	for _, id := range administratorsAlong(ctx, t, tenantA, []identity.Scope{identity.TenantScope()}) {
		if id == viewer {
			t.Error("a VIEWER was named as an administrator")
		}
	}
}
