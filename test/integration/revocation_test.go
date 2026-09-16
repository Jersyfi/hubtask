// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	syncdomain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// revocationsFor is the revocation service against the real repositories (N-08), the way
// cmd/server/main.go builds it.
func revocationsFor(t *testing.T, permits access.Permitter) access.Revocations {
	t.Helper()
	hybrid, err := clockadapter.NewHybridClock(portclock.Fixed(created), "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	return access.Revocations{
		Permits:    permits,
		Grants:     postgres.NewMembershipGrantRepository(pageCursors()),
		Groups:     postgres.NewGroupRepository(pageCursors()),
		Containers: containerRepo(),
		Items:      itemRepo(),
		Changes:    postgres.NewChangeLog(),
		HLC:        hybrid,
	}
}

// revokerFor is the RevokeMembership use case the way main.go builds it, with the real revocation
// service behind it.
func revokerFor(ctx context.Context, t *testing.T) identityservice.RevokeMembership {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	sink := postgres.NewAuditSink(clockadapter.NewUUIDv7(fixed))
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork, Audit: sink, Clock: fixed,
	}
	return identityservice.RevokeMembership{
		Grants:      postgres.NewMembershipGrantRepository(pageCursors()),
		Authorizer:  authorizer,
		Revocations: revocationsFor(t, authorizer),
		Audit:       sink, UnitOfWork: unitOfWork, Clock: fixed,
	}
}

// grantOn gives the account a role at the scope and returns the grant, so that a test can revoke
// exactly it.
func grantOn(ctx context.Context, t *testing.T, tenant, account shared.ID, scope identity.Scope, role identity.Role) identity.Grant {
	t.Helper()
	grant, err := identity.NewGrant(freshID(t), tenant, account, "", scope, role)
	if err != nil {
		t.Fatalf("building the grant: %v", err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return postgres.NewMembershipGrantRepository(pageCursors()).Grant(ctx, grant)
	}); err != nil {
		t.Fatalf("granting: %v", err)
	}
	return grant
}

// revocationsOn are the ACCESS_REVOKED records on a page, by the container they name.
func revocationsOn(records []syncservice.Record) map[shared.ID]shared.ID {
	found := map[shared.ID]shared.ID{}
	for _, record := range records {
		if record.Op == changelog.AccessRevoked {
			found[record.ContainerID] = record.ActorID
		}
	}
	return found
}

// SY-6: access revoked during an offline phase. The pull delivers the record to the account that
// lost access - and to no other member of the same hub, and not to a person with a second path to
// the collection - the push is refused, and the stream carries the record the same way.
func TestAccessRevokedOfflineIsDeliveredAndThePushRefused(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantA, authorA)
	var hub shared.ID
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := containerRepo().Find(ctx, collection)
		hub = found.ParentID
		return err
	}); err != nil {
		t.Fatalf("reading the hub: %v", err)
	}

	// Three people on the hub: one who will lose it, one who keeps it, and one who loses the hub
	// but holds the collection directly.
	member, other, twoPaths := seedAccount(ctx, t, tenantA), seedAccount(ctx, t, tenantA), seedAccount(ctx, t, tenantA)
	lost := grantOn(ctx, t, tenantA, member, identity.HubScope(hub), identity.RoleMember)
	grantOn(ctx, t, tenantA, other, identity.HubScope(hub), identity.RoleMember)
	alsoLost := grantOn(ctx, t, tenantA, twoPaths, identity.HubScope(hub), identity.RoleMember)
	grantOn(ctx, t, tenantA, twoPaths, identity.CollectionScope(collection), identity.RoleMember)

	// Every device goes offline here.
	stream, pull := streamFor(ctx, t), pullFor(ctx, t)
	positions := map[shared.ID]syncservice.Position{}
	for _, account := range []shared.ID{member, other, twoPaths} {
		from, err := stream.Resume(ctx, streamActor(tenantA, account), "")
		if err != nil {
			t.Fatalf("resuming: %v", err)
		}
		positions[account] = from
	}

	revoke := revokerFor(ctx, t)
	for _, grant := range []identity.Grant{lost, alsoLost} {
		if err := revoke.Execute(ctx, memberAdministrator(tenantA, authorA),
			identityservice.RevokeMembershipCommand{MembershipID: grant.ID}); err != nil {
			t.Fatalf("revoking: %v", err)
		}
	}

	pulled := func(account shared.ID) map[shared.ID]shared.ID {
		page, err := pull.Pull(ctx, streamActor(tenantA, account), syncservice.PullRequest{
			DeviceID: freshID(t), Cursor: pull.Encode(positions[account]), Limit: 100,
		})
		if err != nil {
			t.Fatalf("pulling: %v", err)
		}
		return revocationsOn(page.Records)
	}
	if got := pulled(member); got[hub] != member {
		t.Errorf("the pull delivered %v, want the revocation at the hub, addressed to the member", got)
	}
	if got := pulled(other); len(got) != 0 {
		t.Errorf("another member of the hub was handed a revocation: %v", got)
	}
	if got := pulled(twoPaths); len(got) != 0 {
		t.Errorf("a person with a second path to the collection was told: %v", got)
	}

	// The stream carries it the same way.
	batch, err := stream.Next(ctx, streamActor(tenantA, member), positions[member])
	if err != nil {
		t.Fatalf("reading the stream: %v", err)
	}
	if got := revocationsOn(batch.Records); got[hub] != member {
		t.Errorf("the stream carried %v, want the revocation", got)
	}
	batch, err = stream.Next(ctx, streamActor(tenantA, other), positions[other])
	if err != nil {
		t.Fatalf("reading the stream: %v", err)
	}
	if got := revocationsOn(batch.Records); len(got) != 0 {
		t.Errorf("the stream handed another member a revocation: %v", got)
	}

	// The queued mutation is refused with forbidden - and answered, not swallowed.
	push := pushFor(ctx, t, 5*time.Minute)
	response, err := push.Push(ctx, pushActor(tenantA, member), syncservice.PushRequest{
		DeviceID: freshID(t),
		Mutations: []syncservice.Mutation{{
			OpID: freshID(t), Kind: syncdomain.ItemCreate, ItemID: freshID(t),
			Payload: map[string]any{"type": "TASK", "collection_id": collection.String(), "title": "Written on the train"},
		}},
	})
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if result := response.Results[0]; result.Result != syncdomain.Rejected || result.Error == nil || result.Error.Code != "forbidden" {
		t.Errorf("the push after the revocation was answered %+v, want REJECTED with forbidden", result)
	}
}

// The cross-tenant negative for OfGroup (gate SG-3): a group's grants are not readable from the
// other tenant's transaction.
func TestAGroupsGrantsAreNotReadableAcrossTheTenantBoundary(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	grants := postgres.NewMembershipGrantRepository(pageCursors())

	var own, foreign []identity.Grant
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		own, err = grants.OfGroup(ctx, membershipGroup)
		return err
	}); err != nil {
		t.Fatalf("reading the group's grants: %v", err)
	}
	if len(own) == 0 {
		t.Fatalf("the seeded group holds no grant, so the negative proves nothing")
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		foreign, err = grants.OfGroup(ctx, membershipGroup)
		return err
	}); err != nil {
		t.Fatalf("reading across the boundary: %v", err)
	}
	if len(foreign) != 0 {
		t.Errorf("the other tenant read %d grants of the group", len(foreign))
	}
}
