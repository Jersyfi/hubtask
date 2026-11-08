// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The group list behind the members screen, against the real boundary. Gate SG-3: List has its
// cross-tenant negative below; Find and Members have theirs in identity_test.go.

func groupsListed(ctx context.Context, t *testing.T, tenant shared.ID, page repository.Page) repository.GroupPage {
	t.Helper()
	var listed repository.GroupPage
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		listed, err = postgres.NewGroupRepository(pageCursors()).List(ctx, page)
		return err
	}); err != nil {
		t.Fatalf("listing the groups: %v", err)
	}
	return listed
}

func groupNamed(ctx context.Context, t *testing.T, tenant shared.ID, name string) identity.Group {
	t.Helper()
	group, err := identity.NewGroup(identity.NewGroupInput{ID: freshID(t), TenantID: tenant, Name: name})
	if err != nil {
		t.Fatalf("building the group: %v", err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return postgres.NewGroupRepository(pageCursors()).Insert(ctx, group)
	}); err != nil {
		t.Fatalf("writing the group: %v", err)
	}
	return group
}

// The walk is by name regardless of case, every group exactly once, and the last page says so.
// The database is shared with every other test, so the walk finds its own rows among the rest
// rather than expecting to be alone.
func TestTheGroupsArePagedByName(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	prefix := strings.ToLower(freshName(t))
	own := map[shared.ID]string{}
	for _, suffix := range []string{"c", "A", "b"} {
		group := groupNamed(ctx, t, tenantA, prefix+"-"+suffix)
		own[group.ID] = group.Name
	}

	var walked []identity.Group
	cursor := ""
	for pages := 0; ; pages++ {
		page := groupsListed(ctx, t, tenantA, repository.Page{Cursor: cursor, Size: 2})
		walked = append(walked, page.Groups...)
		if !page.Info.HasMore {
			break
		}
		if pages > 200 {
			t.Fatal("the walk does not end")
		}
		cursor = page.Info.NextCursor
	}

	var mine []string
	seen := map[shared.ID]int{}
	for _, group := range walked {
		if _, ours := own[group.ID]; ours {
			mine = append(mine, group.Name)
			seen[group.ID]++
		}
	}
	if len(mine) != 3 {
		t.Fatalf("the walk found %d of the three groups: %v", len(mine), mine)
	}
	for id, times := range seen {
		if times != 1 {
			t.Errorf("group %s was listed %d times", id, times)
		}
	}
	for i := 1; i < len(mine); i++ {
		if strings.ToLower(mine[i-1]) > strings.ToLower(mine[i]) {
			t.Errorf("%q came before %q; the order is by name regardless of case", mine[i-1], mine[i])
		}
	}
}

// The cross-tenant negative for List (gate SG-3).
func TestTheGroupsOfAnotherTenantAreNotListed(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	group := groupNamed(ctx, t, tenantA, freshName(t))

	cursor := ""
	for pages := 0; ; pages++ {
		page := groupsListed(ctx, t, tenantB, repository.Page{Cursor: cursor, Size: 200})
		for _, listed := range page.Groups {
			if listed.ID == group.ID {
				t.Fatalf("tenant B listed tenant A's group %s", group.ID)
			}
		}
		if !page.Info.HasMore || pages > 50 {
			break
		}
		cursor = page.Info.NextCursor
	}
}

// The groups are any member's and no requester's, against the real memberships (UC-ID-23): a
// person holding a READ role of the workspace's own on one hub lists them and reads one with its
// members; a person holding only REQUEST at a collection - the portal's requester - is refused both
// as READ, recorded; and the member asking in another workspace, where they hold nothing, is
// refused too (gate SG-3).
func TestTheGroupsAreAMembersAndNotARequesters(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	ids := make([]shared.ID, 8)
	for i := range ids {
		ids[i] = freshID(t)
	}
	member, requester, readRole, requestRole, hub, desk, readGrant, requestGrant :=
		ids[0], ids[1], ids[2], ids[3], ids[4], ids[5], ids[6], ids[7]
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Mira')`, []any{member.String(), tenantA.String()}},
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Hanna')`, []any{requester.String(), tenantA.String()}},
		{`INSERT INTO custom_role (id, tenant_id, key, name, permissions, created_at, updated_at)
		  VALUES ($1, $2, $3, 'Contractor', ARRAY['READ'], now(), now())`,
			[]any{readRole.String(), tenantA.String(), "contractor_" + readRole.String()[30:]}},
		{`INSERT INTO custom_role (id, tenant_id, key, name, permissions, created_at, updated_at)
		  VALUES ($1, $2, $3, 'Client', ARRAY['REQUEST'], now(), now())`,
			[]any{requestRole.String(), tenantA.String(), "client_" + requestRole.String()[30:]}},
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		  VALUES ($1, $2, 'HUB', $3, 'a0', $4)`, []any{hub.String(), tenantA.String(), freshName(t), authorA.String()}},
		{`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  VALUES ($1, $2, 'COLLECTION', $3, 'Desk', 'a0', $4)`,
			[]any{desk.String(), tenantA.String(), hub.String(), authorA.String()}},
		{`INSERT INTO custom_role_grant (id, tenant_id, account_id, scope_type, scope_id, role_id, created_at)
		  VALUES ($1, $2, $3, 'HUB', $4, $5, now())`,
			[]any{readGrant.String(), tenantA.String(), member.String(), hub.String(), readRole.String()}},
		{`INSERT INTO custom_role_grant (id, tenant_id, account_id, scope_type, scope_id, role_id, created_at)
		  VALUES ($1, $2, $3, 'COLLECTION', $4, $5, now())`,
			[]any{requestGrant.String(), tenantA.String(), requester.String(), desk.String(), requestRole.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v\n%s", err, statement.sql)
		}
	}
	group := groupNamed(ctx, t, tenantA, freshName(t))

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork,
		Audit: postgres.NewAuditSink(clockadapter.NewUUIDv7(clockadapter.System{})), Clock: portclock.Fixed(created),
	}
	groups := postgres.NewGroupRepository(pageCursors())
	list := identityservice.ListGroups{Members: authorizer, Groups: groups, UnitOfWork: unitOfWork}
	get := identityservice.GetGroup{Members: authorizer, Groups: groups, UnitOfWork: unitOfWork}
	as := func(tenant, account shared.ID) appshared.ActorContext {
		return appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant, AccountID: account, Scopes: []string{"members:read"}}
	}

	if _, err := list.Execute(ctx, as(tenantA, member), identityservice.ListGroupsQuery{Size: 200}); err != nil {
		t.Errorf("a member listing the groups: %v", err)
	}
	if _, err := get.Execute(ctx, as(tenantA, member), group.ID); err != nil {
		t.Errorf("a member reading a group: %v", err)
	}
	for name, actor := range map[string]appshared.ActorContext{
		"the requester": as(tenantA, requester), "the member in another workspace": as(tenantB, member),
	} {
		_, err := list.Execute(ctx, actor, identityservice.ListGroupsQuery{})
		if !errors.Is(err, shared.ErrForbidden) || shared.AsError(err).DetailCode != "access.not_permitted" {
			t.Errorf("%s listing the groups = %v, want access.not_permitted", name, err)
		}
		if _, err := get.Execute(ctx, actor, group.ID); !errors.Is(err, shared.ErrForbidden) {
			t.Errorf("%s reading a group = %v, want forbidden", name, err)
		}
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE actor_id = $1 AND action = 'group.read' AND outcome = 'DENIED'`,
		requester.String()); n != 2 {
		t.Errorf("%d refusals of the requester recorded, want two", n)
	}

	// A credential without members:read is refused for its scope, and that refusal is recorded
	// as any other read's is (audit.md §4).
	unscoped := as(tenantA, member)
	unscoped.Scopes = []string{"items:read"}
	if _, err := list.Execute(ctx, unscoped, identityservice.ListGroupsQuery{}); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "access.insufficient_scope" {
		t.Errorf("listing without the scope = %v, want access.insufficient_scope", err)
	}
	if _, err := get.Execute(ctx, unscoped, group.ID); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "access.insufficient_scope" {
		t.Errorf("reading without the scope = %v, want access.insufficient_scope", err)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE actor_id = $1 AND tenant_id = $2 AND action = 'group.read'
		AND outcome = 'DENIED'`, member.String(), tenantA.String()); n != 2 {
		t.Errorf("%d scope refusals of the member recorded, want two", n)
	}
}

// The page's total is the length of the whole walk, on every page, and counts the workspace's own
// groups only: tenant B's total is its own walk's, without tenant A's group (gate SG-3).
func TestTheGroupsTotalIsTheWalksLengthInItsOwnWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	group := groupNamed(ctx, t, tenantA, freshName(t))

	for _, tenant := range []shared.ID{tenantA, tenantB} {
		var walked, totals []int
		seen := false
		cursor := ""
		for pages := 0; ; pages++ {
			page := groupsListed(ctx, t, tenant, repository.Page{Cursor: cursor, Size: 50})
			if page.Info.Total == nil {
				t.Fatalf("tenant %s: a page of the groups carries no total", tenant)
			}
			totals = append(totals, *page.Info.Total)
			walked = append(walked, len(page.Groups))
			for _, listed := range page.Groups {
				seen = seen || listed.ID == group.ID
			}
			if !page.Info.HasMore || pages > 50 {
				break
			}
			cursor = page.Info.NextCursor
		}
		length := 0
		for _, n := range walked {
			length += n
		}
		for _, total := range totals {
			if total != length {
				t.Errorf("tenant %s: total %d, the walk %d", tenant, total, length)
			}
		}
		if seen != (tenant == tenantA) {
			t.Errorf("tenant %s: tenant A's group listed = %v", tenant, seen)
		}
	}
}
