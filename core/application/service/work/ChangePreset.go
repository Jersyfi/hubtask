// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"strconv"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// ChangePreset renames a saved preset, copies a structure into it again or sets settings on top
// (UC-WRK-30 check 3). Hubs and collections made from it keep their settings: a preset is copied,
// never followed.
type ChangePreset struct{ Desk PresetDesk }

// ChangePresetCommand is the input, typed. An empty field is left as it is.
type ChangePresetCommand struct {
	PresetID        shared.ID
	Name            string
	ContainerID     shared.ID
	Settings        domain.StructureValues
	ExpectedVersion int
}

// Execute changes the preset.
func (h ChangePreset) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ChangePresetCommand,
) (domain.Preset, error) {
	d := h.Desk
	current, err := d.writable(ctx, actor, cmd.PresetID, PresetChangedAction)
	if err != nil {
		return domain.Preset{}, err
	}
	var container domain.Container
	if !cmd.ContainerID.IsZero() {
		if container, err = d.readableContainer(ctx, actor, cmd.ContainerID, PresetChangedAction); err != nil {
			return domain.Preset{}, err
		}
	}

	var changed domain.Preset
	err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		next, moved, err := h.decide(ctx, current, container, cmd)
		if err != nil {
			return err
		}
		changed = next
		if len(moved) == 0 {
			return nil
		}
		expected := current.Version
		if cmd.ExpectedVersion != 0 {
			expected = cmd.ExpectedVersion
		}
		now := d.Clock.Now()
		if err := d.Presets.Update(ctx, next, expected, now); err != nil {
			return err
		}
		changed.Version = expected + 1
		return d.Audit.Append(ctx, presetAudit(ctx, actor, PresetChangedAction, next, now, moved...))
	})
	if err != nil {
		return domain.Preset{}, err
	}
	return changed, nil
}

// decide is the preset after the change, and what moved: the name (never its text, rule 10) and the
// settings by name.
func (h ChangePreset) decide(
	ctx context.Context, current domain.Preset, container domain.Container, cmd ChangePresetCommand,
) (domain.Preset, []audit.Change, error) {
	d := h.Desk
	if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
		return domain.Preset{}, nil, shared.ErrVersionConflict.WithDetail("presets.version_conflict").
			WithParams(map[string]string{"current_version": strconv.Itoa(current.Version)})
	}
	next := current
	var moved []audit.Change
	if cmd.Name != "" {
		name, err := domain.PresetName(cmd.Name, d.Text)
		if err != nil {
			return domain.Preset{}, nil, err
		}
		if name != current.Name {
			next.Name = name
			moved = append(moved, audit.Change{Field: "name", Classification: audit.Sensitive})
		}
	}
	if !container.ID.IsZero() {
		values, err := d.inForce(ctx, container)
		if err != nil {
			return domain.Preset{}, nil, err
		}
		next.Values = domain.WithoutCategories(values)
	}
	if len(cmd.Settings.Decided()) > 0 {
		bound, err := d.Structure.within(repository.LayerRead).Bound(ctx)
		if err != nil {
			return domain.Preset{}, nil, err
		}
		validated, err := bound.Validate(cmd.Settings, d.Text)
		if err != nil {
			return domain.Preset{}, nil, err
		}
		if err := refuseWorkspaceCategories(validated); err != nil {
			return domain.Preset{}, nil, err
		}
		next.Values = next.Values.Overlay(validated)
	}
	for _, setting := range current.Values.Moved(next.Values) {
		moved = append(moved, audit.Change{Field: "structure." + string(setting), Classification: audit.Open})
	}
	return next, moved, nil
}

// writable reads a saved preset the actor may change: their own, or the workspace's with the
// right to change its shape. Another person's own preset is not found (T-04).
func (d PresetDesk) writable(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, action audit.Action,
) (domain.Preset, error) {
	var preset domain.Preset
	err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := d.Presets.Find(ctx, id)
		preset = found
		return err
	})
	if err != nil {
		return domain.Preset{}, err
	}
	if !mayChoose(actor, preset) {
		return domain.Preset{}, shared.ErrNotFound.WithDetail("presets.not_found").
			WithParams(map[string]string{"preset_id": id.String()})
	}
	if err := authorizePreset(ctx, d.Authorizer, actor, preset.Origin, action, preset.ID); err != nil {
		return domain.Preset{}, err
	}
	return preset, nil
}

// Descriptor is the catalogue entry.
func (h ChangePreset) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ChangePresetName,
		Summary: "Changes a saved preset: renames it, copies a hub's or collection's structure in " +
			"force into it again, or sets settings on top, held to what the installation offers. " +
			"Hubs and collections made from it keep their settings. An own preset is changed by " +
			"its owner alone; a workspace preset needs the right to change the workspace's shape.",
		SideEffects: "Writes the preset and an audit entry naming what moved, without the name.",
		TokenScope:  containersWrite,
		Input: []usecase.Field{
			{Name: "preset_id", Kind: usecase.KindID, Required: true, Description: "The saved preset."},
			{Name: "name", Kind: usecase.KindString, Description: "The new name. Omitted leaves it."},
			{Name: "container_id", Kind: usecase.KindID,
				Description: "A hub or collection whose structure in force replaces the preset's settings."},
			{Name: "settings", Kind: usecase.KindObject,
				Description: "Settings to set on top, each on its own, as StructureSettings."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true,
				Description: "The version last read, from the If-Match header over REST."},
		},
		Audit: usecase.AuditDeclaration{
			Action: PresetChangedAction, TargetType: presetTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: presetActivityExempt,
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h ChangePreset) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("preset_id")
	if err != nil {
		return nil, err
	}
	cmd := ChangePresetCommand{PresetID: id, Name: in.String("name"), ExpectedVersion: in.Int("expected_version")}
	if in.String("container_id") != "" {
		if cmd.ContainerID, err = in.ID("container_id"); err != nil {
			return nil, err
		}
	}
	if raw, held := in["settings"].(map[string]any); held {
		if cmd.Settings, err = structureValuesFrom(raw); err != nil {
			return nil, err
		}
	}
	preset, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return presetOutput(preset), nil
}

// WithdrawPreset withdraws a saved preset (UC-WRK-30 check 3): offered no longer, while what was
// made from it keeps its settings. The workspace's default is refused until another is chosen.
type WithdrawPreset struct{ Desk PresetDesk }

// Execute withdraws the preset.
func (h WithdrawPreset) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, expectedVersion int,
) error {
	d := h.Desk
	preset, err := d.writable(ctx, actor, id, PresetWithdrawnAction)
	if err != nil {
		return err
	}
	return d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if preset.Origin == domain.PresetOriginWorkspace && d.Structure.Structures != nil {
			// Held shared, so a change of the default waits for this withdrawal or this one for it.
			workspace, _, err := d.Structure.within(repository.LayerShared).Workspace(ctx)
			if err != nil {
				return err
			}
			if workspace.DefaultPreset == preset.Key {
				return shared.ErrConflict.WithDetail("presets.workspace_default")
			}
		}
		expected := preset.Version
		if expectedVersion != 0 {
			expected = expectedVersion
		}
		if err := d.Presets.Delete(ctx, preset.ID, expected); err != nil {
			return err
		}
		return d.Audit.Append(ctx, presetAudit(ctx, actor, PresetWithdrawnAction, preset, d.Clock.Now()))
	})
}

// Descriptor is the catalogue entry.
func (h WithdrawPreset) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: WithdrawPresetName,
		Summary: "Withdraws a saved preset: it is offered no longer, and hubs and collections made " +
			"from it keep their settings. The workspace's default preset is refused as " +
			"presets.workspace_default until another default is chosen, or none. Rights as for a change.",
		SideEffects: "Removes the preset and writes an audit entry.",
		TokenScope:  containersWrite,
		Destructive: true,
		Input: []usecase.Field{
			{Name: "preset_id", Kind: usecase.KindID, Required: true, Description: "The saved preset."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true,
				Description: "The version last read, from the If-Match header over REST."},
		},
		Audit: usecase.AuditDeclaration{
			Action: PresetWithdrawnAction, TargetType: presetTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: presetActivityExempt,
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h WithdrawPreset) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("preset_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, id, in.Int("expected_version")); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
