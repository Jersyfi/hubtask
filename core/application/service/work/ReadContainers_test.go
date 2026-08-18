// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// reader answers the plural permission question. permit says which paths come back allowed; a nil map
// allows everything, which is the ordinary case and keeps the tests that do not care about filtering
// from having to say so.
type reader struct {
	err     error
	permit  map[shared.ID]bool
	asked   [][]identity.Scope
	request access.Request
}

func (r *reader) Permitted(
	_ context.Context, _ appshared.ActorContext, request access.Request, paths [][]identity.Scope,
) ([]bool, error) {
	r.asked, r.request = paths, request
	if r.err != nil {
		return nil, r.err
	}

	allowed := make([]bool, len(paths))
	for i, path := range paths {
		// The last scope of a path is the container itself, which is what the fixture keys on.
		allowed[i] = r.permit == nil || r.permit[path[len(path)-1].ID]
	}
	return allowed, nil
}

func hubFixture(id shared.ID, name, orderKey string) domain.Container {
	return domain.Container{
		ID: id, TenantID: tenantID, Type: domain.ContainerHub, Name: name,
		OrderKey: orderKey, CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
}

func collectionFixture(id, parent shared.ID, name string) domain.Container {
	container := hubFixture(id, name, "a0")
	container.Type, container.ParentID = domain.ContainerCollection, parent
	return container
}

func TestGetContainerReturnsTheContainer(t *testing.T) {
	collectionID := shared.MustParseID("0192f000-0000-7000-8000-000000000201")
	store := &containers{stored: map[shared.ID]domain.Container{
		collectionID: collectionFixture(collectionID, hubID, "Groceries"),
	}}
	guard, uow := &authorizer{}, &unitOfWork{}

	got, err := GetContainer{Containers: store, Authorizer: guard, UnitOfWork: uow}.
		Execute(t.Context(), actorFixture(), GetContainerQuery{ContainerID: collectionID})
	if err != nil {
		t.Fatalf("reading the collection: %v", err)
	}
	if got.ID != collectionID || got.Name != "Groceries" {
		t.Errorf("read back %+v", got)
	}

	// The acceptance criterion: read-only throughout. A read that opened a write transaction would
	// pin every list to the primary.
	if uow.writes != 0 {
		t.Errorf("the read opened %d write transactions", uow.writes)
	}
	if uow.reads == 0 {
		t.Error("the read opened no transaction at all")
	}
}

// The path is what decides the answer, so it is what the test is about: a membership held at the hub
// applies to the collection inside it, and a path that named only the collection would refuse
// somebody who does have the right (domain-model.md §3.2).
func TestGetContainerAsksAboutTheWholePath(t *testing.T) {
	collectionID := shared.MustParseID("0192f000-0000-7000-8000-000000000202")

	cases := map[string]struct {
		container domain.Container
		want      []identity.Scope
	}{
		"a collection": {
			container: collectionFixture(collectionID, hubID, "Groceries"),
			want: []identity.Scope{
				identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(collectionID),
			},
		},
		// A hub's own scope is a hub scope, not a collection scope. Getting that wrong would make a
		// hub-scoped membership fail to authorise a read of the very hub it is held on.
		"a hub": {
			container: hubFixture(hubID, "Private", "a0"),
			want:      []identity.Scope{identity.TenantScope(), identity.HubScope(hubID)},
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			store := &containers{stored: map[shared.ID]domain.Container{c.container.ID: c.container}}
			guard := &authorizer{}

			if _, err := (GetContainer{Containers: store, Authorizer: guard, UnitOfWork: &unitOfWork{}}).
				Execute(t.Context(), actorFixture(), GetContainerQuery{ContainerID: c.container.ID}); err != nil {
				t.Fatalf("reading: %v", err)
			}

			if len(guard.requests) != 1 {
				t.Fatalf("%d permission questions asked, want 1", len(guard.requests))
			}
			request := guard.requests[0]
			if request.Permission != service.PermissionRead {
				t.Errorf("asked for %s, want READ", request.Permission)
			}
			if request.TokenScope != containersRead {
				t.Errorf("asked for the scope %q, want %q", request.TokenScope, containersRead)
			}
			assertPath(t, request.Path, c.want)
		})
	}
}

func TestGetContainerRefusedReturnsNothing(t *testing.T) {
	store := &containers{stored: map[shared.ID]domain.Container{hubID: hubFixture(hubID, "Private", "a0")}}
	refusal := shared.ErrForbidden.WithDetail("access.not_permitted")

	got, err := GetContainer{
		Containers: store, Authorizer: &authorizer{err: refusal}, UnitOfWork: &unitOfWork{},
	}.Execute(t.Context(), actorFixture(), GetContainerQuery{ContainerID: hubID})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a refused read answered %v", err)
	}
	if got.ID != "" {
		t.Errorf("a refused read still returned %s", got.ID)
	}
}

func TestGetContainerReportsAMissingOneAsNotFound(t *testing.T) {
	_, err := GetContainer{
		Containers: &containers{stored: map[shared.ID]domain.Container{}},
		Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
	}.Execute(t.Context(), actorFixture(), GetContainerQuery{
		ContainerID: shared.MustParseID("0192f000-0000-7000-8000-0000000002ff"),
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing container answered %v", err)
	}
}

// A level under a named hub is one question about that hub, and the answer covers every row in it.
func TestListingUnderAHubIsOneCheckAtThatHub(t *testing.T) {
	store := &containers{page: repository.ContainerPage{
		Containers: []domain.Container{
			collectionFixture(shared.MustParseID("0192f000-0000-7000-8000-000000000211"), hubID, "One"),
		},
	}}
	guard, view := &authorizer{}, &reader{}

	page, err := ListContainers{
		Containers: store, Authorizer: guard, Reader: view, UnitOfWork: &unitOfWork{},
	}.Execute(t.Context(), actorFixture(), ListContainersQuery{ParentID: hubID})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page.Containers) != 1 {
		t.Errorf("the page holds %d rows, want 1", len(page.Containers))
	}

	if len(guard.requests) != 1 {
		t.Fatalf("%d permission questions asked, want 1", len(guard.requests))
	}
	assertPath(t, guard.requests[0].Path, []identity.Scope{
		identity.TenantScope(), identity.HubScope(hubID),
	})
	// The plural form is for the hub level only. Asking it here would be one membership read per page
	// on top of a check that already covers the whole level.
	if view.asked != nil {
		t.Error("an anchored list also asked the plural permission question")
	}
	if store.asked.ParentID != hubID {
		t.Errorf("the repository was asked for the level under %s", store.asked.ParentID)
	}
}

func TestListingUnderAHubRefusedReturnsNothing(t *testing.T) {
	store := &containers{page: repository.ContainerPage{
		Containers: []domain.Container{hubFixture(hubID, "Private", "a0")},
	}}
	refusal := shared.ErrForbidden.WithDetail("access.not_permitted")

	page, err := ListContainers{
		Containers: store, Authorizer: &authorizer{err: refusal}, Reader: &reader{},
		UnitOfWork: &unitOfWork{},
	}.Execute(t.Context(), actorFixture(), ListContainersQuery{ParentID: hubID})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a refused list answered %v", err)
	}
	if len(page.Containers) != 0 {
		t.Error("a refused list still returned rows")
	}
	// A refusal, not an empty page: the client named the hub, so "you may not" is the honest answer
	// and an empty page would be indistinguishable from an empty hub.
	if store.asked.ParentID != "" {
		t.Error("the repository was queried despite the refusal")
	}
}

// The hub level is anchored to nothing. Checking the tenant scope would refuse everybody whose
// membership sits on a hub, so the page is narrowed instead of refused.
func TestTheHubLevelIsNarrowedRatherThanRefused(t *testing.T) {
	mine := shared.MustParseID("0192f000-0000-7000-8000-000000000221")
	somebodyElses := shared.MustParseID("0192f000-0000-7000-8000-000000000222")

	store := &containers{page: repository.ContainerPage{
		Containers: []domain.Container{
			hubFixture(mine, "Mine", "a0"),
			hubFixture(somebodyElses, "Theirs", "a1"),
		},
		Info: repository.PageInfo{NextCursor: "opaque", HasMore: true},
	}}
	guard := &authorizer{}
	view := &reader{permit: map[shared.ID]bool{mine: true}}

	page, err := ListContainers{
		Containers: store, Authorizer: guard, Reader: view, UnitOfWork: &unitOfWork{},
	}.Execute(t.Context(), actorFixture(), ListContainersQuery{})
	if err != nil {
		t.Fatalf("listing the hubs: %v", err)
	}

	if len(guard.requests) != 0 {
		t.Error("the hub level asked the single permission question, which would refuse hub members")
	}
	if len(page.Containers) != 1 || page.Containers[0].ID != mine {
		t.Fatalf("the page holds %+v, want only %s", page.Containers, mine)
	}
	if view.request.Permission != service.PermissionRead || view.request.TokenScope != containersRead {
		t.Errorf("the plural question asked for %s / %q", view.request.Permission, view.request.TokenScope)
	}

	// The cursor is a boundary in the scanned set, not in the filtered one. Narrowing it to the last
	// visible row would skip everything between it and the last row actually read.
	if page.Info.NextCursor != "opaque" || !page.Info.HasMore {
		t.Errorf("filtering changed the walk: %+v", page.Info)
	}
}

func TestAFailedPermissionReadIsNotAnEmptyPage(t *testing.T) {
	store := &containers{page: repository.ContainerPage{
		Containers: []domain.Container{hubFixture(hubID, "Private", "a0")},
	}}
	broken := errors.New("the membership table is unreachable")

	_, err := ListContainers{
		Containers: store, Authorizer: &authorizer{}, Reader: &reader{err: broken},
		UnitOfWork: &unitOfWork{},
	}.Execute(t.Context(), actorFixture(), ListContainersQuery{})
	if !errors.Is(err, broken) {
		t.Errorf("a failed membership read answered %v - an empty page would claim the actor sees nothing", err)
	}
}

// The contract's limits, enforced in the application layer so that all three channels have them
// (api-guidelines.md §4).
func TestThePageSizeIsClampedToTheContract(t *testing.T) {
	cases := map[string]struct {
		asked, want int
	}{
		"unset defaults":      {0, DefaultPageSize},
		"negative defaults":   {-5, DefaultPageSize},
		"one is honoured":     {1, 1},
		"inside the range":    {77, 77},
		"the maximum":         {MaxPageSize, MaxPageSize},
		"beyond it is capped": {5000, MaxPageSize},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			store := &containers{}
			if _, err := (ListContainers{
				Containers: store, Authorizer: &authorizer{}, Reader: &reader{},
				UnitOfWork: &unitOfWork{},
			}).Execute(t.Context(), actorFixture(), ListContainersQuery{
				ParentID: hubID, Size: c.asked,
			}); err != nil {
				t.Fatalf("listing: %v", err)
			}
			if store.asked.Page.Size != c.want {
				t.Errorf("a size of %d reached the repository as %d, want %d",
					c.asked, store.asked.Page.Size, c.want)
			}
		})
	}
}

func TestTheListQueryReachesTheRepositoryWhole(t *testing.T) {
	store := &containers{}

	if _, err := (ListContainers{
		Containers: store, Authorizer: &authorizer{}, Reader: &reader{}, UnitOfWork: &unitOfWork{},
	}).Execute(t.Context(), actorFixture(), ListContainersQuery{
		ParentID: hubID, Type: domain.ContainerCollection, IncludeArchived: true,
		Cursor: "an opaque cursor", Size: 10,
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}

	asked := store.asked
	if asked.Type != domain.ContainerCollection || !asked.IncludeArchived {
		t.Errorf("the filters reached the repository as %+v", asked)
	}
	// Passed through untouched. The application layer does not read a cursor: it is the adapter's
	// value, signed with a key this layer must not hold (security.md §8).
	if asked.Page.Cursor != "an opaque cursor" {
		t.Errorf("the cursor reached the repository as %q", asked.Page.Cursor)
	}
}

// The page shape of api-guidelines.md §4, which every channel returns.
func TestThePagedOutputCarriesTheWalkState(t *testing.T) {
	cases := map[string]struct {
		info       repository.PageInfo
		wantCursor any
		wantMore   bool
	}{
		"a page with a successor": {repository.PageInfo{NextCursor: "opaque", HasMore: true}, "opaque", true},
		// Null rather than absent: a client reads the field either way, and an omitted one would make
		// "no more pages" indistinguishable from "this server does not page".
		"the last page": {repository.PageInfo{}, nil, false},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			store := &containers{page: repository.ContainerPage{
				Containers: []domain.Container{hubFixture(hubID, "Private", "a0")},
				Info:       c.info,
			}}

			out, err := ListContainers{
				Containers: store, Authorizer: &authorizer{}, Reader: &reader{},
				UnitOfWork: &unitOfWork{},
			}.invoke(t.Context(), actorFixture(), usecase.Input{"parent_id": hubID.String()})
			if err != nil {
				t.Fatalf("listing: %v", err)
			}

			page, ok := out["page"].(map[string]any)
			if !ok {
				t.Fatalf("the output carries no page: %+v", out)
			}
			if page["next_cursor"] != c.wantCursor {
				t.Errorf("next_cursor is %v, want %v", page["next_cursor"], c.wantCursor)
			}
			if page["has_more"] != c.wantMore {
				t.Errorf("has_more is %v, want %v", page["has_more"], c.wantMore)
			}
			data, ok := out["data"].([]usecase.Output)
			if !ok || len(data) != 1 {
				t.Fatalf("the output's data is %+v", out["data"])
			}
		})
	}
}

// Both read descriptors are read-only and destructive of nothing, which is what an MCP client reads
// before deciding whether to ask for confirmation (ai-first.md).
func TestTheReadDescriptorsSayTheyRead(t *testing.T) {
	for _, descriptor := range []usecase.Descriptor{
		GetContainer{}.Descriptor(), ListContainers{}.Descriptor(),
		GetWorkItem{}.Descriptor(), ListWorkItems{}.Descriptor(),
	} {
		t.Run(descriptor.Name, func(t *testing.T) {
			if !descriptor.ReadOnly {
				t.Error("is not declared read-only")
			}
			if descriptor.Destructive {
				t.Error("is declared destructive")
			}
			// An ordinary read is not an auditable event (audit.md §4 lists none), and the action is
			// named all the same: a refused read is recorded against it.
			if descriptor.Audit.Required {
				t.Error("claims every read owes an audit entry")
			}
			if descriptor.Audit.Action == "" {
				t.Error("names no action for the authorisation service to record a refusal against")
			}
		})
	}
}

func assertPath(t *testing.T, got, want []identity.Scope) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("the path is %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scope %d of the path is %+v, want %+v", i, got[i], want[i])
		}
	}
}

func actorFixture() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: accountID,
	}
}
