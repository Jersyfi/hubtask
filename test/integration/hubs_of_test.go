// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	identityrepository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// hubCollectionAndEntry seeds a hub with one collection and one entry in it.
func hubCollectionAndEntry(ctx context.Context, t *testing.T, tenant, author shared.ID) (hub, collection, item shared.ID) {
	t.Helper()
	seedContainerTenants(ctx, t)

	hub, collection = hubWithCollection(ctx, t, tenant, author)
	item = freshID(t)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, taskIn(tenant, author, collection, item, "Entry", "a0"))
	}); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}
	return hub, collection, item
}

func hubsOf(ctx context.Context, t *testing.T, tenant shared.ID, ids ...shared.ID) map[shared.ID]identityrepository.Hub {
	t.Helper()

	var hubs map[shared.ID]identityrepository.Hub
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		hubs, err = postgres.NewMembershipRepository().HubsOf(ctx, ids)
		return err
	}); err != nil {
		t.Fatalf("reading the hubs: %v", err)
	}
	return hubs
}

// A hub answers itself, a collection its hub, an entry its collection's hub: the authoriser
// completes a path from this, so a role on the hub counts below it.
func TestTheHubOfAContainerAndAnEntryIsFound(t *testing.T) {
	ctx := context.Background()
	hub, collection, item := hubCollectionAndEntry(ctx, t, tenantA, authorA)

	hubs := hubsOf(ctx, t, tenantA, hub, collection, item, freshID(t))
	for _, id := range []shared.ID{hub, collection, item} {
		if hubs[id].ID != hub {
			t.Errorf("%s sits under %q, want the hub %s", id, hubs[id].ID, hub)
		}
	}
	if len(hubs) != 3 {
		t.Errorf("an identifier naming nothing reached the answer: %+v", hubs)
	}
}

// A private hub answers its flag for itself, its collections and their entries alike: the authoriser
// reads privacy from storage rather than from the path it was given (ADR-0073 §1).
func TestTheHubsAnswerTheirPrivacy(t *testing.T) {
	ctx := context.Background()
	hub := privateHub(ctx, t, tenantA, authorA)
	sharedHub, collection, item := hubCollectionAndEntry(ctx, t, tenantA, authorA)

	hubs := hubsOf(ctx, t, tenantA, hub, sharedHub, collection, item)
	if !hubs[hub].Private {
		t.Error("the private hub answered shared")
	}
	for _, id := range []shared.ID{sharedHub, collection, item} {
		if hubs[id].Private {
			t.Errorf("%s answered private under a shared hub", id)
		}
	}
}

// The cross-tenant negative test for HubsOf (gate SG-3): another tenant's containers and entries
// answer nothing, even when their identifiers are known.
func TestTheHubsOfAnotherTenantAreInvisible(t *testing.T) {
	ctx := context.Background()
	hub, collection, item := hubCollectionAndEntry(ctx, t, tenantA, authorA)

	if hubs := hubsOf(ctx, t, tenantB, hub, collection, item); len(hubs) != 0 {
		t.Errorf("tenant B read %d of tenant A's hubs: %+v", len(hubs), hubs)
	}
}

// The hub's lock is taken from anything under it, answers its privacy, and sees a group holding a
// role anywhere below it; another tenant's hub is neither locked nor answered.
func TestTheHubIsLockedFromBelowAndItsGroupsAreSeen(t *testing.T) {
	ctx := context.Background()
	hub := privateHub(ctx, t, tenantA, authorA)
	_, collection, item := hubCollectionAndEntry(ctx, t, tenantA, authorA)
	locks := postgres.NewHubLockRepository()

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		got, found, err := locks.LockHubOf(ctx, hub)
		if err != nil || !found || got.ID != hub || !got.Private {
			t.Errorf("locking the private hub: %+v, found %v, error %v", got, found, err)
		}
		for _, id := range []shared.ID{collection, item} {
			got, found, err := locks.LockHubOf(ctx, id)
			if err != nil || !found || got.Private {
				t.Errorf("locking the hub of %s: %+v, found %v, error %v", id, got, found, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	group := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO account_group (id, tenant_id, name) VALUES ($1, $2, 'Parents')`,
		group.String(), tenantA.String()); err != nil {
		t.Fatalf("seeding the group: %v", err)
	}
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO membership (id, tenant_id, group_id, scope_type, scope_id, role)
		 VALUES ($1, $2, $3, 'ITEM', $4, 'VIEWER')`,
		freshID(t).String(), tenantA.String(), group.String(), item.String()); err != nil {
		t.Fatalf("granting the group: %v", err)
	}
	hubOfItem := hubsOf(ctx, t, tenantA, item)[item].ID
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		if held, err := locks.GroupHoldsRoleUnder(ctx, hubOfItem); err != nil || !held {
			t.Errorf("a group on an entry was not seen under its hub: %v, %v", held, err)
		}
		if held, err := locks.GroupHoldsRoleUnder(ctx, hub); err != nil || held {
			t.Errorf("a group was seen under a hub it holds nothing in: %v, %v", held, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Tenant B: nothing to lock, and nothing held.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, found, err := locks.LockHubOf(ctx, hub); err != nil || found {
			t.Errorf("tenant B locked tenant A's hub: found %v, error %v", found, err)
		}
		if held, err := locks.GroupHoldsRoleUnder(ctx, hubOfItem); err != nil || held {
			t.Errorf("tenant B saw tenant A's group: %v, %v", held, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
