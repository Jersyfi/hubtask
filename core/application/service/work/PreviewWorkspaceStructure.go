// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const PreviewWorkspaceStructureName = "PreviewWorkspaceStructure"

// WorkspaceStructurePreviewedAction is the audit code of a refused preview.
const WorkspaceStructurePreviewedAction audit.Action = "workspace.structure_previewed"

// PreviewWorkspaceStructure answers what a change of the workspace's own structure would stop
// showing in every hub and collection that follows it, before it is made: the hub preview's
// answer, spanning the workspace. It decides the change as SetWorkspaceStructure does, so it is
// refused as that would be, and writes nothing.
type PreviewWorkspaceStructure struct {
	Set    SetWorkspaceStructure
	Shapes repository.EntryShapes
}

// Execute decides the change and counts what it would hide, in a read-only transaction.
func (h PreviewWorkspaceStructure) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd SetWorkspaceStructureCommand,
) (StructurePreview, error) {
	if err := h.Set.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     WorkspaceStructurePreviewedAction,
		TokenScope: workspaceManageScope,
		TargetType: workspaceStructureTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return StructurePreview{}, err
	}
	if !h.Set.Structure.Wired() || h.Shapes == nil {
		return StructurePreview{}, shared.ErrInternal.WithDetail("containers.structure_unwired")
	}
	var preview StructurePreview
	err := h.Set.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		counted, err := h.count(ctx, actor, cmd)
		preview = counted
		return err
	})
	return preview, err
}

func (h PreviewWorkspaceStructure) count(
	ctx context.Context, actor appshared.ActorContext, cmd SetWorkspaceStructureCommand,
) (StructurePreview, error) {
	reading := h.Set.Structure.within(repository.LayerRead)
	current, _, err := reading.Workspace(ctx)
	if err != nil {
		return StructurePreview{}, err
	}
	bound, err := reading.Bound(ctx)
	if err != nil {
		return StructurePreview{}, err
	}
	next, err := h.Set.decide(ctx, reading, bound, current, actor, cmd)
	if err != nil {
		return StructurePreview{}, err
	}

	// Past the fan-out ceiling the counts are over the first containers, as a hub's are over its
	// first entries: a person confirms a count, and one that stops at a large number says so.
	heirs, err := h.Set.Structure.Structures.Heirs(ctx, shared.ID(""), structureFanOutCeiling)
	if err != nil {
		return StructurePreview{}, err
	}
	collections := []shared.ID{}
	befores := map[shared.ID]domain.EffectiveStructure{}
	for _, heir := range heirs {
		if heir.Type != domain.ContainerCollection {
			continue
		}
		if befores[heir.ID], err = reading.Of(ctx, heir); err != nil {
			return StructurePreview{}, err
		}
		collections = append(collections, heir.ID)
	}
	moved := current.Layer.Values.Moved(next.Layer.Values)
	reading.workspace = &next
	preview := StructurePreview{Keeping: []KeptOwnValue{}}
	afters := make(map[shared.ID]domain.EffectiveStructure, len(collections))
	for _, heir := range heirs {
		kept := []domain.StructureSetting{}
		for _, setting := range moved {
			if heir.Structure.Values.Has(setting) {
				kept = append(kept, setting)
			}
		}
		if len(kept) > 0 {
			preview.Keeping = append(preview.Keeping, KeptOwnValue{Container: heir, Settings: kept})
		}
		if heir.Type != domain.ContainerCollection {
			continue
		}
		if afters[heir.ID], err = reading.Of(ctx, heir); err != nil {
			return StructurePreview{}, err
		}
	}

	shapes, err := h.Shapes.ShapesIn(ctx, collections, previewEntryCeiling)
	if err != nil {
		return StructurePreview{}, err
	}
	byCollection := entryShapesByCollection(shapes)
	preview.Flatten = flattenCount(bound, afters, shapes)
	for _, id := range collections {
		preview.Narrowing = preview.Narrowing.Add(
			domain.NarrowingOf(bound, befores[id], afters[id], byCollection[id]))
	}
	return preview, nil
}

// Descriptor is the catalogue entry.
func (h PreviewWorkspaceStructure) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       PreviewWorkspaceStructureName,
		Permission: service.PermissionStructure,
		Summary: "Answers what a change of the workspace's own structure would stop showing in " +
			"every hub and collection that follows it, before it is made: the levels whose entries " +
			"would be hidden with how many, the fields whose values would be hidden with on how " +
			"many entries, the views no longer offered, what would no longer run by itself, and " +
			"the hubs and collections that keep their own value. Takes what SetWorkspaceStructure " +
			"takes and refuses what it would refuse. Writes nothing; nothing a change hides is deleted.",
		SideEffects: "None.",
		ReadOnly:    true,
		TokenScope:  workspaceManageScope,
		Input:       h.Set.Descriptor().Input,
		Audit: usecase.AuditDeclaration{
			Action: WorkspaceStructurePreviewedAction, TargetType: workspaceStructureTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a preview writes nothing.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h PreviewWorkspaceStructure) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := workspaceStructureCommandFrom(in)
	if err != nil {
		return nil, err
	}
	preview, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return structurePreviewOutput(preview), nil
}
