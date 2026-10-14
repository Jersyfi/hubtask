// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
)

// membershipUseCases wires the grant and the revocation against the database, with the real audit
// sink and authoriser: what the registry runs for REST, MCP and hubctl alike.
func membershipUseCases(
	ctx context.Context, t *testing.T,
) (identityservice.GrantMembership, identityservice.RevokeMembership) {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	sink := postgres.NewAuditSink(ids)
	authoriser := realAuthoriser(ctx, t)
	grants := postgres.NewMembershipGrantRepository(pageCursors())
	grant := identityservice.GrantMembership{
		Grants: grants, Accounts: postgres.NewAccountRepository(),
		Groups: postgres.NewGroupRepository(pageCursors()), Hubs: postgres.NewHubLockRepository(),
		Authorizer: authoriser, Audit: sink, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids,
	}
	revoke := identityservice.RevokeMembership{
		Grants: grants, Authorizer: authoriser, Revocations: revocationsFor(t, authoriser),
		Audit: sink, UnitOfWork: unitOfWork, Clock: fixed,
	}
	return grant, revoke
}

// A role given twice is one membership: the second grant answers the first's identifier and leaves
// one row and one `membership.granted` entry, so revoking that membership takes the role away.
// Before the unique index a second, equal row kept the role in force after the revocation.
func TestAGrantGivenTwiceIsOneMembershipAndOneRevocationEndsIt(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	grant, revoke := membershipUseCases(ctx, t)
	actor := memberAdministrator(tenantA, authorA)

	cases := []struct {
		name    string
		command func() identityservice.GrantMembershipCommand
	}{
		{"a person at the workspace", func() identityservice.GrantMembershipCommand {
			return identityservice.GrantMembershipCommand{
				AccountID: invitedIn(t, tenantA).ID, Scope: identity.TenantScope(), Role: identity.RoleMember,
			}
		}},
		{"a person at a hub", func() identityservice.GrantMembershipCommand {
			return identityservice.GrantMembershipCommand{
				AccountID: invitedIn(t, tenantA).ID, Scope: identity.HubScope(membershipHub), Role: identity.RoleViewer,
			}
		}},
		{"a group at a hub", func() identityservice.GrantMembershipCommand {
			return identityservice.GrantMembershipCommand{
				GroupID: groupIn(t, tenantA).ID, Scope: identity.HubScope(membershipHub), Role: identity.RoleContributor,
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			command := tc.command()
			first, err := grant.Execute(ctx, actor, command)
			if err != nil {
				t.Fatalf("granting: %v", err)
			}
			second, err := grant.Execute(ctx, actor, command)
			if err != nil {
				t.Fatalf("granting again: %v", err)
			}
			if second.ID != first.ID {
				t.Errorf("the repeated grant answered %s, want the membership already there, %s", second.ID, first.ID)
			}

			subject, column := command.AccountID, "account_id"
			if subject.IsZero() {
				subject, column = command.GroupID, "group_id"
			}
			equal := `SELECT count(*) FROM membership WHERE ` + column + ` = $1 AND role = $2`
			if rows := countIn(ctx, t, equal, subject.String(), string(command.Role)); rows != 1 {
				t.Errorf("%d rows for the role, want 1", rows)
			}
			if entries := countIn(ctx, t,
				`SELECT count(*) FROM audit_log WHERE target_id = $1 AND action = 'membership.granted'`,
				first.ID.String()); entries != 1 {
				t.Errorf("%d granted entries, want 1: a repeat gives nothing and the trail says nothing", entries)
			}

			if err := revoke.Execute(ctx, actor,
				identityservice.RevokeMembershipCommand{MembershipID: first.ID}); err != nil {
				t.Fatalf("revoking: %v", err)
			}
			if rows := countIn(ctx, t, equal, subject.String(), string(command.Role)); rows != 0 {
				t.Errorf("%d rows still give the role after its revocation", rows)
			}
			if !command.AccountID.IsZero() {
				path := []identity.Scope{identity.TenantScope(), command.Scope}
				if held := membershipsAlong(ctx, t, tenantA, command.AccountID, path); len(held) != 0 {
					t.Errorf("after the revocation the person still holds %v", held)
				}
			}
		})
	}

	// A guard, not the defect: another role at the same scope is another membership (an auditor's
	// second one, migration 0042).
	account := invitedIn(t, tenantA).ID
	t.Run("another role at the same scope", func(t *testing.T) {
		member, err := grant.Execute(ctx, actor, identityservice.GrantMembershipCommand{
			AccountID: account, Scope: identity.TenantScope(), Role: identity.RoleMember,
		})
		if err != nil {
			t.Fatalf("granting MEMBER: %v", err)
		}
		auditor, err := grant.Execute(ctx, actor, identityservice.GrantMembershipCommand{
			AccountID: account, Scope: identity.TenantScope(), Role: identity.RoleAuditor,
		})
		if err != nil {
			t.Fatalf("granting AUDITOR: %v", err)
		}
		if member.ID == auditor.ID {
			t.Error("two roles were answered as one membership")
		}
	})
}

// The cross-tenant guard for the grant's repository method: tenant B naming tenant A's account, at
// the scope and role A already gave it, is refused and is not answered with A's membership. The
// tenant-scoped foreign key (0005) refuses it before the index can; this holds that order.
func TestAGrantNamingAnotherTenantsAccountIsRefused(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := invitedIn(t, tenantA)
	grants := postgres.NewMembershipGrantRepository(pageCursors())

	own, err := identity.NewGrant(freshID(t), tenantA, account.ID, "", identity.TenantScope(), identity.RoleMember)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, _, err := grants.Grant(ctx, own)
		return err
	}); err != nil {
		t.Fatalf("granting in tenant A: %v", err)
	}

	foreign, err := identity.NewGrant(freshID(t), tenantB, account.ID, "", identity.TenantScope(), identity.RoleMember)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	var answered identity.Grant
	err = write(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		answered, _, err = grants.Grant(ctx, foreign)
		return err
	})
	if err == nil {
		t.Fatalf("tenant B granted tenant A's account a role, answered %+v", answered)
	}
	if answered.ID == own.ID {
		t.Error("tenant B was answered tenant A's membership")
	}
	if rows := countIn(ctx, t,
		`SELECT count(*) FROM membership WHERE account_id = $1`, account.ID.String()); rows != 1 {
		t.Errorf("%d memberships for the account, want tenant A's one", rows)
	}
	if rows := countIn(ctx, t,
		`SELECT count(*) FROM membership WHERE id = $1 AND tenant_id = $2`, own.ID.String(), tenantA.String()); rows != 1 {
		t.Error("tenant A's membership did not survive tenant B's attempt")
	}
}
