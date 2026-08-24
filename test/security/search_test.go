// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// The acceptance sentence of C-08 that is about security rather than about language: no result is
// ever an entry the actor may not read.
//
// A search is the one read that is not anchored to a place in the workspace, so what keeps an
// unreadable title out of the answer is the narrowing and nothing else - there is no scope check
// upstream that could have refused the request. The narrowing asks `Permitted` about each hit's own
// path, and this is that question, per role, at the decision point.
//
// The paths are the ones SearchItems builds (work.hitPath): tenant, hub, collection, entry. That
// they end on the entry is the whole reason an individually shared entry is findable by the person
// it was shared with - and, in the other direction, the reason a neighbouring entry is not.

// searchPath is one hit's path, as the search builds it.
func searchPath(item shared.ID) []identity.Scope {
	return append(collectionPath(), identity.ItemScope(item))
}

// searchRequest is the question the narrowing asks: a read, on the search's own target type,
// naming no single entry - a search is one view over everything, and every hit is judged by its
// path rather than as a subject.
func searchRequest() access.Request {
	return access.Request{
		Permission: service.PermissionRead,
		Action:     "item.read",
		TokenScope: "items:read",
		TargetType: "search",
	}
}

// visible runs the narrowing over two entries in one collection and answers which survived.
func visible(t *testing.T, held []identity.Membership) []bool {
	t.Helper()

	authorizer, _ := serviceWith(held)
	allowed, err := authorizer.Permitted(
		context.Background(), actor("items:read"), searchRequest(),
		[][]identity.Scope{searchPath(ownItemID), searchPath(otherItemID)},
	)
	if err != nil {
		t.Fatalf("the narrowing failed: %v", err)
	}
	if len(allowed) != 2 {
		t.Fatalf("%d answers for two entries", len(allowed))
	}
	return allowed
}

// Every role in the matrix carries READ where it is granted, so every role finds the entries of the
// collection it holds - and this is the positive half. The negative halves are the two tests below,
// and they are the ones the acceptance criterion is about.
func TestASearchAnswersEveryRoleTheEntriesItsRoleReaches(t *testing.T) {
	for _, role := range []identity.Role{
		identity.RoleOwner, identity.RoleAdmin, identity.RoleMember,
		identity.RoleContributor, identity.RoleViewer, identity.RoleGuest,
	} {
		t.Run(string(role), func(t *testing.T) {
			allowed := visible(t, grantedAt(identity.CollectionScope(collectionID), role))
			if !allowed[0] || !allowed[1] {
				t.Errorf("a %s searching their own collection found %v", role, allowed)
			}
		})
	}
}

// An account that holds nothing on the path finds nothing, whatever it searches for. That is T-04
// with a different verb: an entry nobody granted them anything on is not refused, it is absent -
// and a search that answered its title would be the disclosure the refusal elsewhere prevents.
func TestASearchAnswersNothingToAnAccountThatHoldsNothing(t *testing.T) {
	allowed := visible(t, nil)
	if allowed[0] || allowed[1] {
		t.Errorf("an account holding no membership found %v", allowed)
	}

	// And a membership somewhere else in the workspace is the same as none: a role on another hub
	// is not a role on this path.
	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	allowed = visible(t, grantedAt(identity.HubScope(elsewhere), identity.RoleOwner))
	if allowed[0] || allowed[1] {
		t.Errorf("an owner of another hub found %v", allowed)
	}
}

// Somebody an entry was shared with finds that entry and not the one beside it. Both live in the
// same collection, so a narrowing that judged hits by their collection would answer both - which is
// exactly the mistake a path ending on the entry cannot make.
func TestASearchAnswersASharedEntryAndNotItsNeighbour(t *testing.T) {
	allowed := visible(t, grantedAt(identity.ItemScope(ownItemID), identity.RoleGuest))

	if !allowed[0] {
		t.Error("the entry that was shared was not found by the person it was shared with")
	}
	if allowed[1] {
		t.Error("the entry beside the shared one was found - the share opened its collection")
	}
}

// A token without the read scope finds nothing, whatever the memberships say. The scope is checked
// once for the whole page rather than per hit, which is what makes it a refusal rather than an
// empty answer: a client with the wrong token is told so.
func TestASearchWithoutTheReadScopeIsRefused(t *testing.T) {
	authorizer, _ := serviceWith(grantedAt(identity.CollectionScope(collectionID), identity.RoleOwner))

	_, err := authorizer.Permitted(
		context.Background(), actor("items:write"), searchRequest(),
		[][]identity.Scope{searchPath(ownItemID)},
	)
	if err == nil {
		t.Fatal("a token without items:read searched anyway")
	}
}
