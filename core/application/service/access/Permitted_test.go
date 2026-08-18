// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var otherHubID = shared.MustParseID("0192f000-0000-7000-8000-00000000000c")

func readRequest() Request {
	return Request{
		Permission: service.PermissionRead,
		Action:     "container.read",
		TokenScope: "containers:read",
		TargetType: "container",
	}
}

func hubPaths(ids ...shared.ID) [][]identity.Scope {
	paths := make([][]identity.Scope, 0, len(ids))
	for _, id := range ids {
		paths = append(paths, []identity.Scope{identity.TenantScope(), identity.HubScope(id)})
	}
	return paths
}

func readService(held []identity.Membership) (Service, *memberships, *sink) {
	store, trail := &memberships{held: held}, &sink{}
	return Service{
		Memberships: store, UnitOfWork: &unitOfWork{}, Audit: trail, Clock: clock.Fixed(now),
	}, store, trail
}

// The reason this method exists: a membership on one hub answers yes for that hub and no for the
// other, where a single check at the tenant scope would answer no for both.
func TestPermittedAnswersPerPath(t *testing.T) {
	held := []identity.Membership{
		{AccountID: accountID, Scope: identity.HubScope(hubID), Role: identity.RoleViewer},
	}
	guard, _, _ := readService(held)

	allowed, err := guard.Permitted(
		t.Context(), actorWithScopes("containers:read"), readRequest(), hubPaths(hubID, otherHubID))
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if len(allowed) != 2 {
		t.Fatalf("%d answers for 2 paths", len(allowed))
	}
	if !allowed[0] {
		t.Error("the hub the membership is held on came back refused")
	}
	if allowed[1] {
		t.Error("a hub with no membership came back allowed")
	}
}

// One membership read for the whole page, not one per row: the port's comment is explicit that the
// query may be generous and must not be unbounded.
func TestPermittedReadsTheMembershipsOnce(t *testing.T) {
	guard, store, _ := readService(nil)
	uow := guard.UnitOfWork.(*unitOfWork)

	if _, err := guard.Permitted(
		t.Context(), actorWithScopes("containers:read"), readRequest(),
		hubPaths(hubID, otherHubID),
	); err != nil {
		t.Fatalf("asking: %v", err)
	}

	if uow.reads != 1 {
		t.Errorf("the membership table was read in %d transactions, want 1", uow.reads)
	}
	if uow.writes != 0 {
		t.Errorf("a permission question opened %d write transactions", uow.writes)
	}

	// The union, deduplicated: the tenant scope appears on both paths and is asked about once.
	if len(store.path) != 3 {
		t.Fatalf("the query asked about %+v, want the tenant and both hubs once each", store.path)
	}
	seen := map[identity.Scope]int{}
	for _, scope := range store.path {
		seen[scope]++
	}
	for scope, count := range seen {
		if count != 1 {
			t.Errorf("%+v appears %d times in the union", scope, count)
		}
	}
}

// A row left out of a list is not a denied access - nobody was refused anything. The refusal that is
// recorded is the token scope, because that one refuses the whole operation (audit.md §4).
func TestPermittedRecordsNoRefusalForANarrowedAnswer(t *testing.T) {
	guard, _, trail := readService(nil)

	allowed, err := guard.Permitted(
		t.Context(), actorWithScopes("containers:read"), readRequest(), hubPaths(hubID))
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if allowed[0] {
		t.Error("an actor with no membership came back allowed")
	}
	if len(trail.entries) != 0 {
		t.Errorf("narrowing a page wrote %d audit entries", len(trail.entries))
	}
}

func TestPermittedRefusesAndRecordsAMissingTokenScope(t *testing.T) {
	guard, _, trail := readService(nil)

	_, err := guard.Permitted(
		t.Context(), actorWithScopes("items:read"), readRequest(), hubPaths(hubID))
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a token without the scope answered %v", err)
	}
	if len(trail.entries) != 1 {
		t.Fatalf("%d audit entries for a refused scope, want 1", len(trail.entries))
	}
	if trail.entries[0].Action != readRequest().Action {
		t.Errorf("the entry names the action %q", trail.entries[0].Action)
	}
}

func TestPermittedNeedsACredential(t *testing.T) {
	guard, _, _ := readService(nil)

	_, err := guard.Permitted(t.Context(), appshared.ActorContext{}, readRequest(), hubPaths(hubID))
	if !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("an unauthenticated actor answered %v", err)
	}
}

// Nothing to ask about is not a failure: a page with no rows narrows to no rows, and the membership
// table is not read at all.
func TestPermittedWithNoPathsReadsNothing(t *testing.T) {
	guard, _, _ := readService(nil)
	uow := guard.UnitOfWork.(*unitOfWork)

	allowed, err := guard.Permitted(
		t.Context(), actorWithScopes("containers:read"), readRequest(), nil)
	if err != nil {
		t.Fatalf("asking about nothing: %v", err)
	}
	if len(allowed) != 0 {
		t.Errorf("%d answers for no paths", len(allowed))
	}
	if uow.reads != 0 {
		t.Errorf("asking about nothing read the membership table %d times", uow.reads)
	}
}

// A membership table that cannot be read is not "the actor sees nothing". Reporting it as an empty
// page would hide an outage as an empty workspace.
func TestPermittedReportsAFailedReadRatherThanRefusing(t *testing.T) {
	broken := errors.New("the membership table is unreachable")
	store, trail := &memberships{err: broken}, &sink{}
	guard := Service{
		Memberships: store, UnitOfWork: &unitOfWork{}, Audit: trail, Clock: clock.Fixed(now),
	}

	_, err := guard.Permitted(
		t.Context(), actorWithScopes("containers:read"), readRequest(), hubPaths(hubID))
	if !errors.Is(err, broken) {
		t.Errorf("a failed membership read answered %v", err)
	}
	if len(trail.entries) != 0 {
		t.Error("a failed read was recorded as a refusal")
	}
}
