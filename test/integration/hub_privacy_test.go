// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/text"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// privacyRegistry is SetHubPrivacy the way main.go builds it.
func privacyRegistry(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)
	authoriser := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork, Audit: sink, Clock: fixed,
	}
	registry, err := usecase.NewRegistry(nil, work.SetHubPrivacy{
		Writer: work.ContainerWriter{
			Containers: containerRepo(), Authorizer: authoriser,
			Events: postgres.NewOutbox(jobQueue(t)), Changes: postgres.NewChangeLog(), Audit: sink,
			UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid, Text: text.Composing{},
		},
		Hubs:        postgres.NewHubLockRepository(),
		Revocations: revocationsFor(t, authoriser),
	}.Descriptor())
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

// D3 and D8 against PostgreSQL: the hub's owner, who may create hubs, makes it private; the
// workspace's owner, who read it through the workspace, gets ACCESS_REVOKED in the same
// transaction, and the hub's own member gets nothing.
func TestMakingAHubPrivateRevokesItOnTheWorkspacesDevices(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	hub, _ := hubWithCollection(ctx, t, tenantA, authorA)
	// The member owns the shared hub and may create hubs (STRUCTURE through ADMIN).
	grantOn(ctx, t, w.tenant, w.member, identity.HubScope(hub), identity.RoleOwner)
	grantOn(ctx, t, w.tenant, w.member, identity.TenantScope(), identity.RoleAdmin)

	registry := privacyRegistry(ctx, t)
	actor := administrator(w.tenant, w.member)

	out, err := registry.Invoke(ctx, work.SetHubPrivacyName, actor,
		usecase.Input{"container_id": hub.String(), "private": true})
	if err != nil {
		t.Fatalf("marking: %v", err)
	}
	if out["private"] != true || !storedContainer(ctx, t, w.tenant, hub).Private {
		t.Fatal("the hub is not private")
	}

	revoked := func(account shared.ID) int {
		return countIn(ctx, t, `SELECT count(*) FROM change_log WHERE tenant_id = $1
			AND op = 'ACCESS_REVOKED' AND container_id = $2 AND actor_id = $3`,
			w.tenant.String(), hub.String(), account.String())
	}
	if revoked(w.administrator) != 1 {
		t.Errorf("the workspace's owner was told %d times, want once", revoked(w.administrator))
	}
	if revoked(w.member) != 0 {
		t.Error("the hub's own owner was told they lost it")
	}
	if events := countIn(ctx, t, `SELECT count(*) FROM outbox_event WHERE tenant_id = $1
		AND event_type = 'de.hubtask.work.container.policies_updated.v1' AND payload->>'id' = $2`,
		w.tenant.String(), hub.String()); events != 1 {
		t.Errorf("%d policies_updated events, want one", events)
	}

	// The workspace's owner cannot share it again: they are not in it.
	_, err = registry.Invoke(ctx, work.SetHubPrivacyName, administrator(w.tenant, w.administrator),
		usecase.Input{"container_id": hub.String(), "private": false})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the workspace's owner sharing it: %v, want not found", err)
	}
}

// D7: a group holding a role in the hub stops it turning private, and a group is refused a role in
// a private one - the two writes meet on the hub's row.
func TestAGroupInTheHubStopsItTurningPrivate(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	hub, collection := hubWithCollection(ctx, t, tenantA, authorA)
	grantOn(ctx, t, w.tenant, w.member, identity.HubScope(hub), identity.RoleOwner)
	grantOn(ctx, t, w.tenant, w.member, identity.TenantScope(), identity.RoleAdmin)

	group := freshID(t)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `INSERT INTO account_group (id, tenant_id, name) VALUES ($1, $2, 'Kids')`,
		group.String(), w.tenant.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO membership (id, tenant_id, group_id, scope_type, scope_id, role)
		VALUES ($1, $2, $3, 'COLLECTION', $4, 'VIEWER')`,
		freshID(t).String(), w.tenant.String(), group.String(), collection.String()); err != nil {
		t.Fatal(err)
	}

	_, err := privacyRegistry(ctx, t).Invoke(ctx, work.SetHubPrivacyName, administrator(w.tenant, w.member),
		usecase.Input{"container_id": hub.String(), "private": true})
	if !errors.Is(err, shared.ErrConflict) || shared.AsError(err).DetailCode != "containers.private_hub_has_groups" {
		t.Fatalf("error %v, want containers.private_hub_has_groups", err)
	}
	if storedContainer(ctx, t, w.tenant, hub).Private {
		t.Error("the hub turned private with a group in it")
	}
}
