// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The summer party's hub follows the workspace; Hardware keeps its own views.
func newWorkspacePreviewHarness(t *testing.T) (*structureHarness, *usecase.Registry) {
	t.Helper()
	h, _ := newPreviewHarness(t)
	hub := h.containers.stored[hubID]
	hub.Structure = domain.StructureLayer{}
	h.containers.stored[hubID] = hub
	project, _ := domain.FindPreset(domain.PresetProject, domain.NewStructureBound(seededProfiles(), seededProfiles()))
	h.store.workspace.Layer = domain.StructureLayer{Preset: domain.PresetProject, Values: project.Values}

	preview := PreviewWorkspaceStructure{
		Set: SetWorkspaceStructure{
			Structure: h.set.Structure, Authorizer: h.authorizer, Changes: h.changes, Epochs: h.epochs,
			Audit: h.audit, UnitOfWork: h.uow, Clock: clock.Fixed(now), HLC: &hlcSource{},
		},
		Shapes: summerPartyShapes(),
	}
	registry, err := usecase.NewRegistry(nil, preview.Descriptor())
	if err != nil {
		t.Fatalf("the catalogue refused the entry: %v", err)
	}
	return h, registry
}

// The workspace's own structure switched to a list: every collection that follows it is counted -
// the levels and values that would no longer be shown, over the whole workspace - Hardware is
// named as keeping its own views, and nothing is written.
func TestAPreviewOfTheWorkspaceCountsEveryCollectionThatFollowsIt(t *testing.T) {
	h, registry := newWorkspacePreviewHarness(t)

	out, err := registry.Invoke(context.Background(), PreviewWorkspaceStructureName, actor(),
		usecase.Input{"preset": "LIST"})
	if err != nil {
		t.Fatalf("the preview failed: %v", err)
	}
	if out["narrows"] != true {
		t.Errorf("narrows = %v", out["narrows"])
	}
	wantLevels := []any{
		map[string]any{"type": "WORK_PACKAGE", "entries": 2},
		map[string]any{"type": "ACTIVITY", "entries": 2},
	}
	if !reflect.DeepEqual(out["levels"], wantLevels) {
		t.Errorf("levels = %v", out["levels"])
	}
	wantKeeping := []any{map[string]any{
		"container_id": otherCollectionID.String(), "name": "Hardware", "settings": []any{"views"},
	}}
	if !reflect.DeepEqual(out["keeping"], wantKeeping) {
		t.Errorf("keeping = %v", out["keeping"])
	}
	if h.store.workspace.Layer.Preset != domain.PresetProject || len(h.store.written) != 0 {
		t.Error("the preview changed the workspace")
	}
	if len(h.audit.entries) != 0 || len(h.changes.recorded) != 0 {
		t.Errorf("the preview wrote: audit %d, changes %d", len(h.audit.entries), len(h.changes.recorded))
	}
}

// A preview is refused as the change would be, under its own audit code.
func TestAPreviewOfTheWorkspaceIsRefusedAsTheChangeWouldBe(t *testing.T) {
	h, registry := newWorkspacePreviewHarness(t)
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.denied")

	_, err := registry.Invoke(context.Background(), PreviewWorkspaceStructureName, actor(),
		usecase.Input{"preset": "LIST"})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("err = %v", err)
	}
	if h.authorizer.requests[len(h.authorizer.requests)-1].Action != WorkspaceStructurePreviewedAction {
		t.Errorf("asked %+v", h.authorizer.requests)
	}
}
