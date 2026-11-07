// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	PreviewContainerStructureName = "PreviewContainerStructure"

	// previewEntryCeiling bounds the entries one preview reads. A hub past it is counted over its
	// first entries by collection and depth: the preview is what a person confirms, and a count
	// that stops at a large number says what it has to.
	previewEntryCeiling = 50000
)

// ContainerStructurePreviewedAction is the audit code of a refused preview.
const ContainerStructurePreviewedAction audit.Action = "container.structure_previewed"

// PreviewContainerStructure answers what a change of a hub's or a collection's own structure would
// stop showing, before it is made (UC-WRK-24 checks 2 and 7). It decides the change exactly as
// SetContainerStructure does, and so refuses it as that would, then writes nothing.
type PreviewContainerStructure struct {
	Set    SetContainerStructure
	Shapes repository.EntryShapes
}

// StructurePreview is the answer: what would no longer be shown, and for a hub the collections
// that keep their own value of a setting the change moves.
type StructurePreview struct {
	Narrowing domain.Narrowing
	Keeping   []KeptOwnValue
	// Flatten is how many entries narrowing FLATTEN would move to the top level.
	Flatten int
}

// KeptOwnValue is a collection that follows a hub and keeps its own value of these settings.
type KeptOwnValue struct {
	Container domain.Container
	Settings  []domain.StructureSetting
}

// Execute decides the change and counts what it would hide, in a read-only transaction.
func (h PreviewContainerStructure) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd SetContainerStructureCommand,
) (StructurePreview, error) {
	w := h.Set.Writer
	if cmd.ContainerID.IsZero() {
		return StructurePreview{}, containerIDRequired()
	}
	current, err := w.read(ctx, actor, cmd.ContainerID)
	if err != nil {
		return StructurePreview{}, err
	}
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       containerPath(current),
		Action:     ContainerStructurePreviewedAction,
		TokenScope: containersWrite,
		TargetType: containerTarget,
		TargetID:   cmd.ContainerID,
	}); err != nil {
		return StructurePreview{}, err
	}
	if !h.Set.Structure.Wired() || h.Shapes == nil {
		return StructurePreview{}, shared.ErrInternal.WithDetail("containers.structure_unwired")
	}

	var preview StructurePreview
	err = w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		counted, err := h.count(ctx, actor, cmd)
		preview = counted
		return err
	})
	return preview, err
}

func (h PreviewContainerStructure) count(ctx context.Context, actor appshared.ActorContext, cmd SetContainerStructureCommand) (StructurePreview, error) {
	reading := h.Set.Structure.within(repository.LayerRead)
	container, err := findContainer(ctx, h.Set.Writer.Containers, cmd.ContainerID)
	if err != nil {
		return StructurePreview{}, err
	}
	if err := container.EnsureEditable(); err != nil {
		return StructurePreview{}, err
	}
	layer, _, err := h.Set.decide(ctx, reading, actor, container, cmd)
	if err != nil {
		return StructurePreview{}, err
	}
	bound, err := reading.Bound(ctx)
	if err != nil {
		return StructurePreview{}, err
	}

	// The collections whose structure in force the change moves, before and after it.
	type change struct{ before, after domain.EffectiveStructure }
	changes := map[shared.ID]change{}
	preview := StructurePreview{Keeping: []KeptOwnValue{}}
	collections := []shared.ID{}
	if container.Type == domain.ContainerCollection {
		before, err := reading.Of(ctx, container)
		if err != nil {
			return StructurePreview{}, err
		}
		after, err := reading.with(ctx, container, layer)
		if err != nil {
			return StructurePreview{}, err
		}
		changes[container.ID] = change{before, after}
		collections = append(collections, container.ID)
	} else {
		heirs, err := h.Set.Structure.Structures.Heirs(ctx, container.ID, structureFanOutCeiling)
		if err != nil {
			return StructurePreview{}, err
		}
		befores := make(map[shared.ID]domain.EffectiveStructure, len(heirs))
		for _, heir := range heirs {
			if befores[heir.ID], err = reading.Of(ctx, heir); err != nil {
				return StructurePreview{}, err
			}
		}
		moved := container.Structure.Values.Moved(layer.Values)
		changed := container
		changed.Structure = layer
		reading.hubs[container.ID] = changed
		for _, heir := range heirs {
			after, err := reading.Of(ctx, heir)
			if err != nil {
				return StructurePreview{}, err
			}
			changes[heir.ID] = change{befores[heir.ID], after}
			collections = append(collections, heir.ID)
			kept := []domain.StructureSetting{}
			for _, setting := range moved {
				if heir.Structure.Values.Has(setting) {
					kept = append(kept, setting)
				}
			}
			if len(kept) > 0 {
				preview.Keeping = append(preview.Keeping, KeptOwnValue{Container: heir, Settings: kept})
			}
		}
	}

	shapes, err := h.Shapes.ShapesIn(ctx, collections, previewEntryCeiling)
	if err != nil {
		return StructurePreview{}, err
	}
	byCollection := entryShapesByCollection(shapes)
	afters := make(map[shared.ID]domain.EffectiveStructure, len(changes))
	for id, moved := range changes {
		afters[id] = moved.after
	}
	preview.Flatten = flattenCount(bound, afters, shapes)
	for _, id := range collections {
		moved := changes[id]
		preview.Narrowing = preview.Narrowing.Add(
			domain.NarrowingOf(bound, moved.before, moved.after, byCollection[id]))
	}
	return preview, nil
}

// entryShapesByCollection puts each entry's ancestor types beside it, read off its path among the
// entries of its collection: the live entries above a live entry are all of them there.
func entryShapesByCollection(shapes []repository.EntryShape) map[shared.ID][]domain.EntryShape {
	types := make(map[shared.ID]domain.ItemType, len(shapes))
	for _, shape := range shapes {
		types[shape.ID] = shape.Type
	}
	grouped := map[shared.ID][]domain.EntryShape{}
	for _, shape := range shapes {
		ids := domain.PathIDs(shape.Path)
		if len(ids) > 0 {
			ids = ids[:len(ids)-1]
		}
		slices.Reverse(ids)
		ancestors := make([]domain.ItemType, 0, len(ids))
		for _, id := range ids {
			if itemType, held := types[id]; held {
				ancestors = append(ancestors, itemType)
			}
		}
		grouped[shape.CollectionID] = append(grouped[shape.CollectionID], domain.EntryShape{
			Type: shape.Type, Ancestors: ancestors, Holds: shape.Holds,
		})
	}
	return grouped
}

// Descriptor is the catalogue entry.
func (h PreviewContainerStructure) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       PreviewContainerStructureName,
		Permission: service.PermissionStructure,
		Summary: "Answers what a change of a hub's or a collection's own structure would stop " +
			"showing, before it is made: the levels whose entries would be hidden with how many, " +
			"the fields whose values would be hidden with on how many entries, the views no longer " +
			"offered, what would no longer run by itself, and for a hub the collections that keep " +
			"their own value. Takes what SetContainerStructure takes and refuses what it would " +
			"refuse. Writes nothing; nothing a change hides is deleted.",
		SideEffects: "None.",
		ReadOnly:    true,
		TokenScope:  containersWrite,
		Input:       h.Set.Descriptor().Input,
		Audit: usecase.AuditDeclaration{
			Action: ContainerStructurePreviewedAction, TargetType: containerTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a preview writes nothing.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h PreviewContainerStructure) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return nil, err
	}
	cmd := SetContainerStructureCommand{ContainerID: containerID, Preset: domain.PresetKey(in.String("preset"))}
	if raw, held := in["settings"].(map[string]any); held {
		if cmd.Settings, err = structureValuesFrom(raw); err != nil {
			return nil, err
		}
	}
	if cmd.Reset, err = settingsResetFrom(in); err != nil {
		return nil, err
	}
	preview, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return structurePreviewOutput(preview), nil
}

// structurePreviewOutput is the preview in the contract's words.
func structurePreviewOutput(preview StructurePreview) usecase.Output {
	n := preview.Narrowing
	levels := make([]any, 0, len(n.Levels))
	for _, level := range n.Levels {
		levels = append(levels, map[string]any{"type": string(level.Type), "entries": level.Entries})
	}
	fields := make([]any, 0, len(n.Fields))
	for _, field := range n.Fields {
		fields = append(fields, map[string]any{
			"type": string(field.Type), "capability": string(field.Capability), "entries": field.Entries,
		})
	}
	views := make([]any, 0, len(n.Views))
	for _, view := range n.Views {
		views = append(views, string(view))
	}
	automation := make([]any, 0, len(n.Automation))
	for _, behaviour := range n.Automation {
		automation = append(automation, string(behaviour))
	}
	keeping := make([]any, 0, len(preview.Keeping))
	for _, kept := range preview.Keeping {
		settings := make([]any, 0, len(kept.Settings))
		for _, setting := range kept.Settings {
			settings = append(settings, string(setting))
		}
		keeping = append(keeping, map[string]any{
			"container_id": kept.Container.ID.String(), "name": kept.Container.Name, "settings": settings,
		})
	}
	return usecase.Output{
		"narrows": n.Narrows(), "levels": levels, "fields": fields, "views": views,
		"automation": automation, "keeping": keeping,
		"flatten": map[string]any{"entries": preview.Flatten},
	}
}
