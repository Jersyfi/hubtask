// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"errors"
	"slices"
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func (a *assured) RemoveGroup(_ context.Context, id shared.ID, expected int) error {
	i := slices.IndexFunc(a.groups, func(g domain.Group) bool { return g.ID == id })
	switch {
	case i < 0:
		return shared.ErrNotFound.WithDetail("cmdb.group_not_found")
	case a.groups[i].Version != expected:
		return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
	}
	a.groups = slices.Delete(a.groups, i, i+1)
	return nil
}

// UC-AST-11: a writer deletes a saved group by its key at the version read; the objects it held
// stay, the change log tells devices to drop it, and the trail names its key only. A group
// changed meanwhile is a version conflict, a key gone is not found.
func TestASavedGroupIsDeletedByItsKey(t *testing.T) {
	f, a := assuredFixture()
	log := &changeLog{}
	f.inv.Changes, f.inv.HLC = log, &hlcs{}
	if _, err := call(t, DefineCiGroup{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{
		"hub_id": hub.String(), "key": "servers", "name": "Servers", "filter": map[string]any{"classes": []any{"server"}},
	}); err != nil {
		t.Fatal(err)
	}
	g := a.groups[0]
	remove := DeleteCiGroup{Inventory: f.inv}.Descriptor()
	if _, err := call(t, remove, as(anna), usecase.Input{"hub_id": hub.String(), "key": "servers", "expected_version": g.Version + 1}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale version deleted: %v", err)
	}
	if _, err := call(t, remove, as(anna), usecase.Input{"hub_id": hub.String(), "key": "servers", "expected_version": g.Version}); err != nil {
		t.Fatal(err)
	}
	if len(a.groups) != 0 || len(f.m.cis) != 5 {
		t.Errorf("groups %d, objects %d", len(a.groups), len(f.m.cis))
	}
	changes := log.of(GroupEntity, g.ID)
	if last := changes[len(changes)-1]; last.Op != changelog.Delete || last.ContainerID != hub || last.Payload != nil {
		t.Errorf("the change log: %+v", last)
	}
	entries := f.inv.Audit.(*trail).entries
	if last := entries[len(entries)-1]; last.Action != GroupDeletedAction || last.TargetID != g.ID || len(last.Changes) != 1 {
		t.Errorf("trail: %s %s %v", last.Action, last.TargetID, last.Changes)
	}
	if _, err := call(t, remove, as(anna), usecase.Input{"hub_id": hub.String(), "key": "servers"}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a gone group: %v", err)
	}
	if _, err := call(t, remove, as(bert), usecase.Input{"hub_id": private.String(), "key": "servers"}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a hub out of reach: %v", err)
	}
}
