// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

func TestArchivingWritesTheStampTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()

	updated, err := ArchiveContainer{Writer: h.writer}.Execute(t.Context(), actor(),
		ArchiveContainerCommand{ContainerID: shoppingID})
	if err != nil {
		t.Fatalf("archiving failed: %v", err)
	}

	if updated.ArchivedAt == nil || !updated.ArchivedAt.Equal(now) {
		t.Errorf("archived at %v, want the clock's %v", updated.ArchivedAt, now)
	}
	if len(h.containers.written) != 1 || h.containers.written[0].method != "archived" {
		t.Fatalf("unexpected writes: %+v", h.containers.written)
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ContainerArchived {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ContainerArchivedAction {
		t.Fatalf("unexpected audit entries: %+v", h.audit.entries)
	}
	// A timestamp is not user content, so the trail carries it as it stands: "when was this taken
	// out of use" is the question the entry exists to answer.
	recorded, ok := h.audit.entries[0].Changes["archived_at"].(map[string]any)
	if !ok || recorded["to"] == nil {
		t.Errorf("the entry does not record when: %+v", h.audit.entries[0].Changes)
	}
}

// Idempotent, which is what makes a retry after a lost response harmless.
func TestArchivingAnArchivedContainerWritesNothing(t *testing.T) {
	h := newContainerHarness()
	collection := h.withCollection()
	collection.ArchivedAt = &archivedEarly
	h.containers.stored[shoppingID] = collection

	updated, err := ArchiveContainer{Writer: h.writer}.Execute(t.Context(), actor(),
		ArchiveContainerCommand{ContainerID: shoppingID})
	if err != nil {
		t.Fatalf("the repeat was refused: %v", err)
	}
	if !updated.ArchivedAt.Equal(archivedEarly) || updated.Version != 3 {
		t.Errorf("a repeat moved the stamp or the version: %+v", updated)
	}
	if len(h.containers.written) != 0 || len(h.events.appended) != 0 {
		t.Errorf("a no-op wrote something: %+v", h.containers.written)
	}
}

func TestUnarchivingLiftsTheStamp(t *testing.T) {
	h := newContainerHarness()
	collection := h.withCollection()
	collection.ArchivedAt = &archivedEarly
	h.containers.stored[shoppingID] = collection

	updated, err := UnarchiveContainer{Writer: h.writer}.Execute(t.Context(), actor(),
		ArchiveContainerCommand{ContainerID: shoppingID})
	if err != nil {
		t.Fatalf("unarchiving failed: %v", err)
	}

	if updated.ArchivedAt != nil {
		t.Errorf("still archived: %v", updated.ArchivedAt)
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ContainerUnarchived {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}
	// A cleared field reaches an offline client as null, not as the empty string: the API spells
	// "not set" that way, and a client merging "" into a timestamp would be merging a value this
	// system never renders.
	if len(h.changes.recorded) != 1 {
		t.Fatalf("%d change log entries, want one", len(h.changes.recorded))
	}
	if value, present := h.changes.recorded[0].Payload["archived_at"]; !present || value != nil {
		t.Errorf("the payload says %v, want an explicit null", value)
	}
}

// The acceptance criterion, in the domain's terms: unarchiving restores writability for the
// subtree. Nothing under the hub is written, so a collection archived in its own right stays
// archived - which is what makes the hub's stamp a covering rather than a copy.
func TestArchivingAHubWritesOnlyTheHubsOwnRow(t *testing.T) {
	h := newContainerHarness()
	h.withHub(hubID, "Private")
	h.withCollection()

	if _, err := (ArchiveContainer{Writer: h.writer}).Execute(t.Context(), actor(),
		ArchiveContainerCommand{ContainerID: hubID}); err != nil {
		t.Fatalf("archiving the hub failed: %v", err)
	}

	if len(h.containers.written) != 1 || h.containers.written[0].container.ID != hubID {
		t.Fatalf("archiving a hub wrote more than the hub: %+v", h.containers.written)
	}
	if stored := h.containers.stored[shoppingID]; stored.ArchivedAt != nil {
		t.Errorf("the collection was stamped as well: %v", stored.ArchivedAt)
	}
}

// Inside somebody else's archived subtree, both verbs are writes into an archived subtree - and the
// refusal names the hub, which is the container a client would have to unarchive.
func TestArchivingIsRefusedInsideAnArchivedHub(t *testing.T) {
	h := newContainerHarness()
	collection := h.withCollection()
	collection.ParentArchivedAt = &archivedEarly
	h.containers.stored[shoppingID] = collection

	for name, execute := range map[string]func() (domain.Container, error){
		"archive": func() (domain.Container, error) {
			return ArchiveContainer{Writer: h.writer}.Execute(t.Context(), actor(),
				ArchiveContainerCommand{ContainerID: shoppingID})
		},
		"unarchive": func() (domain.Container, error) {
			return UnarchiveContainer{Writer: h.writer}.Execute(t.Context(), actor(),
				ArchiveContainerCommand{ContainerID: shoppingID})
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := execute()
			assertConflict(t, err, "containers.archived")
		})
	}
}

// An archived hub has to remain unarchivable, which is why the two verbs read the inherited stamp
// rather than the effective one. Reading the effective one would leave an archived hub stuck.
func TestAnArchivedHubCanStillBeUnarchived(t *testing.T) {
	h := newContainerHarness()
	hub := h.withHub(hubID, "Private")
	hub.ArchivedAt = &archivedEarly
	h.containers.stored[hubID] = hub

	updated, err := UnarchiveContainer{Writer: h.writer}.Execute(t.Context(), actor(),
		ArchiveContainerCommand{ContainerID: hubID})
	if err != nil {
		t.Fatalf("an archived hub refused to be unarchived: %v", err)
	}
	if updated.ArchivedAt != nil {
		t.Errorf("still archived: %v", updated.ArchivedAt)
	}
}

func TestArchivingHonoursTheExpectedVersion(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()

	_, err := ArchiveContainer{Writer: h.writer}.Execute(t.Context(), actor(),
		ArchiveContainerCommand{ContainerID: shoppingID, ExpectedVersion: 2})
	if err != nil {
		t.Fatalf("archiving failed: %v", err)
	}
	// The version the caller named is what the write matches on, so a row somebody else moved on
	// matches nothing and the adapter reports the conflict.
	if h.containers.written[0].expected != 2 {
		t.Errorf("written against version %d, want the one the caller named",
			h.containers.written[0].expected)
	}
}
