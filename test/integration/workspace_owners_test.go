// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/repository/identity"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// ownersWorld is a workspace of its own - the owners are counted over a whole tenant, and the shared
// tenants carry other tests' memberships - with every kind of holder the count has to tell apart.
type ownersWorld struct {
	tenant                              shared.ID
	paula, ida, robot, gerd, erik, dora shared.ID
	adam                                shared.ID
	group                               shared.ID
	paulaGrant, groupGrant              shared.ID
}

func seedOwnersWorld(ctx context.Context, t *testing.T) ownersWorld {
	t.Helper()
	w := ownersWorld{
		tenant: freshID(t), paula: freshID(t), ida: freshID(t), robot: freshID(t), gerd: freshID(t),
		erik: freshID(t), dora: freshID(t), adam: freshID(t), group: freshID(t),
		paulaGrant: freshID(t), groupGrant: freshID(t),
	}
	digits := strings.ReplaceAll(w.tenant.String(), "-", "")
	person := func(id shared.ID, name, status, deleted string) struct {
		sql  string
		args []any
	} {
		return struct {
			sql  string
			args []any
		}{
			`INSERT INTO account (id, tenant_id, email, display_name, status, deleted_at)
			 VALUES ($1, $2, $3, $4, $5::account_status, ` + deleted + `)`,
			[]any{id.String(), w.tenant.String(), strings.ToLower(name) + "-" + digits + "@example.org", name, status},
		}
	}
	owner := func(account shared.ID, grant shared.ID, role string) struct {
		sql  string
		args []any
	} {
		return struct {
			sql  string
			args []any
		}{
			`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', $4::membership_role)`,
			[]any{grant.String(), w.tenant.String(), account.String(), role},
		}
	}
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Owners')`,
			[]any{w.tenant.String(), "owners-" + digits[len(digits)-20:]}},
		person(w.paula, "Paula", "ACTIVE", "NULL"),
		person(w.ida, "Ida", "INVITED", "NULL"),
		person(w.gerd, "Gerd", "RESTRICTED", "NULL"),
		person(w.erik, "Erik", "ANONYMIZED", "NULL"),
		person(w.dora, "Dora", "ACTIVE", "now()"),
		person(w.adam, "Adam", "ACTIVE", "NULL"),
		{`INSERT INTO account (id, tenant_id, kind, display_name) VALUES ($1, $2, 'SERVICE_ACCOUNT', 'Robot')`,
			[]any{w.robot.String(), w.tenant.String()}},
		owner(w.paula, w.paulaGrant, "OWNER"),
		owner(w.ida, freshID(t), "OWNER"),
		owner(w.robot, freshID(t), "OWNER"),
		owner(w.dora, freshID(t), "OWNER"),
		owner(w.adam, freshID(t), "ADMIN"),
		{`INSERT INTO account_group (id, tenant_id, name) VALUES ($1, $2, 'Owners')`,
			[]any{w.group.String(), w.tenant.String()}},
		{`INSERT INTO account_group_member (tenant_id, group_id, account_id) VALUES ($5, $1, $2), ($5, $1, $3), ($5, $1, $4)`,
			[]any{w.group.String(), w.gerd.String(), w.erik.String(), w.adam.String(), w.tenant.String()}},
		{`INSERT INTO membership (id, tenant_id, group_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'OWNER')`,
			[]any{w.groupGrant.String(), w.tenant.String(), w.group.String()}},
	} {
		if _, err := adminPool(ctx, t).Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the workspace: %v\n%s", err, statement.sql)
		}
	}
	return w
}

// The guard of domain-model.md §3.2 counts the people who keep a workspace administered: a person's
// own OWNER grant and a group's, through each member, and only a USER who can act - not an
// invitation never redeemed, an erased or deleted account, or a service account.
func TestTheOwnersAreThePeopleWhoCanAct(t *testing.T) {
	ctx := context.Background()
	w := seedOwnersWorld(ctx, t)
	other := seedOwnersWorld(ctx, t)
	grants := postgres.NewMembershipGrantRepository(pageCursors())

	var paths []identity.OwnerPath
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		if err := grants.LockOwners(ctx); err != nil {
			return err
		}
		var err error
		paths, err = grants.OwnerPaths(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the owners: %v", err)
	}

	want := []identity.OwnerPath{
		{MembershipID: w.paulaGrant, AccountID: w.paula},
		{MembershipID: w.groupGrant, AccountID: w.gerd, GroupID: w.group},
		// Adam holds ADMIN in his own right and OWNER through the group: the group's path counts.
		{MembershipID: w.groupGrant, AccountID: w.adam, GroupID: w.group},
	}
	for _, path := range want {
		if !slices.Contains(paths, path) {
			t.Errorf("owners %+v miss %+v", paths, path)
		}
	}
	if len(paths) != len(want) {
		t.Errorf("owners %+v, want exactly %+v", paths, want)
	}
	for _, path := range paths {
		if path.MembershipID == other.paulaGrant || path.MembershipID == other.groupGrant {
			t.Errorf("another workspace's owner %+v was counted", path)
		}
	}
}

// #1429 against PostgreSQL through the use cases (domain-model.md §3.2): Paula owns the workspace,
// Gerd and Adam through the owners' group. Deleting the group leaves Paula; revoking Paula's own
// grant then would leave only an invitation, a service account and a deleted account holding
// OWNER, none of whom counts - refused, and the grant stays.
func TestTheWorkspaceKeepsAnOwner(t *testing.T) {
	ctx := context.Background()
	w := seedOwnersWorld(ctx, t)
	authoriser := realAuthoriser(ctx, t)
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	grants := postgres.NewMembershipGrantRepository(pageCursors())
	sink := postgres.NewAuditSink(clockadapter.NewUUIDv7(portclock.Fixed(created)))
	paula := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: w.tenant, AccountID: w.paula, AccountName: "Paula",
		Scopes: []string{"members:write"},
	}

	err := identityservice.DeleteGroup{
		Groups: postgres.NewGroupRepository(pageCursors()), Grants: grants, Authorizer: authoriser,
		Revocations: revocationsFor(t, authoriser), Audit: sink, UnitOfWork: unitOfWork,
		Clock: portclock.Fixed(created), Owners: grants, StepUp: provenStepUp{},
	}.Execute(ctx, paula, identityservice.DeleteGroupCommand{GroupID: w.group, StepUpToken: "hbt_sup_x"})
	if err != nil {
		t.Fatalf("deleting the owners' group while Paula owns the workspace: %v", err)
	}

	err = identityservice.RevokeMembership{
		Grants: grants, Authorizer: authoriser, Revocations: revocationsFor(t, authoriser), Audit: sink,
		UnitOfWork: unitOfWork, Clock: portclock.Fixed(created), Owners: grants, StepUp: provenStepUp{},
	}.Execute(ctx, paula, identityservice.RevokeMembershipCommand{MembershipID: w.paulaGrant, StepUpToken: "hbt_sup_x"})
	if shared.AsError(err).DetailCode != "memberships.last_owner" {
		t.Fatalf("revoking the last owner answered %v, want memberships.last_owner", err)
	}

	var paths []identity.OwnerPath
	if err := read(ctx, t, w.tenant, func(ctx context.Context) error {
		var err error
		paths, err = grants.OwnerPaths(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if len(paths) != 1 || paths[0].AccountID != w.paula {
		t.Errorf("owners after the refusal %+v, want Paula's grant", paths)
	}
}
