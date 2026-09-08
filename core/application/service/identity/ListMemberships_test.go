// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	otherHubID   = shared.ID("01936f2a-7c1e-7000-8000-0000000000b2")
	collectionID = shared.ID("01936f2a-7c1e-7000-8000-0000000000b3")
	itemID       = shared.ID("01936f2a-7c1e-7000-8000-0000000000e1")
)

// containerFinder and itemFinder answer the two questions the path resolution asks.
type containerFinder struct{ byID map[shared.ID]work.Container }

func (f containerFinder) Find(_ context.Context, id shared.ID) (work.Container, error) {
	container, found := f.byID[id]
	if !found {
		return work.Container{}, shared.ErrNotFound.WithDetail("containers.not_found")
	}
	return container, nil
}

type itemFinder struct{ byID map[shared.ID]work.WorkItem }

func (f itemFinder) Find(_ context.Context, id shared.ID) (work.WorkItem, error) {
	item, found := f.byID[id]
	if !found {
		return work.WorkItem{}, appshared.ItemNotFound(id)
	}
	return item, nil
}

// permitter is the unrecorded half of the authorisation service: it answers and remembers the
// path it was asked about.
type permitter struct {
	allowed  bool
	requests []access.Request
}

func (p *permitter) Permits(_ context.Context, _ appshared.ActorContext, request access.Request) (bool, error) {
	p.requests = append(p.requests, request)
	return p.allowed, nil
}

func workspace() containerFinder {
	return containerFinder{byID: map[shared.ID]work.Container{
		hubID:        {ID: hubID, Type: work.ContainerHub},
		otherHubID:   {ID: otherHubID, Type: work.ContainerHub},
		collectionID: {ID: collectionID, Type: work.ContainerCollection, ParentID: hubID},
	}}
}

func reader() appshared.ActorContext {
	actor := admin()
	actor.Scopes = []string{membersRead}
	return actor
}

func grantAt(id shared.ID, scope domain.Scope) domain.Grant {
	grant, err := domain.NewGrant(id, tenant, invitedID, "", scope, domain.RoleMember)
	if err != nil {
		panic(err)
	}
	return grant
}

func listHandler(grants *grantStore, auth *authorizer, permits *permitter) ListMemberships {
	return ListMemberships{
		Grants: grants, Containers: workspace(),
		Items:      itemFinder{byID: map[shared.ID]work.WorkItem{itemID: {ID: itemID, CollectionID: collectionID}}},
		Authorizer: auth, Permits: permits, UnitOfWork: &unitOfWork{},
	}
}

func TestTheMembershipsAtAHubAreListedAndNothingGrantedElsewhere(t *testing.T) {
	grants := newGrants(
		grantAt("01936f2a-7c1e-7000-8000-0000000000d1", domain.HubScope(hubID)),
		grantAt("01936f2a-7c1e-7000-8000-0000000000d2", domain.HubScope(otherHubID)),
		grantAt("01936f2a-7c1e-7000-8000-0000000000d3", domain.TenantScope()),
	)
	permits := &permitter{allowed: true}

	page, err := listHandler(grants, &authorizer{}, permits).
		Execute(t.Context(), reader(), ListMembershipsQuery{ScopeType: domain.ScopeHub, ScopeID: hubID})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}

	if len(page.Grants) != 1 || page.Grants[0].Scope.ID != hubID {
		t.Errorf("listed %+v, want the one grant at the hub", page.Grants)
	}
	if len(permits.requests) != 1 {
		t.Fatalf("%d permission questions, want one", len(permits.requests))
	}
	path := permits.requests[0].Path
	if len(path) != 2 || path[0].Type != domain.ScopeTenant || path[1] != domain.HubScope(hubID) {
		t.Errorf("the path walked was %v, want the tenant and then the hub", path)
	}
	if grants.listedPage.Size != defaultPageSize {
		t.Errorf("page size %d, want the default", grants.listedPage.Size)
	}
}

// A collection's path runs through its hub, and an entry's through both: a membership held at the
// hub applies downwards, and a path that stopped short would refuse the person who holds it.
func TestAnEntryScopeWalksThroughItsCollectionAndItsHub(t *testing.T) {
	permits := &permitter{allowed: true}

	if _, err := listHandler(newGrants(), &authorizer{}, permits).
		Execute(t.Context(), reader(), ListMembershipsQuery{ScopeType: domain.ScopeItem, ScopeID: itemID}); err != nil {
		t.Fatalf("listing: %v", err)
	}

	want := []domain.Scope{
		domain.TenantScope(), domain.HubScope(hubID), domain.CollectionScope(collectionID), domain.ItemScope(itemID),
	}
	got := permits.requests[0].Path
	if len(got) != len(want) {
		t.Fatalf("the path walked was %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d of the path is %v, want %v", i, got[i], want[i])
		}
	}
}

// T-04: a scope the reader holds nothing on and a scope that does not exist are one answer, down
// to the message code - the difference would be an oracle for which identifiers exist.
func TestAScopeTheReaderCannotReachIsNotFoundInTheWordsAMissingOneProduces(t *testing.T) {
	cases := []struct {
		name  string
		query ListMembershipsQuery
	}{
		{"a hub that does not exist", ListMembershipsQuery{
			ScopeType: domain.ScopeHub, ScopeID: "01936f2a-7c1e-7000-8000-0000000000ff"}},
		{"a hub the reader may not read", ListMembershipsQuery{ScopeType: domain.ScopeHub, ScopeID: hubID}},
		{"a collection named as a hub", ListMembershipsQuery{ScopeType: domain.ScopeHub, ScopeID: collectionID}},
		{"a hub named as a collection", ListMembershipsQuery{ScopeType: domain.ScopeCollection, ScopeID: hubID}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := listHandler(newGrants(), &authorizer{}, &permitter{allowed: false}).
				Execute(t.Context(), reader(), tc.query)
			if !errors.Is(err, shared.ErrNotFound) {
				t.Fatalf("got %v, want not found", err)
			}
			if code := shared.AsError(err).DetailCode; code != "containers.not_found" {
				t.Errorf("detail %q, want the repository's own words", code)
			}
		})
	}

	t.Run("an entry the reader may not read", func(t *testing.T) {
		_, err := listHandler(newGrants(), &authorizer{}, &permitter{allowed: false}).
			Execute(t.Context(), reader(), ListMembershipsQuery{ScopeType: domain.ScopeItem, ScopeID: itemID})
		if !errors.Is(err, shared.ErrNotFound) || shared.AsError(err).DetailCode != "items.not_found" {
			t.Errorf("got %v, want the entry not found", err)
		}
	})
}

// The workspace is the one scope whose existence is no secret, so a refusal there is a refusal,
// and it goes through the recording half of the authorisation service.
func TestTheWorkspaceScopeIsRefusedRatherThanHidden(t *testing.T) {
	refusal := shared.ErrForbidden.WithDetail("access.not_permitted")
	auth := &authorizer{refuse: refusal}
	permits := &permitter{allowed: true}

	_, err := listHandler(newGrants(), auth, permits).
		Execute(t.Context(), reader(), ListMembershipsQuery{ScopeType: domain.ScopeTenant})

	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("got %v, want the refusal as it was", err)
	}
	if len(auth.requests) != 1 || len(permits.requests) != 0 {
		t.Errorf("%d recorded and %d unrecorded questions, want one recorded", len(auth.requests), len(permits.requests))
	}
}

func TestTheScopeIsCheckedBeforeAnythingIsRead(t *testing.T) {
	cases := []struct {
		name  string
		query ListMembershipsQuery
		code  string
	}{
		{"a hub without an identifier", ListMembershipsQuery{ScopeType: domain.ScopeHub}, "memberships.scope_identifier_required"},
		{"the workspace with one", ListMembershipsQuery{ScopeType: domain.ScopeTenant, ScopeID: hubID}, "memberships.scope_identifier_unexpected"},
		{"a level that is not one", ListMembershipsQuery{ScopeType: "BUCKET", ScopeID: hubID}, "memberships.scope_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			permits := &permitter{allowed: true}
			_, err := listHandler(newGrants(), &authorizer{}, permits).Execute(t.Context(), reader(), tc.query)
			if !errors.Is(err, shared.ErrValidation) || shared.AsError(err).DetailCode != tc.code {
				t.Errorf("got %v, want %s", err, tc.code)
			}
			if len(permits.requests) != 0 {
				t.Error("a permission question was asked about a scope that was never valid")
			}
		})
	}
}

func TestAPageSizeIsClampedToTheContract(t *testing.T) {
	grants := newGrants()
	if _, err := listHandler(grants, &authorizer{}, &permitter{allowed: true}).
		Execute(t.Context(), reader(), ListMembershipsQuery{ScopeType: domain.ScopeHub, ScopeID: hubID, Size: 500}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	if grants.listedPage.Size != maxPageSize {
		t.Errorf("page size %d, want the maximum", grants.listedPage.Size)
	}
}

// A credential without the scope is refused before any path is walked (ADR-0005).
func TestACredentialWithoutTheScopeReadsNothing(t *testing.T) {
	permits := &permitter{allowed: true}
	_, err := listHandler(newGrants(), &authorizer{}, permits).
		Execute(t.Context(), admin(), ListMembershipsQuery{ScopeType: domain.ScopeHub, ScopeID: hubID})
	if !errors.Is(err, shared.ErrForbidden) || len(permits.requests) != 0 {
		t.Errorf("got %v after %d questions, want the scope refusal and no question", err, len(permits.requests))
	}
}

// The descriptor is the contract every channel is validated against: the exact keys the REST
// controller builds have to be declared fields, or the route answers 400.
func TestTheListDescriptorAcceptsWhatTheControllerSends(t *testing.T) {
	descriptor := ListMemberships{}.Descriptor()
	in := usecase.Input{
		"scope_type": "HUB", "scope_id": hubID.String(), "cursor": "abc", "size": 20,
	}
	if err := descriptor.ValidateInput(in); err != nil {
		t.Errorf("the controller's input is refused: %v", err)
	}
	if !descriptor.ReadOnly {
		t.Error("a list is a read")
	}
}
