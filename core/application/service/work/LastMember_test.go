// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// orphanStore answers the orphan questions from two lists: what is orphaned before the lock, and
// what still is after it - a grant committing in between is the second list's difference.
type orphanStore struct {
	before, after []shared.ID
	locked        []shared.ID
	asked         [][]shared.ID
	owners        []shared.ID
	held          []shared.ID
}

func (s *orphanStore) Orphaned(_ context.Context, named []shared.ID) ([]shared.ID, error) {
	s.asked = append(s.asked, named)
	if s.locked == nil {
		return s.before, nil
	}
	return s.after, nil
}

func (s *orphanStore) Lock(_ context.Context, ids []shared.ID) error {
	s.locked = append([]shared.ID{}, ids...)
	return nil
}

func (s *orphanStore) HeldBy(context.Context, shared.ID) ([]shared.ID, error) { return s.held, nil }

func (s *orphanStore) WorkspaceOwners(context.Context) ([]shared.ID, error) { return s.owners, nil }

type ownersTold struct{ calls [][]shared.ID }

func (o *ownersTold) PrivateHubTrashed(_ context.Context, _ shared.ID, recipients []shared.ID) error {
	o.calls = append(o.calls, recipients)
	return nil
}

var workspaceOwner = shared.ID("018f0000-0000-7000-8000-0000000000f1")

// UC-ID-16 check 6: the hub goes to the trash as the system, with the reason, and the workspace's
// owners are told - nobody is asked for a permission, because nobody asked for this.
func TestAPrivateHubWithoutAMemberGoesToTheTrash(t *testing.T) {
	h := newContainerHarness()
	h.withHub(hubID, "Diary")
	h.withCollection()
	store := &orphanStore{before: []shared.ID{hubID}, after: []shared.ID{hubID}, owners: []shared.ID{workspaceOwner}}
	told := &ownersTold{}

	trashed, err := LastMember{Hubs: store, Writer: h.writer, Owners: told}.
		AfterMemberLeft(t.Context(), tenantID, []shared.ID{shoppingID})
	if err != nil {
		t.Fatalf("the check failed: %v", err)
	}
	if trashed != 1 {
		t.Errorf("%d hubs trashed, want 1", trashed)
	}
	if !slices.Equal(store.locked, []shared.ID{hubID}) {
		t.Errorf("locked %v, want the hub", store.locked)
	}
	if len(store.asked) != 2 || !slices.Equal(store.asked[0], []shared.ID{shoppingID}) {
		t.Errorf("asked %v, want the named collection and then the locked hub", store.asked)
	}
	if !h.containers.stored[hubID].IsTrashed() || !h.containers.stored[shoppingID].IsTrashed() {
		t.Error("the hub and its collection are not in the trash")
	}
	if by := h.containers.stored[hubID].DeletedBy; by.Kind != appshared.ActorSystem {
		t.Errorf("deleted by %q, want the system", by.Kind)
	}
	if len(h.authorizer.requests) != 0 {
		t.Errorf("%d permission questions, want none", len(h.authorizer.requests))
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ContainerDeleted {
		t.Errorf("the events are %v, want one %s", h.events.appended, event.ContainerDeleted)
	}
	if len(h.audit.entries) != 1 {
		t.Fatalf("%d audit entries, want 1", len(h.audit.entries))
	}
	entry := h.audit.entries[0]
	if entry.Action != ContainerDeletedAction || entry.ActorKind != appshared.ActorSystem {
		t.Errorf("the entry is %s by %s, want %s by the system", entry.Action, entry.ActorKind, ContainerDeletedAction)
	}
	if reason := changeTo(entry.Changes, "reason"); reason != ReasonPrivateHubOrphaned {
		t.Errorf("the reason is %q, want %q", reason, ReasonPrivateHubOrphaned)
	}
	if len(told.calls) != 1 || !slices.Equal(told.calls[0], []shared.ID{workspaceOwner}) {
		t.Errorf("told %v, want the workspace's owner once", told.calls)
	}
}

// A person given a role while the last one left is seen under the lock, and the hub stays.
func TestAMemberAddedMeanwhileKeepsTheHub(t *testing.T) {
	h := newContainerHarness()
	h.withHub(hubID, "Diary")
	store := &orphanStore{before: []shared.ID{hubID}, owners: []shared.ID{workspaceOwner}}
	told := &ownersTold{}

	trashed, err := LastMember{Hubs: store, Writer: h.writer, Owners: told}.
		AfterMemberLeft(t.Context(), tenantID, []shared.ID{hubID})
	if err != nil {
		t.Fatalf("the check failed: %v", err)
	}
	if trashed != 0 || h.containers.stored[hubID].IsTrashed() || len(told.calls) != 0 {
		t.Errorf("trashed %d, hub in the trash %v, told %v; want nothing", trashed,
			h.containers.stored[hubID].IsTrashed(), told.calls)
	}
}

// Nothing named asks nothing - an empty list would read as "every hub" in storage - and the sweep
// asks about every hub on purpose.
func TestOnlyTheSweepAsksAboutEveryHub(t *testing.T) {
	h := newContainerHarness()
	store := &orphanStore{}
	last := LastMember{Hubs: store, Writer: h.writer}

	if _, err := last.AfterMemberLeft(t.Context(), tenantID, nil); err != nil {
		t.Fatal(err)
	}
	if len(store.asked) != 0 {
		t.Errorf("an empty name list asked %v", store.asked)
	}
	if _, err := last.Sweep(t.Context(), tenantID); err != nil {
		t.Fatal(err)
	}
	if len(store.asked) != 1 || store.asked[0] != nil {
		t.Errorf("the sweep asked %v, want every hub (nil)", store.asked)
	}

	store.held = []shared.ID{hubID}
	held, err := last.HeldBy(t.Context(), accountID)
	if err != nil || !slices.Equal(held, []shared.ID{hubID}) {
		t.Errorf("held %v, %v", held, err)
	}
}

func changeTo(changes map[string]any, field string) string {
	change, ok := changes[field].(map[string]any)
	if !ok {
		return ""
	}
	to, _ := change["to"].(string)
	return to
}
