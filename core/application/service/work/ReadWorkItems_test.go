// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

var (
	readCollectionID = shared.MustParseID("0192f000-0000-7000-8000-000000000301")
	readItemID       = shared.MustParseID("0192f000-0000-7000-8000-000000000302")
)

func itemFixture(id, collection shared.ID, title string) domain.WorkItem {
	return domain.WorkItem{
		ID: id, TenantID: tenantID, CollectionID: collection, Type: domain.ItemTask,
		Path: domain.RootPath(id), Depth: 1, Title: title, OrderKey: "a0",
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
}

// readFixture wires a collection inside a hub, with one item in it.
func readFixture() (*items, *containers) {
	return &items{stored: map[shared.ID]domain.WorkItem{
			readItemID: itemFixture(readItemID, readCollectionID, "Buy milk"),
		}},
		&containers{stored: map[shared.ID]domain.Container{
			readCollectionID: collectionFixture(readCollectionID, hubID, "Groceries"),
		}}
}

func TestGetWorkItemReturnsTheItem(t *testing.T) {
	store, containerStore := readFixture()
	uow := &unitOfWork{}

	got, err := GetWorkItem{
		Items: store, Containers: containerStore, Authorizer: &authorizer{}, UnitOfWork: uow,
	}.Execute(t.Context(), actorFixture(), GetWorkItemQuery{ItemID: readItemID})
	if err != nil {
		t.Fatalf("reading the item: %v", err)
	}
	if got.ID != readItemID || got.Title != "Buy milk" {
		t.Errorf("read back %+v", got)
	}
	if uow.writes != 0 {
		t.Errorf("the read opened %d write transactions", uow.writes)
	}
}

// The item names its collection and the collection names the hub, and both are on the path: a
// membership at the hub authorises reading an item three levels down (domain-model.md §3.2).
func TestGetWorkItemAsksAboutThePathThroughTheCollection(t *testing.T) {
	store, containerStore := readFixture()
	guard := &authorizer{}

	if _, err := (GetWorkItem{
		Items: store, Containers: containerStore, Authorizer: guard, UnitOfWork: &unitOfWork{},
	}).Execute(t.Context(), actorFixture(), GetWorkItemQuery{ItemID: readItemID}); err != nil {
		t.Fatalf("reading: %v", err)
	}

	if len(guard.requests) != 1 {
		t.Fatalf("%d permission questions asked, want 1", len(guard.requests))
	}
	request := guard.requests[0]
	assertPath(t, request.Path, []identity.Scope{
		identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(readCollectionID),
	})
	if request.Permission != service.PermissionRead || request.TokenScope != itemsRead {
		t.Errorf("asked for %s / %q", request.Permission, request.TokenScope)
	}
	// The refusal names the item, which is what was asked for.
	if request.TargetID != readItemID || request.TargetType != itemTarget {
		t.Errorf("the refusal would name %s/%s", request.TargetType, request.TargetID)
	}
}

func TestGetWorkItemRefusedReturnsNothing(t *testing.T) {
	store, containerStore := readFixture()

	got, err := GetWorkItem{
		Items: store, Containers: containerStore, UnitOfWork: &unitOfWork{},
		Authorizer: &authorizer{err: shared.ErrForbidden.WithDetail("access.not_permitted")},
	}.Execute(t.Context(), actorFixture(), GetWorkItemQuery{ItemID: readItemID})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a refused read answered %v", err)
	}
	if got.ID != "" {
		t.Errorf("a refused read still returned %s", got.ID)
	}
}

func TestGetWorkItemReportsAMissingOneAsNotFound(t *testing.T) {
	_, containerStore := readFixture()

	_, err := GetWorkItem{
		Items:      &items{stored: map[shared.ID]domain.WorkItem{}},
		Containers: containerStore, Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
	}.Execute(t.Context(), actorFixture(), GetWorkItemQuery{ItemID: readItemID})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing item answered %v", err)
	}
}

// An item whose collection is gone is not the client's mistake and not something it can act on. A
// tenant-scoped foreign key makes it unreachable (ADR-0024), which is why reaching it is a defect and
// not a 404 for an item that does exist.
func TestAnItemWithoutItsCollectionIsAnInternalError(t *testing.T) {
	store, _ := readFixture()

	_, err := GetWorkItem{
		Items: store, Containers: &containers{stored: map[shared.ID]domain.Container{}},
		Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
	}.Execute(t.Context(), actorFixture(), GetWorkItemQuery{ItemID: readItemID})
	if !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("a dangling collection answered %v, want an internal error", err)
	}
	if got := shared.AsError(err).DetailCode; got != "items.collection_missing" {
		t.Errorf("the detail code is %q", got)
	}
}

// The item list is anchored, so one check at the collection covers the whole page - unlike the hub
// level of ListContainers, which has nothing to anchor to.
func TestListingItemsIsOneCheckAtTheCollection(t *testing.T) {
	store, containerStore := readFixture()
	store.page = repository.ItemPage{
		Items: []domain.WorkItem{itemFixture(readItemID, readCollectionID, "Buy milk")},
	}
	guard := &authorizer{}

	page, err := ListWorkItems{
		Items: store, Containers: containerStore, Authorizer: guard, UnitOfWork: &unitOfWork{},
	}.Execute(t.Context(), actorFixture(), ListWorkItemsQuery{CollectionID: readCollectionID})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page.Items) != 1 {
		t.Errorf("the page holds %d rows, want 1", len(page.Items))
	}

	if len(guard.requests) != 1 {
		t.Fatalf("%d permission questions asked, want 1", len(guard.requests))
	}
	assertPath(t, guard.requests[0].Path, []identity.Scope{
		identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(readCollectionID),
	})
}

func TestListingItemsRefusedDoesNotQueryTheRepository(t *testing.T) {
	store, containerStore := readFixture()

	_, err := ListWorkItems{
		Items: store, Containers: containerStore, UnitOfWork: &unitOfWork{},
		Authorizer: &authorizer{err: shared.ErrForbidden.WithDetail("access.not_permitted")},
	}.Execute(t.Context(), actorFixture(), ListWorkItemsQuery{CollectionID: readCollectionID})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a refused list answered %v", err)
	}
	if store.asked.CollectionID != "" {
		t.Error("the repository was queried despite the refusal")
	}
}

func TestListingItemsNeedsACollection(t *testing.T) {
	store, containerStore := readFixture()

	_, err := ListWorkItems{
		Items: store, Containers: containerStore, Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
	}.Execute(t.Context(), actorFixture(), ListWorkItemsQuery{})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a list with no collection answered %v", err)
	}
	if got := shared.AsError(err).DetailCode; got != "items.collection_id_required" {
		t.Errorf("the detail code is %q", got)
	}
}

func TestListingItemsInAMissingCollectionIsNotFound(t *testing.T) {
	store, _ := readFixture()

	_, err := ListWorkItems{
		Items: store, Containers: &containers{stored: map[shared.ID]domain.Container{}},
		Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
	}.Execute(t.Context(), actorFixture(), ListWorkItemsQuery{CollectionID: readCollectionID})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a missing collection answered %v", err)
	}
	if got := shared.AsError(err).DetailCode; got != "items.collection_not_found" {
		t.Errorf("the detail code is %q", got)
	}
}

func TestTheItemListQueryReachesTheRepositoryWhole(t *testing.T) {
	store, containerStore := readFixture()

	if _, err := (ListWorkItems{
		Items: store, Containers: containerStore, Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
	}).Execute(t.Context(), actorFixture(), ListWorkItemsQuery{
		CollectionID: readCollectionID, ParentID: readItemID, IncludeArchived: true,
		Cursor: "an opaque cursor", Size: 500,
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}

	asked := store.asked
	if asked.CollectionID != readCollectionID || asked.ParentID != readItemID {
		t.Errorf("the level reached the repository as %+v", asked)
	}
	if !asked.IncludeArchived || asked.Page.Cursor != "an opaque cursor" {
		t.Errorf("the filters reached the repository as %+v", asked)
	}
	if asked.Page.Size != MaxPageSize {
		t.Errorf("a size of 500 reached the repository as %d, want %d", asked.Page.Size, MaxPageSize)
	}
}

// The projection is what every channel returns, so a state a create never produces has to survive it:
// a completed item that reported itself open is a lie a client acts on.
func TestTheProjectionCarriesTheStateACreateNeverProduces(t *testing.T) {
	completedAt := now.Add(time.Hour)
	archivedAt := now.Add(2 * time.Hour)

	item := itemFixture(readItemID, readCollectionID, "Buy milk")
	item.Completion = domain.Completion{
		IsCompleted: true, CompletedAt: &completedAt, CompletedBy: accountID,
	}
	item.ArchivedAt = &archivedAt

	out := itemOutput(item)
	completion, ok := out["completion"].(map[string]any)
	if !ok {
		t.Fatalf("the projection carries no completion: %+v", out)
	}
	if completion["is_completed"] != true {
		t.Error("a completed item reports itself open")
	}
	if completion["completed_at"] != completedAt {
		t.Errorf("completed_at is %v, want %v", completion["completed_at"], completedAt)
	}
	if completion["completed_by"] != accountID.String() {
		t.Errorf("completed_by is %v, want %s", completion["completed_by"], accountID)
	}
	if out["archived_at"] != archivedAt {
		t.Errorf("archived_at is %v, want %v", out["archived_at"], archivedAt)
	}

	// Present as null rather than absent, so a client can rely on the field being there.
	if out["deleted_at"] != nil {
		t.Errorf("deleted_at is %v, want null", out["deleted_at"])
	}
	if _, present := out["deleted_at"]; !present {
		t.Error("deleted_at is missing rather than null")
	}
}

func TestTheContainerProjectionCarriesTheLifecycle(t *testing.T) {
	archivedAt := now.Add(time.Hour)

	container := hubFixture(hubID, "Private", "a0")
	container.ArchivedAt = &archivedAt

	out := containerOutput(container)
	if out["archived_at"] != archivedAt {
		t.Errorf("archived_at is %v, want %v", out["archived_at"], archivedAt)
	}
	if value, present := out["deleted_at"]; !present || value != nil {
		t.Errorf("deleted_at is %v (present=%v), want a null that is there", value, present)
	}
}

// The catalogue's untyped input is where every channel meets the typed query, so a malformed
// identifier has to be refused there rather than reaching a repository.
func TestAMalformedIdentifierIsRefusedAtTheCatalogueBoundary(t *testing.T) {
	store, containerStore := readFixture()
	handler := ListWorkItems{
		Items: store, Containers: containerStore, Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
	}

	_, err := handler.invoke(t.Context(), actorFixture(), usecase.Input{
		"collection_id": readCollectionID.String(),
		"parent_id":     "not-an-identifier",
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a malformed parent_id answered %v", err)
	}
	if store.asked.CollectionID != "" {
		t.Error("the repository was queried with a malformed identifier in hand")
	}
}

// `expand=labels`: absent unless asked for, and an empty array when the entry carries none. The
// difference matters - a client that could not tell "no labels" from "I did not ask" would render
// an entry without its chips and have no way to know why (B-09).
func TestExpandingTheLabelsOfOneEntry(t *testing.T) {
	store, containerStore := readFixture()
	carried := newItemLabels()
	carried.carried[readItemID] = []shared.ID{urgentLabel}

	handler := GetWorkItem{
		Items: store, ItemLabels: carried, Containers: containerStore,
		Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
	}

	t.Run("asked for", func(t *testing.T) {
		out, err := handler.invoke(t.Context(), actorFixture(), usecase.Input{
			"item_id": readItemID.String(), "expand_labels": true,
		})
		if err != nil {
			t.Fatalf("reading the item: %v", err)
		}
		ids, asked := out["label_ids"].([]string)
		if !asked || len(ids) != 1 || ids[0] != urgentLabel.String() {
			t.Errorf("label_ids is %v", out["label_ids"])
		}
	})

	t.Run("not asked for", func(t *testing.T) {
		out, err := handler.invoke(t.Context(), actorFixture(), usecase.Input{
			"item_id": readItemID.String(),
		})
		if err != nil {
			t.Fatalf("reading the item: %v", err)
		}
		if _, present := out["label_ids"]; present {
			t.Errorf("label_ids came back although nobody asked: %v", out["label_ids"])
		}
	})

	t.Run("asked for on an entry that carries none", func(t *testing.T) {
		delete(carried.carried, readItemID)

		out, err := handler.invoke(t.Context(), actorFixture(), usecase.Input{
			"item_id": readItemID.String(), "expand_labels": true,
		})
		if err != nil {
			t.Fatalf("reading the item: %v", err)
		}
		ids, asked := out["label_ids"].([]string)
		if !asked || len(ids) != 0 {
			t.Errorf("label_ids is %v, want an empty list", out["label_ids"])
		}
	})
}

// One query for the whole page rather than one per entry: a list of fifty entries is fifty round
// trips the other way round.
func TestExpandingTheLabelsOfAPageAsksOnce(t *testing.T) {
	store, containerStore := readFixture()
	second := shared.MustParseID("0192f000-0000-7000-8000-000000000303")
	store.page = repository.ItemPage{Items: []domain.WorkItem{
		itemFixture(readItemID, readCollectionID, "Buy milk"),
		itemFixture(second, readCollectionID, "Buy bread"),
	}}

	carried := newItemLabels()
	carried.carried[second] = []shared.ID{urgentLabel}

	out, err := ListWorkItems{
		Items: store, ItemLabels: carried, Containers: containerStore,
		Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
	}.invoke(t.Context(), actorFixture(), usecase.Input{
		"collection_id": readCollectionID.String(), "expand_labels": true,
	})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}

	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 2 {
		t.Fatalf("%d rows, want 2", len(rows))
	}
	first, _ := rows[0]["label_ids"].([]string)
	if len(first) != 0 {
		t.Errorf("the first entry carries %v", first)
	}
	last, _ := rows[1]["label_ids"].([]string)
	if len(last) != 1 || last[0] != urgentLabel.String() {
		t.Errorf("the second entry carries %v", last)
	}
}

// A refused read pays for no second query: the labels are read after the permission check.
func TestARefusedReadDoesNotAskForLabels(t *testing.T) {
	store, containerStore := readFixture()
	carried := newItemLabels()
	carried.listErr = errors.New("the labels were read despite the refusal")

	_, err := GetWorkItem{
		Items: store, ItemLabels: carried, Containers: containerStore,
		Authorizer: &authorizer{err: shared.ErrForbidden}, UnitOfWork: &unitOfWork{},
	}.invoke(t.Context(), actorFixture(), usecase.Input{
		"item_id": readItemID.String(), "expand_labels": true,
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the refusal did not come back: %v", err)
	}
}
