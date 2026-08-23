// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

var (
	collectionID = shared.MustParseID("0192f000-0000-7000-8000-00000000000c")
	itemID       = shared.MustParseID("0192f000-0000-7000-8000-000000000010")
	neighbourID  = shared.MustParseID("0192f000-0000-7000-8000-000000000011")
	otherAccount = shared.MustParseID("0192f000-0000-7000-8000-00000000000f")
)

func itemRequest(does service.ItemAction, assignee shared.ID) Request {
	return Request{
		Permission: service.PermissionWriteItems,
		Path: []identity.Scope{
			identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(collectionID),
		},
		Action:     "item.completed",
		TokenScope: "items:write",
		TargetType: "item",
		TargetID:   itemID,
		On:         ItemSubject{Does: does, ID: itemID, Assignee: assignee},
	}
}

func held(scope identity.Scope, role identity.Role) []identity.Membership {
	return []identity.Membership{{AccountID: accountID, Scope: scope, Role: role}}
}

// The entry's own scope joins the path, and it is what makes a share resolvable at all.
func TestTheEntrysScopeIsOnThePathItIsAskedAbout(t *testing.T) {
	authorize, store, _, _ := serviceWith(held(identity.TenantScope(), identity.RoleMember))

	if err := authorize.Authorize(
		context.Background(), actorWithScopes("items:write"), itemRequest(service.ItemChange, accountID),
	); err != nil {
		t.Fatalf("a member was refused: %v", err)
	}
	if len(store.path) != 4 || store.path[3] != identity.ItemScope(itemID) {
		t.Errorf("the entry's scope is not on the path asked about: %+v", store.path)
	}

	// A creation names no entry, so the path ends where the entry would be created.
	request := itemRequest(service.ItemCreate, shared.ID(""))
	request.On.ID = shared.ID("")
	request.TargetID = collectionID
	if err := authorize.Authorize(
		context.Background(), actorWithScopes("items:write"), request,
	); err != nil {
		t.Fatalf("a member was refused a creation: %v", err)
	}
	if len(store.path) != 3 {
		t.Errorf("a creation asked about an entry that does not exist: %+v", store.path)
	}
}

// The narrowing, through the decision point rather than in the domain: a contributor writes what
// is theirs and is refused on what is not, with the code and the status of any other refusal.
func TestAContributorIsRefusedOnAnEntryThatIsNotTheirs(t *testing.T) {
	authorize, _, trail, _ := serviceWith(held(identity.CollectionScope(collectionID), identity.RoleContributor))
	actor := actorWithScopes("items:write")

	if err := authorize.Authorize(
		context.Background(), actor, itemRequest(service.ItemChange, accountID),
	); err != nil {
		t.Fatalf("a contributor was refused their own entry: %v", err)
	}

	err := authorize.Authorize(
		context.Background(), actor, itemRequest(service.ItemChange, otherAccount))
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
	if got := shared.AsError(err).DetailCode; got != "access.not_permitted" {
		t.Errorf("detail code %s, want access.not_permitted - the same as any other refusal", got)
	}
	if got := shared.AsError(err).Params["permission"]; got != string(service.PermissionWriteItems) {
		t.Errorf("the refusal names permission %q, want WRITE_ITEMS", got)
	}

	// The trail says which narrowing refused, although the client is told what it is always told.
	if len(trail.entries) != 1 {
		t.Fatalf("%d audit entries, want 1", len(trail.entries))
	}
	if got := deniedBy(trail.entries[0]); got != "assignment" {
		t.Errorf("the refusal was recorded as %q, want assignment", got)
	}
	if got := permissionOf(trail.entries[0]); got != string(service.PermissionWriteItems) {
		t.Errorf("the entry names permission %q, want the one that was missing", got)
	}
}

// A guest comments and changes nothing - the one cell where the matrix widens, and the one beside
// it where it does not.
func TestAGuestCommentsAndChangesNothing(t *testing.T) {
	authorize, _, trail, _ := serviceWith(held(identity.ItemScope(itemID), identity.RoleGuest))
	actor := actorWithScopes("items:write")

	if err := authorize.Authorize(
		context.Background(), actor, itemRequest(service.ItemComment, otherAccount),
	); err != nil {
		t.Fatalf("a guest was refused a comment on what was shared with them: %v", err)
	}

	for _, does := range []service.ItemAction{service.ItemChange, service.ItemCreate} {
		err := authorize.Authorize(context.Background(), actor, itemRequest(does, otherAccount))
		if !errors.Is(err, shared.ErrForbidden) {
			t.Errorf("a guest may %s: %v", does, err)
		}
	}
	for _, entry := range trail.entries {
		if got := deniedBy(entry); got != "permission" {
			t.Errorf("a guest's refusal was recorded as %q, want permission", got)
		}
	}
}

// T-04: an entry nothing on its path grants the actor anything on is not refused but absent, and
// absent in exactly the words a genuinely missing entry produces.
func TestAnEntryOutOfReachIsNotFoundRatherThanForbidden(t *testing.T) {
	authorize, _, trail, _ := serviceWith(held(identity.ItemScope(neighbourID), identity.RoleGuest))

	request := itemRequest(service.ItemRead, otherAccount)
	request.Permission = service.PermissionRead
	request.TokenScope = "items:read"

	err := authorize.Authorize(context.Background(), actorWithScopes("items:read"), request)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error %v, want not found", err)
	}
	if got := shared.AsError(err).DetailCode; got != "items.not_found" {
		t.Errorf("detail code %s, want the code a missing entry produces", got)
	}
	if got := shared.AsError(err).Params["item_id"]; got != itemID.String() {
		t.Errorf("the answer names %q, want the entry that was asked for", got)
	}

	// Not disclosing it to the client is not the same as not recording it: "who tried to reach
	// what" is the question the trail exists for.
	if len(trail.entries) != 1 || deniedBy(trail.entries[0]) != "sharing" {
		t.Errorf("the refusal was not recorded as a sharing refusal: %+v", trail.entries)
	}
}

// A creation discloses no existence, so it is refused as it always was rather than hidden.
func TestACreationOutOfReachStaysForbidden(t *testing.T) {
	authorize, _, _, _ := serviceWith(nil)

	request := itemRequest(service.ItemCreate, shared.ID(""))
	request.On.ID = shared.ID("")
	request.TargetID = collectionID

	err := authorize.Authorize(context.Background(), actorWithScopes("items:write"), request)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
}

// A request that names no entry is decided by the permission alone, exactly as before C-04.
func TestAContainerRequestIsUnchanged(t *testing.T) {
	authorize, store, _, _ := serviceWith(held(identity.TenantScope(), identity.RoleAdmin))

	if err := authorize.Authorize(
		context.Background(), actorWithScopes("containers:write"), request(),
	); err != nil {
		t.Fatalf("an administrator was refused: %v", err)
	}
	if len(store.path) != 2 {
		t.Errorf("a container request grew an entry scope: %+v", store.path)
	}
}

func deniedBy(entry audit.Entry) string { return changeTo(entry, "denied_by") }

func permissionOf(entry audit.Entry) string { return changeTo(entry, "permission") }

// changeTo reads one masked field back out of the entry's change set.
func changeTo(entry audit.Entry, field string) string {
	change, ok := entry.Changes[field].(map[string]any)
	if !ok {
		return ""
	}
	to, _ := change["to"].(string)
	return to
}

// A list anchored to a container has two right answers rather than one, and ReachInto is where
// that is decided (C-04).
func TestAListReachesTheWholeContainerOrOnlyWhatWasShared(t *testing.T) {
	listRequest := Request{
		Permission: service.PermissionRead,
		Path: []identity.Scope{
			identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(collectionID),
		},
		Action:     "item.read",
		TokenScope: "items:read",
		TargetType: "container",
		TargetID:   collectionID,
	}
	actor := actorWithScopes("items:read")

	// A role on the collection answers for every row in it, and the share query never runs.
	authorize, store, trail, _ := serviceWith(held(identity.CollectionScope(collectionID), identity.RoleViewer))
	store.shares = []shared.ID{itemID}
	reach, err := authorize.ReachInto(context.Background(), actor, listRequest, collectionID)
	if err != nil {
		t.Fatalf("a viewer was refused the level: %v", err)
	}
	if !reach.All || len(reach.Shared) != 0 {
		t.Errorf("reach %+v, want the whole container", reach)
	}
	if len(trail.entries) != 0 {
		t.Errorf("a permitted list recorded a refusal: %+v", trail.entries)
	}

	// No role on the collection, and a membership on one entry inside it: the level is that entry.
	authorize, store, trail, _ = serviceWith(held(identity.ItemScope(itemID), identity.RoleGuest))
	store.shares = []shared.ID{itemID}
	reach, err = authorize.ReachInto(context.Background(), actor, listRequest, collectionID)
	if err != nil {
		t.Fatalf("somebody holding a share was refused the level: %v", err)
	}
	if reach.All || len(reach.Shared) != 1 || reach.Shared[0] != itemID {
		t.Errorf("reach %+v, want the one shared entry", reach)
	}
	// Nobody was refused anything, so nothing is recorded: a DENIED entry here would be a trail an
	// auditor cannot read.
	if len(trail.entries) != 0 {
		t.Errorf("a narrowed list recorded a refusal: %+v", trail.entries)
	}

	// Neither: the refusal stands, and it is recorded once.
	authorize, _, trail, _ = serviceWith(nil)
	_, err = authorize.ReachInto(context.Background(), actor, listRequest, collectionID)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
	if len(trail.entries) != 1 || deniedBy(trail.entries[0]) != "permission" {
		t.Errorf("the refusal was not recorded once: %+v", trail.entries)
	}
}
