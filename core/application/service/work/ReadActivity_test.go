// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	activityrepo "github.com/Jersyfi/hubtask/core/application/repository/activity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// history is the read side of the journal: what it was asked for, and what it hands back.
type history struct {
	page     activityrepo.EntryPage
	err      error
	asked    []shared.ID
	requests []activityrepo.Page
}

func (h *history) List(
	_ context.Context, itemID shared.ID, page activityrepo.Page,
) (activityrepo.EntryPage, error) {
	h.asked = append(h.asked, itemID)
	h.requests = append(h.requests, page)
	return h.page, h.err
}

type activityHarness struct {
	handler    ListActivity
	history    *history
	items      *items
	containers *containers
	authorizer *authorizer
	uow        *unitOfWork
}

func newActivityHarness() *activityHarness {
	store := &items{stored: map[shared.ID]domain.WorkItem{}}
	containerStore := &containers{stored: map[shared.ID]domain.Container{}}

	h := &activityHarness{
		history: &history{}, items: store, containers: containerStore,
		authorizer: &authorizer{}, uow: &unitOfWork{},
	}
	h.handler = ListActivity{
		History: h.history, Items: store, Containers: containerStore,
		Authorizer: h.authorizer, UnitOfWork: h.uow,
	}

	containerStore.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	containerStore.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	store.stored[taskID] = domain.WorkItem{
		ID: taskID, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemTask,
		Path: domain.RootPath(taskID), Depth: 1, Title: "Weekly shop", OrderKey: "a0",
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	return h
}

func (h *activityHarness) holding(entries ...activity.Entry) {
	h.history.page = activityrepo.EntryPage{Entries: entries}
}

func step(verb activity.Verb, at time.Time, changeSet map[string]any) activity.Entry {
	return activity.Entry{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000009e1"), TenantID: tenantID,
		ItemID: taskID, CollectionID: collectionID,
		Actor:      activity.Actor{Kind: shared.ActorUser, ID: accountID},
		Verb:       verb,
		ChangeSet:  changeSet,
		OccurredAt: at,
	}
}

// The projection every channel renders: a message code rather than a sentence, and the change set
// as the history recorded it (ADR-0011, i18n-l10n.md §1).
func TestTheHistoryIsProjectedAsCodesRatherThanSentences(t *testing.T) {
	h := newActivityHarness()
	h.holding(step(activity.ItemUpdated, now, map[string]any{
		"title": map[string]any{"from": "Milk", "to": "Oat milk"},
	}))

	out, err := h.handler.invoke(t.Context(), itemActor(), usecase.Input{"item_id": taskID.String()})
	if err != nil {
		t.Fatalf("reading the history was refused: %v", err)
	}

	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 1 {
		t.Fatalf("the page holds %d steps, want 1", len(rows))
	}
	row := rows[0]
	if row.String("code") != "activity.item_updated" {
		t.Errorf("the code is %q", row.String("code"))
	}
	if row.String("item_id") != taskID.String() {
		t.Errorf("the step is about %q", row.String("item_id"))
	}

	actor, _ := row["actor"].(map[string]any)
	if actor["type"] != string(shared.ActorUser) || actor["id"] != accountID.String() {
		t.Errorf("the actor reads %v", actor)
	}
	changeSet, _ := row["change_set"].(map[string]any)
	if changeSet["title"] == nil {
		t.Errorf("the change set reads %v", changeSet)
	}
}

// A compact step carries an empty object rather than no member at all: a client should not have to
// tell "nothing moved" from "the field is missing".
func TestACompactStepCarriesAnEmptyChangeSetRatherThanNone(t *testing.T) {
	h := newActivityHarness()
	h.holding(step(activity.ItemArchived, now, nil))

	out, err := h.handler.invoke(t.Context(), itemActor(), usecase.Input{"item_id": taskID.String()})
	if err != nil {
		t.Fatalf("reading the history was refused: %v", err)
	}

	rows, _ := out["data"].([]usecase.Output)
	changeSet, present := rows[0]["change_set"].(map[string]any)
	if !present || len(changeSet) != 0 {
		t.Errorf("the change set reads %v, want an empty object", rows[0]["change_set"])
	}
}

// The permission is asked at the whole path, because a membership held at the hub applies downwards
// (domain-model.md §3.2) - and against the entry, because the right to read the entry is the right
// to read what happened to it.
func TestTheHistoryIsAskedForAtTheWholePath(t *testing.T) {
	h := newActivityHarness()

	if _, err := h.handler.Execute(
		t.Context(), itemActor(), ListActivityQuery{ItemID: taskID}); err != nil {
		t.Fatalf("reading the history was refused: %v", err)
	}

	if len(h.authorizer.requests) != 1 {
		t.Fatalf("%d permission questions, want 1", len(h.authorizer.requests))
	}
	request := h.authorizer.requests[0]
	if request.TokenScope != itemsRead || request.TargetID != taskID {
		t.Errorf("the question reads %+v", request)
	}
	if request.Action != ActivityReadAction {
		t.Errorf("a refusal would be recorded against %s", request.Action)
	}

	want := []identity.Scope{
		identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(collectionID),
	}
	if len(request.Path) != len(want) {
		t.Fatalf("the path is %v, want %v", request.Path, want)
	}
	for i, scope := range want {
		if request.Path[i] != scope {
			t.Errorf("the path is %v, want %v", request.Path, want)
			break
		}
	}
}

// A refusal is a refusal, not an empty page: a client that may not read a history has to be able to
// tell that from an entry nothing has happened to.
func TestARefusedHistoryReadsNothing(t *testing.T) {
	h := newActivityHarness()
	h.authorizer.err = shared.ErrForbidden

	_, err := h.handler.Execute(t.Context(), itemActor(), ListActivityQuery{ItemID: taskID})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error = %v, want a refusal", err)
	}
	if len(h.history.asked) != 0 {
		t.Errorf("the history was read anyway: %v", h.history.asked)
	}
}

// The size is clamped in the application layer rather than in an adapter, because all three
// channels page and a limit enforced in one of them is a limit the other two do not have
// (api-guidelines.md §4).
func TestTheRequestedSizeIsClamped(t *testing.T) {
	cases := map[int]int{0: DefaultPageSize, -3: DefaultPageSize, 10: 10, 5000: MaxPageSize}

	for asked, want := range cases {
		h := newActivityHarness()
		if _, err := h.handler.Execute(t.Context(), itemActor(), ListActivityQuery{
			ItemID: taskID, Size: asked,
		}); err != nil {
			t.Fatalf("reading the history was refused: %v", err)
		}

		if got := h.history.requests[0].Size; got != want {
			t.Errorf("asking for %d read %d rows, want %d", asked, got, want)
		}
	}
}

// An entry that is not there is the client's mistake and says so, rather than an empty history for
// something that does not exist.
func TestTheHistoryOfAMissingEntryIsANotFound(t *testing.T) {
	h := newActivityHarness()

	_, err := h.handler.Execute(t.Context(), itemActor(), ListActivityQuery{ItemID: packageID})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error = %v, want not found", err)
	}
	if detail := shared.AsError(err).DetailCode; detail != "items.not_found" {
		t.Errorf("detail %q", detail)
	}
}

// And an empty request names the field rather than reading the history of nothing.
func TestAHistoryReadWithoutAnEntryIsRefused(t *testing.T) {
	h := newActivityHarness()

	_, err := h.handler.Execute(t.Context(), itemActor(), ListActivityQuery{})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error = %v, want a validation error", err)
	}
}

// The walk's state travels as the contract's PageInfo, so a client stops on has_more alone.
func TestThePageCarriesTheWalksState(t *testing.T) {
	h := newActivityHarness()
	h.holding(step(activity.ItemCreated, now, nil))
	h.history.page.Info = activityrepo.PageInfo{NextCursor: "opaque", HasMore: true}

	out, err := h.handler.invoke(t.Context(), itemActor(), usecase.Input{
		"item_id": taskID.String(), "cursor": "previous",
	})
	if err != nil {
		t.Fatalf("reading the history was refused: %v", err)
	}

	page, _ := out["page"].(map[string]any)
	if page["next_cursor"] != "opaque" || page["has_more"] != true {
		t.Errorf("the page reads %v", page)
	}
	// The cursor is passed through untouched: it is the adapter's, and the application layer never
	// looks inside one.
	if h.history.requests[0].Cursor != "previous" {
		t.Errorf("the cursor arrived as %q", h.history.requests[0].Cursor)
	}
}
