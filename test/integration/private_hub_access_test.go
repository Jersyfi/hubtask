// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// privateWorld is one workspace with a private hub in it: an administrator of the workspace who
// holds nothing on the hub, a member of the hub, and an entry in its collection.
type privateWorld struct {
	tenant                shared.ID
	hub, collection, item shared.ID
	administrator, member shared.ID
	sharedHub, sharedColl shared.ID
	// title is the entry's, one word nothing else in the shared database carries, for the search.
	title string
	// trashed is an entry of the hub in the trash, where a reader's table seeds one.
	trashed shared.ID
}

func seedPrivateWorld(ctx context.Context, t *testing.T) privateWorld {
	t.Helper()
	seedContainerTenants(ctx, t)

	w := privateWorld{tenant: tenantA, administrator: freshID(t), member: freshID(t)}
	w.title = "tagebuch" + strings.ReplaceAll(freshID(t).String(), "-", "")[20:]
	w.hub = privateHub(ctx, t, tenantA, authorA)
	w.collection, w.item = freshID(t), freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		collection := containerIn(tenantA, authorA, w.collection, freshName(t), "a0")
		collection.Type = work.ContainerCollection
		collection.ParentID = w.hub
		if err := containerRepo().Insert(ctx, collection); err != nil {
			return err
		}
		return itemRepo().Insert(ctx, taskIn(tenantA, authorA, w.collection, w.item, w.title, "a0"))
	}); err != nil {
		t.Fatalf("seeding the private hub's content: %v", err)
	}
	w.sharedHub, w.sharedColl = hubWithCollection(ctx, t, tenantA, authorA)

	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Paul Parent')`,
			[]any{w.administrator.String(), tenantA.String()}},
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Lena Kind')`,
			[]any{w.member.String(), tenantA.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		  VALUES ($1, $2, $3, 'TENANT', 'OWNER')`,
			[]any{freshID(t).String(), tenantA.String(), w.administrator.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		  VALUES ($1, $2, $3, 'TENANT', 'MEMBER')`,
			[]any{freshID(t).String(), tenantA.String(), w.member.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		  VALUES ($1, $2, $3, 'HUB', $4, 'OWNER')`,
			[]any{freshID(t).String(), tenantA.String(), w.member.String(), w.hub.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the people: %v", err)
		}
	}
	return w
}

func realAuthoriser(ctx context.Context, t *testing.T) access.Service {
	t.Helper()
	return access.Service{
		Memberships: postgres.NewMembershipRepository(),
		UnitOfWork:  postgres.NewUnitOfWork(appPool(ctx, t)),
		Audit:       postgres.NewAuditSink(clockadapter.NewUUIDv7(clockadapter.System{})),
		Clock:       portclock.Fixed(created),
	}
}

func reader(tenant, account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: account, AccountName: "Someone",
		Scopes: []string{"items:read", "containers:read"},
	}
}

// ADR-0073 §1 against PostgreSQL as the application role: the workspace's owner reaches nothing in
// a private hub, the hub's member reaches all of it, and a shared hub beside it is unchanged.
func TestAPrivateHubIsReachedOnlyByItsMembers(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	authoriser := realAuthoriser(ctx, t)

	privatePath := []identity.Scope{
		identity.TenantScope(), identity.HubScope(w.hub), identity.CollectionScope(w.collection),
	}
	sharedPath := []identity.Scope{
		identity.TenantScope(), identity.HubScope(w.sharedHub), identity.CollectionScope(w.sharedColl),
	}
	read := access.Request{Permission: service.PermissionRead, Path: privatePath}

	if allowed, err := authoriser.Permits(ctx, reader(w.tenant, w.administrator), read); err != nil || allowed {
		t.Errorf("the workspace's owner: allowed %v, error %v; want refused", allowed, err)
	}
	if allowed, err := authoriser.Permits(ctx, reader(w.tenant, w.member), read); err != nil || !allowed {
		t.Errorf("the hub's member: allowed %v, error %v; want allowed", allowed, err)
	}

	err := authoriser.Authorize(ctx, reader(w.tenant, w.administrator), access.Request{
		Permission: service.PermissionRead, Path: privatePath, Action: "item.read",
		On: access.ItemSubject{Does: service.ItemRead, ID: w.item},
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the owner reading the entry: %v, want not found (T-04)", err)
	}

	// A collection-only path - an automation rule's - is completed and judged the same way.
	ruleRead := access.Request{
		Permission: service.PermissionRead,
		Path:       []identity.Scope{identity.TenantScope(), identity.CollectionScope(w.collection)},
	}
	if allowed, err := authoriser.Permits(ctx, reader(w.tenant, w.administrator), ruleRead); err != nil || allowed {
		t.Errorf("the owner through a collection path: allowed %v, error %v; want refused", allowed, err)
	}

	// The question a rule's run_as and a subscription's creator are asked (D4, D5).
	for name, tc := range map[string]struct {
		account shared.ID
		path    []identity.Scope
		want    bool
	}{
		"the owner, the private hub":  {w.administrator, privatePath, true},
		"the member, the private hub": {w.member, privatePath, false},
		"the owner, a shared hub":     {w.administrator, sharedPath, false},
	} {
		if hidden, err := authoriser.Hidden(ctx, reader(w.tenant, tc.account), tc.path); err != nil || hidden != tc.want {
			t.Errorf("%s: hidden %v, error %v; want %v", name, hidden, err, tc.want)
		}
	}

	answers, err := authoriser.Permitted(ctx, reader(w.tenant, w.administrator),
		access.Request{Permission: service.PermissionRead}, [][]identity.Scope{privatePath, sharedPath})
	if err != nil || len(answers) != 2 || answers[0] || !answers[1] {
		t.Errorf("the owner's level: %v, error %v; want [false true]", answers, err)
	}
}

// The cross-tenant row: another workspace's owner reaches neither the private hub nor anything
// about it, whatever identifiers they know.
func TestAPrivateHubIsNotReachedFromAnotherWorkspace(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)

	// The boundary is row level security on the read every reader makes before it asks the
	// authoriser: the hub, its collection and its entry are not there in tenant B.
	err := read(ctx, t, tenantB, func(ctx context.Context) error {
		for _, id := range []shared.ID{w.hub, w.collection} {
			if _, err := containerRepo().Find(ctx, id); !errors.Is(err, shared.ErrNotFound) {
				t.Errorf("tenant B read %s: %v, want not found", id, err)
			}
		}
		if _, err := itemRepo().Find(ctx, w.item); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B read the entry: %v, want not found", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if hubs := hubsOf(ctx, t, tenantB, w.hub, w.collection, w.item); len(hubs) != 0 {
		t.Errorf("tenant B learnt about the private hub: %+v", hubs)
	}
}
