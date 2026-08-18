// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// freshEmail keeps the tests independent of each other: the address is unique per tenant, and a
// suite that reused one would pass or fail depending on what ran before it.
func freshEmail(t *testing.T) string {
	t.Helper()
	return freshID(t).String() + "@example.org"
}

func invitedIn(t *testing.T, tenant shared.ID) identity.Account {
	t.Helper()
	account, err := identity.Invite(freshID(t), tenant, freshEmail(t), "Anna")
	if err != nil {
		t.Fatalf("building the account: %v", err)
	}
	if err := write(context.Background(), t, tenant, func(ctx context.Context) error {
		return postgres.NewAccountRepository().Insert(ctx, account)
	}); err != nil {
		t.Fatalf("writing the account: %v", err)
	}
	return account
}

func TestAnAccountIsFoundByIdentifierAndByAddress(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := invitedIn(t, tenantA)

	var byID, byEmail identity.Account
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		accounts := postgres.NewAccountRepository()
		var err error
		if byID, err = accounts.Find(ctx, account.ID); err != nil {
			return err
		}
		// Upper case on the way in: the index compares lower case, and so must the lookup.
		byEmail, err = accounts.FindByEmail(ctx, account.Email)
		return err
	}); err != nil {
		t.Fatalf("reading the account: %v", err)
	}

	if byID.Email != account.Email || byID.Status != identity.AccountInvited {
		t.Errorf("read %+v, want the invited account", byID)
	}
	if byEmail.ID != account.ID {
		t.Errorf("the address found %s, want %s", byEmail.ID, account.ID)
	}
}

// The cross-tenant negative test for the account repository: neither lookup reaches another
// tenant's account, and the answer is "not found" rather than "forbidden" - anything else confirms
// that it exists (multi-tenancy.md §2).
func TestAnAccountOfAnotherTenantIsNotFound(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := invitedIn(t, tenantA)

	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := postgres.NewAccountRepository().Find(ctx, account.ID)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("finding by identifier across tenants: %v, want not found", err)
	}

	err = write(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := postgres.NewAccountRepository().FindByEmail(ctx, account.Email)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("finding by address across tenants: %v, want not found", err)
	}
}

// The same address in two tenants is two people, and both are allowed. The uniqueness is per
// tenant, which is what makes a workspace a boundary rather than a namespace.
func TestTheSameAddressCanExistInTwoTenants(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	address := freshEmail(t)

	for _, tenant := range []shared.ID{tenantA, tenantB} {
		account, err := identity.Invite(freshID(t), tenant, address, "Anna")
		if err != nil {
			t.Fatalf("building: %v", err)
		}
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			return postgres.NewAccountRepository().Insert(ctx, account)
		}); err != nil {
			t.Fatalf("writing into %s: %v", tenant, err)
		}
	}
}

// Twice in one tenant is one person invited twice, and the index says so.
func TestTheSameAddressTwiceInOneTenantIsAConflict(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	first := invitedIn(t, tenantA)

	second, err := identity.Invite(freshID(t), tenantA, first.Email, "Anna again")
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	err = write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewAccountRepository().Insert(ctx, second)
	})

	if !errors.Is(err, shared.ErrConflict) || shared.AsError(err).DetailCode != "accounts.email_taken" {
		t.Fatalf("error %v, want a conflict naming the address", err)
	}
}

func TestPreferencesAreWrittenAndReadBack(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := invitedIn(t, tenantA)

	updated, err := account.WithPreferences(identity.Preferences{
		Locale: "de-AT", TimeZone: "Europe/Vienna", WeekStart: "MONDAY",
	})
	if err != nil {
		t.Fatalf("applying: %v", err)
	}

	var read identity.Account
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		accounts := postgres.NewAccountRepository()
		if err := accounts.UpdatePreferences(ctx, updated, created); err != nil {
			return err
		}
		read, err = accounts.Find(ctx, account.ID)
		return err
	}); err != nil {
		t.Fatalf("writing the preferences: %v", err)
	}

	if read.Locale != "de-AT" || read.TimeZone != "Europe/Vienna" || read.WeekStart != "MONDAY" {
		t.Errorf("read back %+v", read)
	}
}

// The cross-tenant negative test for the write: another tenant's account cannot be changed, and
// the caller is told nothing was there rather than that it succeeded.
func TestPreferencesOfAnotherTenantCannotBeWritten(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := invitedIn(t, tenantA)

	updated, err := account.WithPreferences(identity.Preferences{Locale: "nl"})
	if err != nil {
		t.Fatalf("applying: %v", err)
	}

	err = write(ctx, t, tenantB, func(ctx context.Context) error {
		return postgres.NewAccountRepository().UpdatePreferences(ctx, updated, created)
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error %v, want not found", err)
	}

	var read identity.Account
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		read, err = postgres.NewAccountRepository().Find(ctx, account.ID)
		return err
	}); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if read.Locale != "" {
		t.Errorf("tenant B changed tenant A's locale to %q", read.Locale)
	}
}

func groupIn(t *testing.T, tenant shared.ID) identity.Group {
	t.Helper()
	group, err := identity.NewGroup(identity.NewGroupInput{
		ID: freshID(t), TenantID: tenant, Name: freshName(t),
	})
	if err != nil {
		t.Fatalf("building the group: %v", err)
	}
	if err := write(context.Background(), t, tenant, func(ctx context.Context) error {
		return postgres.NewGroupRepository().Insert(ctx, group)
	}); err != nil {
		t.Fatalf("writing the group: %v", err)
	}
	return group
}

func TestAGroupKeepsItsMembersAndItsVersion(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	group := groupIn(t, tenantA)
	member := invitedIn(t, tenantA)

	var members []shared.ID
	var read identity.Group
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		groups := postgres.NewGroupRepository()
		if err := groups.AddMember(ctx, group.ID, member.ID); err != nil {
			return err
		}
		// Idempotent: a retried request is not a second membership.
		if err := groups.AddMember(ctx, group.ID, member.ID); err != nil {
			return err
		}
		var err error
		if members, err = groups.Members(ctx, group.ID); err != nil {
			return err
		}
		read, err = groups.Find(ctx, group.ID)
		return err
	}); err != nil {
		t.Fatalf("working with the group: %v", err)
	}

	if len(members) != 1 || members[0] != member.ID {
		t.Errorf("members %v, want exactly the one added twice", members)
	}
	if read.Version != 1 {
		t.Errorf("version %d, want 1", read.Version)
	}
}

// Optimistic locking against the real row: the second writer of two loses cleanly.
func TestAGroupUpdateWithAStaleVersionIsRefused(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	group := groupIn(t, tenantA)

	renamed, err := group.Rename(freshName(t))
	if err != nil {
		t.Fatalf("renaming: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewGroupRepository().Update(ctx, renamed, 1)
	}); err != nil {
		t.Fatalf("the first update: %v", err)
	}

	err = write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewGroupRepository().Update(ctx, renamed, 1)
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("the second update: %v, want a version conflict", err)
	}
}

// The cross-tenant negative test for the group repository: not readable, not writable, not
// deletable from another tenant.
func TestAGroupOfAnotherTenantIsUnreachable(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	group := groupIn(t, tenantA)

	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := postgres.NewGroupRepository().Find(ctx, group.ID)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("reading across tenants: %v, want not found", err)
	}

	renamed, _ := group.Rename("taken over")
	err = write(ctx, t, tenantB, func(ctx context.Context) error {
		return postgres.NewGroupRepository().Update(ctx, renamed, 1)
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("writing across tenants: %v, want the update to match nothing", err)
	}

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return postgres.NewGroupRepository().Delete(ctx, group.ID)
	}); err != nil {
		t.Fatalf("deleting across tenants reported an error rather than deleting nothing: %v", err)
	}

	var survived identity.Group
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		survived, err = postgres.NewGroupRepository().Find(ctx, group.ID)
		return err
	}); err != nil {
		t.Fatalf("the group did not survive tenant B's attempts: %v", err)
	}
	if survived.Name != group.Name {
		t.Errorf("the group is now named %q - tenant B changed it", survived.Name)
	}
}

func TestAMembershipIsGrantedFoundAndRevoked(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := invitedIn(t, tenantA)

	grant, err := identity.NewGrant(freshID(t), tenantA, account.ID, "",
		identity.TenantScope(), identity.RoleMember)
	if err != nil {
		t.Fatalf("building the grant: %v", err)
	}

	var found identity.Grant
	var removed bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		grants := postgres.NewMembershipGrantRepository()
		if err := grants.Grant(ctx, grant); err != nil {
			return err
		}
		// Idempotent: the same grant twice is one membership.
		if err := grants.Grant(ctx, grant); err != nil {
			return err
		}
		var err error
		if found, err = grants.Find(ctx, grant.ID); err != nil {
			return err
		}
		removed, err = grants.Revoke(ctx, grant.ID)
		return err
	}); err != nil {
		t.Fatalf("working with the membership: %v", err)
	}

	if found.Role != identity.RoleMember || found.AccountID != account.ID {
		t.Errorf("found %+v", found)
	}
	if !removed {
		t.Error("the revocation reported that nothing was there")
	}
}

// A granted membership takes effect on the next request, and a revoked one stops applying - read
// through the resolution the authorisation service uses, which is what the acceptance asks.
func TestAGrantTakesEffectAndARevocationEndsIt(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := invitedIn(t, tenantA)
	path := []identity.Scope{identity.TenantScope()}

	grant, err := identity.NewGrant(freshID(t), tenantA, account.ID, "",
		identity.TenantScope(), identity.RoleAdmin)
	if err != nil {
		t.Fatalf("building: %v", err)
	}

	var before, during, after []identity.Membership
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		memberships := postgres.NewMembershipRepository()
		grants := postgres.NewMembershipGrantRepository()

		var err error
		if before, err = memberships.Along(ctx, account.ID, path); err != nil {
			return err
		}
		if err := grants.Grant(ctx, grant); err != nil {
			return err
		}
		if during, err = memberships.Along(ctx, account.ID, path); err != nil {
			return err
		}
		if _, err := grants.Revoke(ctx, grant.ID); err != nil {
			return err
		}
		after, err = memberships.Along(ctx, account.ID, path)
		return err
	}); err != nil {
		t.Fatalf("granting and revoking: %v", err)
	}

	if len(before) != 0 {
		t.Errorf("the account held %v before anything was granted", before)
	}
	if len(during) != 1 || during[0].Role != identity.RoleAdmin {
		t.Errorf("after the grant the account holds %v, want the role", during)
	}
	if len(after) != 0 {
		t.Errorf("after the revocation the account still holds %v", after)
	}
}

// A right held through a group reaches the resolution as the account's own, which is the rule the
// whole group mechanism rests on.
func TestARoleHeldThroughAGroupReachesTheAccount(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := invitedIn(t, tenantA)
	group := groupIn(t, tenantA)

	grant, err := identity.NewGrant(freshID(t), tenantA, "", group.ID,
		identity.TenantScope(), identity.RoleViewer)
	if err != nil {
		t.Fatalf("building: %v", err)
	}

	var held []identity.Membership
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := postgres.NewGroupRepository().AddMember(ctx, group.ID, account.ID); err != nil {
			return err
		}
		if err := postgres.NewMembershipGrantRepository().Grant(ctx, grant); err != nil {
			return err
		}
		var err error
		held, err = postgres.NewMembershipRepository().
			Along(ctx, account.ID, []identity.Scope{identity.TenantScope()})
		return err
	}); err != nil {
		t.Fatalf("granting through the group: %v", err)
	}

	if len(held) != 1 || held[0].Role != identity.RoleViewer {
		t.Errorf("the account holds %v, want the role its group was given", held)
	}
}

// The cross-tenant negative test for the membership write side.
func TestAMembershipOfAnotherTenantIsUnreachable(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := invitedIn(t, tenantA)

	grant, err := identity.NewGrant(freshID(t), tenantA, account.ID, "",
		identity.TenantScope(), identity.RoleOwner)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewMembershipGrantRepository().Grant(ctx, grant)
	}); err != nil {
		t.Fatalf("granting: %v", err)
	}

	err = write(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := postgres.NewMembershipGrantRepository().Find(ctx, grant.ID)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("reading across tenants: %v, want not found", err)
	}

	var removed bool
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		removed, err = postgres.NewMembershipGrantRepository().Revoke(ctx, grant.ID)
		return err
	}); err != nil {
		t.Fatalf("revoking across tenants: %v", err)
	}
	if removed {
		t.Fatal("tenant B revoked tenant A's membership")
	}

	var held []identity.Membership
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		held, err = postgres.NewMembershipRepository().
			Along(ctx, account.ID, []identity.Scope{identity.TenantScope()})
		return err
	}); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if len(held) != 1 {
		t.Errorf("the account holds %v after tenant B's attempt, want its role intact", held)
	}
}

// Deleting a group takes the memberships it granted with it - one statement rather than three that
// could half-run - and leaves what its members hold in their own right.
func TestDeletingAGroupTakesItsMembershipsAndNothingElse(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := invitedIn(t, tenantA)
	group := groupIn(t, tenantA)

	throughGroup, err := identity.NewGrant(freshID(t), tenantA, "", group.ID,
		identity.TenantScope(), identity.RoleViewer)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	ownRight, err := identity.NewGrant(freshID(t), tenantA, account.ID, "",
		identity.TenantScope(), identity.RoleGuest)
	if err != nil {
		t.Fatalf("building: %v", err)
	}

	var held []identity.Membership
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		groups := postgres.NewGroupRepository()
		grants := postgres.NewMembershipGrantRepository()

		if err := groups.AddMember(ctx, group.ID, account.ID); err != nil {
			return err
		}
		if err := grants.Grant(ctx, throughGroup); err != nil {
			return err
		}
		if err := grants.Grant(ctx, ownRight); err != nil {
			return err
		}
		if err := groups.Delete(ctx, group.ID); err != nil {
			return err
		}
		var err error
		held, err = postgres.NewMembershipRepository().
			Along(ctx, account.ID, []identity.Scope{identity.TenantScope()})
		return err
	}); err != nil {
		t.Fatalf("deleting the group: %v", err)
	}

	if len(held) != 1 || held[0].Role != identity.RoleGuest {
		t.Errorf("the account holds %v, want only what it holds in its own right", held)
	}
}

var (
	_ repository.Accounts         = postgres.NewAccountRepository()
	_ repository.Groups           = postgres.NewGroupRepository()
	_ repository.MembershipGrants = postgres.NewMembershipGrantRepository()
)
