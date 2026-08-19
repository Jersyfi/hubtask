// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// moving wires a harness with two hubs and a collection in the first, which is the shape every
// case below starts from.
func moving(t *testing.T) *containerHarness {
	t.Helper()

	h := newContainerHarness()
	h.withHub(hubID, "Private")
	h.withHub(otherHubID, "Work")
	h.withCollection()
	return h
}

// sibling stores another collection at one level, so that a test about ranking has something to
// rank against.
func (h *containerHarness) sibling(id shared.ID, parentID shared.ID, name, orderKey string) domain.Container {
	container := domain.Container{
		ID: id, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: parentID,
		Name: name, OrderKey: orderKey, CompletionPolicy: domain.CompletionManual,
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	h.containers.stored[id] = container
	return container
}

var (
	booksID  = shared.MustParseID("0192f000-0000-7000-8000-00000000002b")
	travelID = shared.MustParseID("0192f000-0000-7000-8000-00000000002c")
)

func TestMovingACollectionIntoAnotherHub(t *testing.T) {
	h := moving(t)

	moved, err := MoveContainer{Writer: h.writer}.Execute(t.Context(), actor(), MoveContainerCommand{
		ContainerID: shoppingID, TargetParentID: otherHubID,
	})
	if err != nil {
		t.Fatalf("the move was refused: %v", err)
	}

	if moved.ParentID != otherHubID {
		t.Errorf("parent %s, want the other hub", moved.ParentID)
	}
	if len(h.containers.written) != 1 || h.containers.written[0].method != "placement" {
		t.Fatalf("unexpected writes: %+v", h.containers.written)
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ContainerMoved {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}
	// The hub it came from travels beside the snapshot, so a consumer that cares only about
	// reparenting compares it with parent_id.
	if h.events.appended[0].Payload["from_parent_id"] != hubID.String() {
		t.Errorf("the event does not say where it came from: %+v", h.events.appended[0].Payload)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ContainerMovedAction {
		t.Fatalf("unexpected audit entries: %+v", h.audit.entries)
	}
}

// Both hubs are put to the authorisation service, because taking a collection out of a hub is a
// change to that hub as much as to the destination.
func TestMovingAsksThePermissionOfBothHubs(t *testing.T) {
	h := moving(t)

	if _, err := (MoveContainer{Writer: h.writer}).Execute(t.Context(), actor(), MoveContainerCommand{
		ContainerID: shoppingID, TargetParentID: otherHubID,
	}); err != nil {
		t.Fatalf("the move was refused: %v", err)
	}

	if len(h.authorizer.requests) != 2 {
		t.Fatalf("%d permission questions, want one per hub", len(h.authorizer.requests))
	}
}

// A reorder within one hub is the same operation, and it asks one question rather than two: there
// is no second hub for it to be about.
func TestReorderingWithinTheSameHubAsksOneQuestion(t *testing.T) {
	h := moving(t)
	h.sibling(booksID, hubID, "Books", "a1")
	h.sibling(travelID, hubID, "Travel", "a2")

	moved, err := MoveContainer{Writer: h.writer}.Execute(t.Context(), actor(), MoveContainerCommand{
		ContainerID: shoppingID, TargetParentID: hubID, BeforeContainerID: travelID,
	})
	if err != nil {
		t.Fatalf("the reorder was refused: %v", err)
	}
	if len(h.authorizer.requests) != 1 {
		t.Errorf("%d permission questions, want one", len(h.authorizer.requests))
	}
	if moved.ParentID != hubID {
		t.Errorf("the reorder changed the hub: %s", moved.ParentID)
	}
	// The rank lands between the two neighbours, which is what makes an insertion renumber nothing.
	if moved.OrderKey <= "a1" || moved.OrderKey >= "a2" {
		t.Errorf("rank %q is not between Books and Travel", moved.OrderKey)
	}
}

// A client that asked for a position and got the end of the list has been ignored, so a sibling
// that is not at the destination is its own answer.
func TestASiblingThatIsNotAtTheDestinationIsRefused(t *testing.T) {
	h := moving(t)
	// Books is in the hub the collection is leaving, not in the one it is moving into.
	h.sibling(booksID, hubID, "Books", "a1")

	_, err := MoveContainer{Writer: h.writer}.Execute(t.Context(), actor(), MoveContainerCommand{
		ContainerID: shoppingID, TargetParentID: otherHubID, BeforeContainerID: booksID,
	})
	assertValidation(t, err, "containers.before_container_not_in_level")
}

// The acceptance criterion: moving a collection under a name collision is refused. The unique index
// is what decides it, so this proves the adapter's answer reaches the caller unchanged rather than
// being turned into something else on the way.
func TestANameCollisionAtTheDestinationIsRefused(t *testing.T) {
	h := moving(t)
	h.containers.writeErr = shared.ErrConflict.
		WithDetail("containers.name_taken").
		WithParams(map[string]string{"name": "Shopping"})

	_, err := MoveContainer{Writer: h.writer}.Execute(t.Context(), actor(), MoveContainerCommand{
		ContainerID: shoppingID, TargetParentID: otherHubID,
	})
	assertConflict(t, err, "containers.name_taken")
	if len(h.events.appended) != 0 {
		t.Error("a refused move announced itself anyway")
	}
}

func TestMovingRefusesWhatTheTreeDoesNotAllow(t *testing.T) {
	cases := []struct {
		name       string
		prepare    func(*containerHarness) MoveContainerCommand
		detailCode string
	}{
		{
			name: "a hub has no destination",
			prepare: func(*containerHarness) MoveContainerCommand {
				return MoveContainerCommand{ContainerID: hubID, TargetParentID: otherHubID}
			},
			detailCode: "containers.hub_not_movable",
		},
		{
			name: "into itself",
			prepare: func(*containerHarness) MoveContainerCommand {
				return MoveContainerCommand{ContainerID: shoppingID, TargetParentID: shoppingID}
			},
			detailCode: "containers.parent_is_self",
		},
		{
			name: "into a hub that is not there",
			prepare: func(*containerHarness) MoveContainerCommand {
				return MoveContainerCommand{
					ContainerID:    shoppingID,
					TargetParentID: shared.MustParseID("0192f000-0000-7000-8000-00000000003b"),
				}
			},
			detailCode: "containers.parent_not_found",
		},
		{
			name: "without a destination at all",
			prepare: func(*containerHarness) MoveContainerCommand {
				return MoveContainerCommand{ContainerID: shoppingID}
			},
			detailCode: "containers.target_parent_required",
		},
		{
			name: "into an archived hub",
			prepare: func(h *containerHarness) MoveContainerCommand {
				target := h.containers.stored[otherHubID]
				target.ArchivedAt = &archivedEarly
				h.containers.stored[otherHubID] = target
				return MoveContainerCommand{ContainerID: shoppingID, TargetParentID: otherHubID}
			},
			detailCode: "containers.parent_archived",
		},
		{
			name: "an archived collection stays where it is",
			prepare: func(h *containerHarness) MoveContainerCommand {
				collection := h.containers.stored[shoppingID]
				collection.ArchivedAt = &archivedEarly
				h.containers.stored[shoppingID] = collection
				return MoveContainerCommand{ContainerID: shoppingID, TargetParentID: otherHubID}
			},
			detailCode: "containers.archived",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := moving(t)
			cmd := c.prepare(h)

			_, err := MoveContainer{Writer: h.writer}.Execute(t.Context(), actor(), cmd)
			assertCode(t, err, c.detailCode)
			if len(h.containers.written) != 0 {
				t.Errorf("a refused move wrote a row: %+v", h.containers.written)
			}
		})
	}
}

// A repeat is harmless, and it is the exclusion of the moving container from its own level that
// makes it so: the bounds are the same both times, so the rank the ordering service hands back is
// the rank the collection already has, and nothing is written the second time.
func TestRepeatingAMoveWritesNothingTheSecondTime(t *testing.T) {
	h := moving(t)
	h.sibling(booksID, otherHubID, "Books", "a1")
	handler := MoveContainer{Writer: h.writer}
	cmd := MoveContainerCommand{ContainerID: shoppingID, TargetParentID: otherHubID}

	if _, err := handler.Execute(t.Context(), actor(), cmd); err != nil {
		t.Fatalf("the move was refused: %v", err)
	}
	if len(h.containers.written) != 1 {
		t.Fatalf("the first move wrote %d rows", len(h.containers.written))
	}

	if _, err := handler.Execute(t.Context(), actor(), cmd); err != nil {
		t.Fatalf("the repeat was refused: %v", err)
	}
	if len(h.containers.written) != 1 || len(h.events.appended) != 1 {
		t.Errorf("the repeat wrote again: %d rows, %d events",
			len(h.containers.written), len(h.events.appended))
	}
}
