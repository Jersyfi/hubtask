// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The negative suite of C-04, one case per role rather than one per use case.
//
// It runs against the decision point itself, which is where it belongs: fifteen use cases ask this
// one question, so asking it once per use case would be fifteen copies of the same assertion - and
// a sixteenth use case would still be uncovered. What makes that safe is the gate beside it
// (test/architecture, TestEveryItemWriteNamesItsSubject): no use case can reach a repository with
// a write on an entry without going through what is tested here.

var (
	tenantID     = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	actorID      = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	strangerID   = shared.MustParseID("0192f000-0000-7000-8000-00000000000f")
	hubID        = shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	collectionID = shared.MustParseID("0192f000-0000-7000-8000-00000000000c")
	ownItemID    = shared.MustParseID("0192f000-0000-7000-8000-000000000010")
	otherItemID  = shared.MustParseID("0192f000-0000-7000-8000-000000000011")
	at           = time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)
)

// answer is what the caller is told, at the granularity a client can act on.
type answer int

const (
	permitted answer = iota
	// forbidden is the one refusal every narrowing produces: the actor may reach the entry and may
	// not do this to it.
	forbidden
	// invisible is the answer for an entry nothing on its path grants the actor anything on. It is
	// what T-04 requires, and it must be indistinguishable from an entry that does not exist.
	invisible
)

func (a answer) String() string {
	switch a {
	case permitted:
		return "permitted"
	case forbidden:
		return "forbidden"
	default:
		return "invisible"
	}
}

func TestTheNarrowingsPerRole(t *testing.T) {
	// The entry the actor holds a role over, assigned to somebody else unless the case says
	// otherwise, and its neighbour - reachable through the same collection, so that a difference in
	// the answer is the narrowing and not the path.
	suites := []struct {
		role identity.Role
		// on the collection, so the whole level is within reach: what is being tested is what the
		// role does there, not where it was granted.
		read, create, changeOwn, changeOther, commentOwn, commentOther answer
	}{
		{identity.RoleOwner, permitted, permitted, permitted, permitted, permitted, permitted},
		{identity.RoleAdmin, permitted, permitted, permitted, permitted, permitted, permitted},
		{identity.RoleMember, permitted, permitted, permitted, permitted, permitted, permitted},
		// The row C-04 exists for: writes only on what is theirs, and creating is a write on
		// something that will be.
		{identity.RoleContributor, permitted, permitted, permitted, forbidden, permitted, forbidden},
		{identity.RoleViewer, permitted, forbidden, forbidden, forbidden, forbidden, forbidden},
		// A guest comments and changes nothing, whoever the entry is assigned to.
		{identity.RoleGuest, permitted, forbidden, forbidden, forbidden, permitted, permitted},
	}

	for _, suite := range suites {
		t.Run(string(suite.role), func(t *testing.T) {
			on := grantedAt(identity.CollectionScope(collectionID), suite.role)

			assertAnswer(t, "reading", on, request(service.ItemRead, ownItemID, strangerID), suite.read)
			assertAnswer(t, "creating", on, creation(), suite.create)
			assertAnswer(t, "changing their own", on,
				request(service.ItemChange, ownItemID, actorID), suite.changeOwn)
			assertAnswer(t, "changing another's", on,
				request(service.ItemChange, otherItemID, strangerID), suite.changeOther)
			assertAnswer(t, "commenting on their own", on,
				request(service.ItemComment, ownItemID, actorID), suite.commentOwn)
			assertAnswer(t, "commenting on another's", on,
				request(service.ItemComment, otherItemID, strangerID), suite.commentOther)
		})
	}
}

// The acceptance criterion read literally: an entry that was not shared is not refused but absent,
// and absent in the words a genuinely missing entry produces (T-04).
func TestAnEntryNothingGrantsAccessToIsInvisible(t *testing.T) {
	// A share on one entry, asked about the entry beside it. This is the guest of the role matrix:
	// the membership is at ITEM scope, which is what sharing an entry is.
	share := grantedAt(identity.ItemScope(ownItemID), identity.RoleGuest)

	assertAnswer(t, "the entry that was shared", share,
		request(service.ItemRead, ownItemID, strangerID), permitted)
	assertAnswer(t, "the entry beside it", share,
		request(service.ItemRead, otherItemID, strangerID), invisible)

	// And an account that holds nothing anywhere sees neither, by the same sentence: "shared items
	// only" is where the membership was granted, not a rule about the role.
	assertAnswer(t, "an account holding nothing", nil,
		request(service.ItemRead, ownItemID, strangerID), invisible)
}

// A share reaches the entry itself and not the collection around it, which is what makes it a
// share rather than a role.
func TestASharedEntryDoesNotOpenItsCollection(t *testing.T) {
	share := grantedAt(identity.ItemScope(ownItemID), identity.RoleGuest)

	level := access.Request{
		Permission: service.PermissionRead,
		Path:       collectionPath(),
		Action:     "item.read",
		TokenScope: "items:read",
		TargetType: "container",
		TargetID:   collectionID,
	}

	authorizer, _ := serviceWith(share)
	if err := authorizer.Authorize(context.Background(), actor("items:read"), level); !errors.Is(
		err, shared.ErrForbidden,
	) {
		t.Errorf("a share opened the collection around it: %v", err)
	}
}

// assertAnswer runs one case through the real decision point and judges what a client is told.
func assertAnswer(
	t *testing.T, what string, held []identity.Membership, request access.Request, want answer,
) {
	t.Helper()

	authorizer, trail := serviceWith(held)
	err := authorizer.Authorize(context.Background(), actor("items:write", "items:read"), request)

	got := permitted
	switch {
	case errors.Is(err, shared.ErrNotFound):
		got = invisible
	case errors.Is(err, shared.ErrForbidden):
		got = forbidden
	case err != nil:
		t.Fatalf("%s: unexpected error %v", what, err)
	}

	if got != want {
		t.Errorf("%s: %s, want %s (%v)", what, got, want, err)
	}
	if got == permitted {
		return
	}

	// Every refusal is recorded, whichever of the two answers the client got: "who tried to reach
	// what" is the question the trail exists for, and hiding the entry from the client is not
	// hiding the attempt from an auditor (audit.md §4, B-02).
	if len(trail.entries) != 1 || trail.entries[0].Outcome != audit.OutcomeDenied {
		t.Errorf("%s: the refusal was not recorded: %+v", what, trail.entries)
	}
	if got == forbidden {
		// The same code and status as any other refusal - a third party learns that they may not,
		// never why.
		if code := shared.AsError(err).DetailCode; code != "access.not_permitted" {
			t.Errorf("%s: detail code %q, want access.not_permitted", what, code)
		}
		if named := shared.AsError(err).Params["permission"]; named == "" {
			t.Errorf("%s: the refusal names no permission", what)
		}
	}
	if got == invisible {
		// Indistinguishable from an entry that does not exist, down to the parameters.
		if code := shared.AsError(err).DetailCode; code != "items.not_found" {
			t.Errorf("%s: detail code %q, want the code a missing entry produces", what, code)
		}
	}
}

func request(does service.ItemAction, item, assignee shared.ID) access.Request {
	permission, scope := service.PermissionWriteItems, "items:write"
	if does == service.ItemRead {
		permission, scope = service.PermissionRead, "items:read"
	}
	return access.Request{
		Permission: permission,
		Path:       collectionPath(),
		Action:     "item.updated",
		TokenScope: scope,
		TargetType: "item",
		TargetID:   item,
		On:         access.ItemSubject{Does: does, ID: item, Assignee: assignee},
	}
}

// creation names no entry: there is nothing yet to share or to assign.
func creation() access.Request {
	return access.Request{
		Permission: service.PermissionWriteItems,
		Path:       collectionPath(),
		Action:     "item.created",
		TokenScope: "items:write",
		TargetType: "item",
		TargetID:   collectionID,
		On:         access.ItemSubject{Does: service.ItemCreate},
	}
}

func collectionPath() []identity.Scope {
	return []identity.Scope{
		identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(collectionID),
	}
}

func grantedAt(scope identity.Scope, role identity.Role) []identity.Membership {
	return []identity.Membership{{AccountID: actorID, Scope: scope, Role: role}}
}

func actor(scopes ...string) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: actorID, Scopes: scopes,
	}
}

func serviceWith(held []identity.Membership) (access.Service, *trail) {
	recorded := &trail{}
	return access.Service{
		Memberships: &memberships{held: held},
		UnitOfWork:  transactions{},
		Audit:       recorded,
		Clock:       clock.Fixed(at),
	}, recorded
}

// The ports, as fakes: the decision under test reads memberships and writes refusals, and nothing
// here decides anything.

type memberships struct{ held []identity.Membership }

func (m *memberships) Along(
	_ context.Context, _ shared.ID, path []identity.Scope,
) ([]identity.Membership, error) {
	applies := make([]identity.Membership, 0, len(m.held))
	for _, membership := range m.held {
		for _, step := range path {
			if step == membership.Scope {
				applies = append(applies, membership)
				break
			}
		}
	}
	return applies, nil
}

func (m *memberships) SharedItemsIn(_ context.Context, _, _ shared.ID) ([]shared.ID, error) {
	return nil, nil
}

func (m *memberships) Administrators(context.Context, []identity.Scope) ([]shared.ID, error) {
	return nil, nil
}

type trail struct{ entries []audit.Entry }

func (s *trail) Append(_ context.Context, entry audit.Entry) error {
	s.entries = append(s.entries, entry)
	return nil
}

type transactions struct{}

func (transactions) Within(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	return fn(ctx)
}

func (transactions) WithinReadOnly(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	return fn(ctx)
}
