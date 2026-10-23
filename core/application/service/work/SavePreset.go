// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
)

const (
	SavePresetName     = "SavePreset"
	ChangePresetName   = "ChangePreset"
	WithdrawPresetName = "WithdrawPreset"
)

// PresetDesk is what the saved presets' writers share (UC-WRK-30 checks 1, 3): the presets, the
// structure a preset is copied from, the rights.
type PresetDesk struct {
	Presets    repository.Presets
	Structure  StructureResolver
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Text       text.Normalizer
}

// readableContainer reads a container the actor may read: its structure in force is what a
// preset is copied from. A container the actor may not read is not found, like one that never was.
func (d PresetDesk) readableContainer(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, action audit.Action,
) (domain.Container, error) {
	var container domain.Container
	err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := findContainer(ctx, d.Structure.Containers, id)
		container = found
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	if err := d.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(container),
		Action:     action,
		TokenScope: containersRead,
		TargetType: containerTarget,
		TargetID:   container.ID,
	}); err != nil {
		return domain.Container{}, err
	}
	return container, nil
}

// inForce answers the container's eight settings in force, read inside the caller's transaction.
func (d PresetDesk) inForce(ctx context.Context, container domain.Container) (domain.StructureValues, error) {
	effective, err := d.Structure.within(repository.LayerRead).Of(ctx, container)
	if err != nil {
		return domain.StructureValues{}, err
	}
	return effective.Values, nil
}

// SavePreset saves a hub's or collection's structure in force as a preset of the person's own or
// of the workspace (UC-WRK-30 check 1).
type SavePreset struct{ Desk PresetDesk }

// SavePresetCommand is the input, typed.
type SavePresetCommand struct {
	Name        string
	Origin      domain.PresetOrigin
	ContainerID shared.ID
}

// Execute saves the preset.
func (h SavePreset) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd SavePresetCommand,
) (domain.Preset, error) {
	d := h.Desk
	if !d.Structure.Wired() {
		return domain.Preset{}, shared.ErrInternal.WithDetail("containers.structure_unwired")
	}
	id := d.IDs.NewID()
	if cmd.Origin == domain.PresetOriginWorkspace || cmd.Origin == domain.PresetOriginOwn {
		if err := authorizePreset(ctx, d.Authorizer, actor, cmd.Origin, PresetSavedAction, id); err != nil {
			return domain.Preset{}, err
		}
	}
	container, err := d.readableContainer(ctx, actor, cmd.ContainerID, PresetSavedAction)
	if err != nil {
		return domain.Preset{}, err
	}

	var saved domain.Preset
	err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		values, err := d.inForce(ctx, container)
		if err != nil {
			return err
		}
		preset, err := domain.NewSavedPreset(domain.NewSavedPresetInput{
			ID: id, Origin: cmd.Origin, OwnerID: actor.AccountID, Name: cmd.Name, Values: values, Text: d.Text,
		})
		if err != nil {
			return err
		}
		count, err := d.Presets.Count(ctx, preset.Origin, preset.OwnerID)
		if err != nil {
			return err
		}
		if count >= maxPresetsOf(preset.Origin) {
			return domain.ErrPresetsLimitReached(preset.Origin)
		}
		now := d.Clock.Now()
		if err := d.Presets.Insert(ctx, preset, now); err != nil {
			return err
		}
		saved = preset
		return d.Audit.Append(ctx, presetAudit(ctx, actor, PresetSavedAction, preset, now))
	})
	if err != nil {
		return domain.Preset{}, err
	}
	return saved, nil
}

func maxPresetsOf(origin domain.PresetOrigin) int {
	if origin == domain.PresetOriginOwn {
		return domain.MaxOwnPresets
	}
	return domain.MaxWorkspacePresets
}

// Descriptor is the catalogue entry.
func (h SavePreset) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SavePresetName,
		Summary: "Saves the eight settings in force on a hub or collection as a preset under a name, " +
			"its categories left out: OWN for the caller alone, WORKSPACE for everyone in the " +
			"workspace, which needs the right to change the workspace's shape. Reading the " +
			"container needs READ on it. Refused as presets.limit_reached beyond the bound of its origin.",
		SideEffects: "Stores the preset and writes an audit entry without its name.",
		TokenScope:  containersWrite,
		Input: []usecase.Field{
			{Name: "name", Kind: usecase.KindString, Required: true,
				Description: "The preset's name: one line, at most 100 characters."},
			{Name: "origin", Kind: usecase.KindString, Required: true,
				Enum:        []string{string(domain.PresetOriginWorkspace), string(domain.PresetOriginOwn)},
				Description: "For the workspace, or for the caller alone."},
			{Name: "container_id", Kind: usecase.KindID, Required: true,
				Description: "The hub or collection whose structure in force is saved."},
		},
		Audit: usecase.AuditDeclaration{
			Action: PresetSavedAction, TargetType: presetTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: presetActivityExempt,
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// presetActivityExempt is why a preset's writes have no item history.
var presetActivityExempt = usecase.ActivityDeclaration{
	Exempt: "a preset is a starting point copied into a hub or collection, not an item, and the " +
		"history is an item's: `ActivityEntry` is keyed on `itemId`. A write to one is in the audit trail.",
}

func (h SavePreset) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return nil, err
	}
	preset, err := h.Execute(ctx, actor, SavePresetCommand{
		Name: in.String("name"), Origin: domain.PresetOrigin(in.String("origin")), ContainerID: containerID,
	})
	if err != nil {
		return nil, err
	}
	return presetOutput(preset), nil
}
