// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// ranking wires a harness with two hubs and a collection in the first - the same shape the move
// cases start from, because the operations are siblings and the difference should be visible.
func ranking(t *testing.T) *containerHarness {
	t.Helper()

	h := newContainerHarness()
	h.withHub(hubID, "Private")
	h.withHub(otherHubID, "Work")
	h.withCollection()
	return h
}

// hub stores another hub at the tenant level, so that a test about ranking hubs has something to
// rank against.
func (h *containerHarness) hubAt(id shared.ID, name, orderKey string) domain.Container {
	container := domain.Container{
		ID: id, TenantID: tenantID, Type: domain.ContainerHub, Name: name, OrderKey: orderKey,
		CompletionPolicy: domain.CompletionManual, CreatedBy: accountID, CreatedAt: now,
		UpdatedAt: now, Version: 1,
	}
	h.containers.stored[id] = container
	return container
}

// The reason this use case exists: a hub sits in nothing, so `:move` cannot rank it, and without
// this its `order_key` is a field the API answers and nothing can change.
func TestAHubIsRankedAmongTheTenantsHubs(t *testing.T) {
	h := ranking(t)
	h.hubAt(booksID, "Reading", "a5")

	ranked, err := ReorderContainer{Writer: h.writer}.Execute(t.Context(), actor(), ReorderContainerCommand{
		ContainerID: hubID, BeforeContainerID: booksID,
	})
	if err != nil {
		t.Fatalf("ranking a hub was refused: %v", err)
	}

	if ranked.Type != domain.ContainerHub {
		t.Fatalf("type %s, want a hub", ranked.Type)
	}
	if ranked.OrderKey == "a0" {
		t.Error("the rank did not change")
	}
	if !ranked.ParentID.IsZero() {
		t.Errorf("parent %s, want none - a hub sits in the tenant (I-C1)", ranked.ParentID)
	}
	if len(h.containers.written) != 1 || h.containers.written[0].method != "placement" {
		t.Fatalf("unexpected writes: %+v", h.containers.written)
	}
}

func TestACollectionIsRankedAmongItsHubsCollections(t *testing.T) {
	h := ranking(t)
	h.sibling(booksID, hubID, "Books", "a5")

	ranked, err := ReorderContainer{Writer: h.writer}.Execute(t.Context(), actor(), ReorderContainerCommand{
		ContainerID: shoppingID, BeforeContainerID: booksID,
	})
	if err != nil {
		t.Fatalf("ranking a collection was refused: %v", err)
	}
	if ranked.ParentID != hubID {
		t.Errorf("parent %s, want the hub it was already in - a reorder never moves", ranked.ParentID)
	}
}

// The whole write, and what it owes: the row, the event, the change log entry and the audit entry.
func TestRankingWritesTheRowTheEventTheChangeAndTheEntry(t *testing.T) {
	h := ranking(t)
	h.sibling(booksID, hubID, "Books", "a5")

	if _, err := (ReorderContainer{Writer: h.writer}).Execute(
		t.Context(), actor(), ReorderContainerCommand{ContainerID: shoppingID, BeforeContainerID: booksID},
	); err != nil {
		t.Fatalf("ranking was refused: %v", err)
	}

	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ContainerMoved {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}
	// A consumer recognises a reorder by the parent being unchanged, which is what the event's own
	// contract says. A second event type would be a second thing for every consumer to learn.
	payload := h.events.appended[0].Payload
	if payload["from_parent_id"] != payload["parent_id"] {
		t.Errorf("from_parent_id %v and parent_id %v differ - this reads as a move",
			payload["from_parent_id"], payload["parent_id"])
	}
	if len(h.changes.recorded) == 0 {
		t.Error("no change log entry - an offline client cannot learn the new rank")
	}
	if len(h.audit.entries) != 1 {
		t.Fatalf("unexpected audit entries: %+v", h.audit.entries)
	}
	if h.audit.entries[0].Action != ContainerMovedAction {
		t.Errorf("audit action %s, want %s", h.audit.entries[0].Action, ContainerMovedAction)
	}
}

// The rank is a fractional index, and that is not decoration: it is what lets two offline devices
// insert into one list without either one's order being discarded (offline-sync.md §4.2).
func TestRankingRenumbersNoNeighbour(t *testing.T) {
	h := ranking(t)
	before := h.sibling(booksID, hubID, "Books", "a5")
	after := h.sibling(travelID, hubID, "Travel", "a7")

	if _, err := (ReorderContainer{Writer: h.writer}).Execute(
		t.Context(), actor(), ReorderContainerCommand{ContainerID: shoppingID, BeforeContainerID: travelID},
	); err != nil {
		t.Fatalf("ranking was refused: %v", err)
	}

	if len(h.containers.written) != 1 {
		t.Fatalf("%d writes, want one - a neighbour was rewritten", len(h.containers.written))
	}
	if h.containers.stored[booksID].OrderKey != before.OrderKey {
		t.Error("a neighbour's rank was rewritten")
	}
	if h.containers.stored[travelID].OrderKey != after.OrderKey {
		t.Error("a neighbour's rank was rewritten")
	}
}

func TestASiblingThatIsNotAtTheLevelIsRefused(t *testing.T) {
	h := ranking(t)
	// A collection of the *other* hub. It exists, and it is not in this level.
	h.sibling(booksID, otherHubID, "Books", "a5")

	_, err := ReorderContainer{Writer: h.writer}.Execute(t.Context(), actor(), ReorderContainerCommand{
		ContainerID: shoppingID, BeforeContainerID: booksID,
	})
	// Its own answer rather than a silent append: a client that asked for a position and got the
	// end of the list has been ignored.
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want a validation failure", err)
	}
}

func TestRankingAnArchivedContainerIsRefused(t *testing.T) {
	h := ranking(t)
	archived := h.containers.stored[shoppingID]
	archived.ArchivedAt = &now
	h.containers.stored[shoppingID] = archived

	_, err := ReorderContainer{Writer: h.writer}.Execute(t.Context(), actor(), ReorderContainerCommand{
		ContainerID: shoppingID,
	})
	// An archived container is read-only (I-C3), and a rank is a write like any other.
	if err == nil {
		t.Fatal("an archived container was reranked")
	}
	if len(h.containers.written) != 0 {
		t.Errorf("a refused request wrote something: %+v", h.containers.written)
	}
}

func TestRankingAsksThePermissionOfOneLevelOnly(t *testing.T) {
	h := ranking(t)
	h.sibling(booksID, hubID, "Books", "a5")

	if _, err := (ReorderContainer{Writer: h.writer}).Execute(
		t.Context(), actor(), ReorderContainerCommand{ContainerID: shoppingID, BeforeContainerID: booksID},
	); err != nil {
		t.Fatalf("ranking was refused: %v", err)
	}

	// One, where a move asks two: nothing leaves a level here, so there is no second hub being
	// reached into.
	if len(h.authorizer.requests) != 1 {
		t.Fatalf("%d permission questions, want one: %+v", len(h.authorizer.requests), h.authorizer.requests)
	}
}

func TestARefusedRankingWritesNothing(t *testing.T) {
	h := ranking(t)
	h.authorizer.err = shared.ErrForbidden

	if _, err := (ReorderContainer{Writer: h.writer}).Execute(
		t.Context(), actor(), ReorderContainerCommand{ContainerID: shoppingID},
	); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want a refusal", err)
	}

	if len(h.containers.written) != 0 || len(h.events.appended) != 0 {
		t.Error("a refused request wrote something")
	}
	// The permission question is asked before the transaction, because a refusal writes an audit
	// entry and an entry written inside it would be rolled back with the refusal (audit.md §7).
	if h.uow.writes != 0 {
		t.Errorf("%d write transactions, want none - the refusal came after one was opened", h.uow.writes)
	}
}

func TestRepeatingARankWritesNothingTheSecondTime(t *testing.T) {
	h := ranking(t)
	h.sibling(booksID, hubID, "Books", "a5")
	command := ReorderContainerCommand{ContainerID: shoppingID, BeforeContainerID: booksID}

	if _, err := (ReorderContainer{Writer: h.writer}).Execute(t.Context(), actor(), command); err != nil {
		t.Fatalf("the first ranking was refused: %v", err)
	}
	if _, err := (ReorderContainer{Writer: h.writer}).Execute(t.Context(), actor(), command); err != nil {
		t.Fatalf("the second ranking was refused: %v", err)
	}

	// Asking for the position it already holds writes nothing, spends no version and announces
	// nothing - the same idempotence a move that lands where it started has.
	if len(h.containers.written) != 1 {
		t.Errorf("%d writes, want one: %+v", len(h.containers.written), h.containers.written)
	}
	if len(h.events.appended) != 1 {
		t.Errorf("%d events, want one", len(h.events.appended))
	}
}

func TestAContainerOfAnotherTenantIsNotFound(t *testing.T) {
	h := ranking(t)

	// Nothing is stored under this identifier, which is what a repository scoped by
	// `SET LOCAL app.tenant_id` answers for a row of another tenant (ADR-0010). The answer is the
	// same as for a container that does not exist, deliberately: telling the two apart would
	// confirm that an identifier is in use elsewhere (multi-tenancy.md §2).
	_, err := ReorderContainer{Writer: h.writer}.Execute(t.Context(), actor(), ReorderContainerCommand{
		ContainerID: shared.MustParseID("0192f000-0000-7000-8000-0000000000ff"),
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error %v, want not found", err)
	}
	if len(h.containers.written) != 0 {
		t.Error("a request for a foreign container wrote something")
	}
}

func TestAMissingContainerIdIsNamedRatherThanLookedUp(t *testing.T) {
	h := ranking(t)

	_, err := ReorderContainer{Writer: h.writer}.Execute(t.Context(), actor(), ReorderContainerCommand{})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want a validation failure", err)
	}
}
