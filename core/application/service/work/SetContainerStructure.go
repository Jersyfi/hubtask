// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"fmt"
	"slices"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	SetContainerStructureName = "SetContainerStructure"

	// structureFanOutCeiling bounds the change log rows one structure write records for the
	// containers that inherit it. Past it the workspace's synchronisation epoch advances instead and
	// every device takes a fresh snapshot, as after a restore (offline-sync.md §8): a workspace with
	// thousands of collections changes its structure rarely, and a transaction writing thousands of
	// rows for it would hold every one of them.
	structureFanOutCeiling = 1000
)

// ContainerStructureUpdatedAction is the audit code of a change to a hub's or a collection's own
// structure.
const ContainerStructureUpdatedAction audit.Action = "container.structure_updated"

// SetContainerStructure changes how much structure a hub or a collection has (UC-WRK-23 checks 2,
// 5, 8): a preset chosen, settings changed one by one, settings reset to the layer above.
//
// Not destructive, and declared so: a structure says what is offered, never what is kept - every
// value an entry holds stays stored, and comes back when the setting does (decision #1620).
type SetContainerStructure struct {
	Writer    ContainerWriter
	Structure StructureResolver
	// Epochs is advanced instead of a fan-out past structureFanOutCeiling.
	Epochs changelog.Epochs
}

// SetContainerStructureCommand is the input, typed.
type SetContainerStructureCommand struct {
	ContainerID shared.ID
	// Preset is the preset to copy into the container's own layer, replacing its own values; empty
	// for none.
	Preset domain.PresetKey
	// Settings are set on top, each on its own; only the ones decided here change.
	Settings domain.StructureValues
	// Reset are the settings that go back to the layer above.
	Reset []domain.StructureSetting
	// ExpectedVersion is from If-Match; zero accepts whatever is stored.
	ExpectedVersion int
}

// StructureResult is a structure write's answer: the container, its structure in force, and the
// locks that kept a value a chosen preset would have changed.
type StructureResult struct {
	Container  domain.Container
	Structure  domain.EffectiveStructure
	KeptByLock []domain.LockKey
}

// Execute changes the container's own layer and returns it with the structure in force.
func (h SetContainerStructure) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd SetContainerStructureCommand,
) (StructureResult, error) {
	w := h.Writer
	if cmd.ContainerID.IsZero() {
		return StructureResult{}, containerIDRequired()
	}
	current, err := w.read(ctx, actor, cmd.ContainerID)
	if err != nil {
		return StructureResult{}, err
	}
	// Before the transaction: a refusal writes an audit entry that must outlive the refusal.
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       containerPath(current),
		Action:     ContainerStructureUpdatedAction,
		TokenScope: containersWrite,
		TargetType: containerTarget,
		TargetID:   cmd.ContainerID,
	}); err != nil {
		return StructureResult{}, err
	}
	if !h.Structure.Wired() {
		return StructureResult{}, shared.ErrInternal.WithDetail("containers.structure_unwired")
	}

	var result StructureResult
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		written, err := h.write(ctx, actor, cmd)
		result = written
		return err
	})
	if err != nil {
		return StructureResult{}, err
	}
	return result, nil
}

func (h SetContainerStructure) write(
	ctx context.Context, actor appshared.ActorContext, cmd SetContainerStructureCommand,
) (StructureResult, error) {
	w := h.Writer
	now := w.Clock.Now()

	// The workspace's row and the hub's are held before the layers are read, so a change to either
	// waits for this one and its fan-out finds this layer (record UX-09, D10).
	reading := h.Structure.within(repository.LayerShared)
	if _, _, err := reading.Workspace(ctx); err != nil {
		return StructureResult{}, err
	}
	container, err := findContainer(ctx, w.Containers, cmd.ContainerID)
	if err != nil {
		return StructureResult{}, err
	}
	if err := ensureContainerVersion(container, cmd.ExpectedVersion); err != nil {
		return StructureResult{}, err
	}
	if err := container.EnsureEditable(); err != nil {
		return StructureResult{}, err
	}

	layer, kept, err := h.decide(ctx, reading, container, cmd)
	if err != nil {
		return StructureResult{}, err
	}
	before, err := reading.Of(ctx, container)
	if err != nil {
		return StructureResult{}, err
	}
	ownMoved := container.Structure.Values.Moved(layer.Values)
	if len(ownMoved) == 0 && container.Structure.Preset == layer.Preset {
		// Writing what is stored writes nothing and announces nothing.
		return StructureResult{Container: container, Structure: before, KeptByLock: kept}, nil
	}

	updated := container
	updated.Structure = layer
	updated.UpdatedAt = now
	if updated.Type == domain.ContainerCollection {
		updated.CompletionPolicy = layer.Values.Completion.OrDefault()
	}
	if err := h.Structure.Structures.SetContainerStructure(ctx, updated, container.Version); err != nil {
		return StructureResult{}, err
	}
	updated.Version = container.Version + 1
	if updated.Type == domain.ContainerHub {
		reading.hubs[updated.ID] = updated
	}

	after, err := reading.Of(ctx, updated)
	if err != nil {
		return StructureResult{}, err
	}
	moved := structureChangeSet(before, after, ownMoved, container.Structure.Preset, layer.Preset)
	announcement, err := event.NewContainerStructureChanged(
		w.IDs.NewID(), updated, moved, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return StructureResult{}, err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return StructureResult{}, err
	}
	if err := recordStructure(ctx, w.Changes, w.HLC, updated, actor, after); err != nil {
		return StructureResult{}, err
	}
	if updated.Type == domain.ContainerHub {
		if err := fanOut(ctx, h.Structure, h.Epochs, w.Changes, w.HLC, reading, actor, updated.ID); err != nil {
			return StructureResult{}, err
		}
	}
	if err := recordStructureAudit(ctx, w.Audit, ContainerStructureUpdatedAction, actor, updated.ID,
		container.Structure, layer, ownMoved, now); err != nil {
		return StructureResult{}, err
	}
	return StructureResult{Container: updated, Structure: after, KeptByLock: kept}, nil
}

// decide is the container's new own layer: a preset chosen replaces it, held to the locks; the
// settings sent are set on top, refused against a lock; the settings reset go to the layer above.
func (h SetContainerStructure) decide(
	ctx context.Context, reading *layerReading, container domain.Container, cmd SetContainerStructureCommand,
) (domain.StructureLayer, []domain.LockKey, error) {
	layer := container.Structure
	var kept []domain.LockKey
	if cmd.Preset != "" {
		chosen, keptBy, err := choosePreset(ctx, reading, container, cmd.Preset)
		if err != nil {
			return domain.StructureLayer{}, nil, err
		}
		layer, kept = chosen, keptBy
	}
	if len(cmd.Settings.Decided()) > 0 {
		bound, err := reading.Bound(ctx)
		if err != nil {
			return domain.StructureLayer{}, nil, err
		}
		validated, err := bound.Validate(cmd.Settings, h.Writer.Text)
		if err != nil {
			return domain.StructureLayer{}, nil, err
		}
		above, err := reading.Above(ctx, container)
		if err != nil {
			return domain.StructureLayer{}, nil, err
		}
		if broken := domain.LockedAgainst(bound, above, validated); len(broken) > 0 {
			return domain.StructureLayer{}, nil, domain.ErrSettingLocked(broken[0], above.Locks[broken[0]])
		}
		layer.Values = layer.Values.Overlay(validated)
	}
	for _, setting := range cmd.Reset {
		layer.Values = layer.Values.Without(setting)
	}
	return layer, kept, nil
}

// fanOut tells offline clients the structure in force of every container that inherits the layer
// just written: a hub's collections, or with no hub every hub and collection. Past the ceiling it
// advances the workspace's synchronisation epoch instead.
func fanOut(
	ctx context.Context, resolver StructureResolver, epochs changelog.Epochs, changes changelog.ChangeLog,
	hlc clock.HLCSource, reading *layerReading, actor appshared.ActorContext, hubID shared.ID,
) error {
	heirs, err := resolver.Structures.Heirs(ctx, hubID, structureFanOutCeiling+1)
	if err != nil {
		return err
	}
	if len(heirs) > structureFanOutCeiling {
		if epochs == nil {
			return shared.ErrInternal.WithDetail("containers.structure_unwired")
		}
		_, err := epochs.Advance(ctx)
		return err
	}
	for _, heir := range heirs {
		effective, err := reading.Of(ctx, heir)
		if err != nil {
			return err
		}
		if err := recordStructure(ctx, changes, hlc, heir, actor, effective); err != nil {
			return err
		}
	}
	return nil
}

// structureChangeSet is what the event's change set says under `structure`: each setting whose own
// value or value in force moved, with the value in force on both sides, in the contract's words
// (S6). Level names are among the values: the event reaches only those who may read the container,
// who read its name already; the audit trail hashes them (rule 10).
func structureChangeSet(
	before, after domain.EffectiveStructure, ownMoved []domain.StructureSetting,
	presetBefore, presetAfter domain.PresetKey,
) map[string]any {
	moved := map[string]any{}
	for _, setting := range domain.StructureSettingNames() {
		if !slices.Contains(ownMoved, setting) && before.Values.SameSetting(setting, after.Values) &&
			before.SourceOf(setting) == after.SourceOf(setting) {
			continue
		}
		moved[string(setting)] = map[string]any{
			"from": structureSettingOutput(setting, before.Values),
			"to":   structureSettingOutput(setting, after.Values),
		}
	}
	if presetBefore != presetAfter {
		moved["preset"] = map[string]any{"from": presetOrNil(presetBefore), "to": presetOrNil(presetAfter)}
	}
	return moved
}

func presetOrNil(key domain.PresetKey) any {
	if key == "" {
		return nil
	}
	return string(key)
}

// recordStructureAudit writes the evidence: which settings of the layer moved and, except the
// levels, what they now say. The levels carry names people chose, which are their content: hashed,
// never in clear (rule 10, audit.md §4).
func recordStructureAudit(
	ctx context.Context, sink audit.Sink, action audit.Action, actor appshared.ActorContext,
	targetID shared.ID, from, to domain.StructureLayer, moved []domain.StructureSetting, now time.Time,
) error {
	changes := make([]audit.Change, 0, len(moved)+1)
	for _, setting := range moved {
		classification := audit.Open
		if setting == domain.SettingLevels {
			classification = audit.Sensitive
		}
		changes = append(changes, audit.Change{
			Field: "structure." + string(setting), Classification: classification,
			From: settingText(setting, from.Values), To: settingText(setting, to.Values),
		})
	}
	if from.Preset != to.Preset {
		changes = append(changes, audit.Change{
			Field: "preset", Classification: audit.Open,
			From: presetOrNil(from.Preset), To: presetOrNil(to.Preset),
		})
	}
	return sink.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: now,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: containerTarget,
		TargetID:   targetID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	})
}

// settingText is one setting's own value as the trail records it, nil where the layer leaves it to
// the one above.
func settingText(setting domain.StructureSetting, values domain.StructureValues) any {
	if !values.Has(setting) {
		return nil
	}
	return fmt.Sprint(structureSettingOutput(setting, values))
}

// Descriptor is the catalogue entry.
func (h SetContainerStructure) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetContainerStructureName,
		Summary: "Changes how much structure a hub or a collection has: chooses a preset (its eight " +
			"settings are copied, replacing the container's own), sets settings one by one, or " +
			"resets them to what the hub or the workspace says. A value beyond what the " +
			"installation offers is refused as capability_not_supported; a setting the " +
			"workspace locked is refused as structure.setting_locked, except that a preset chosen " +
			"under a lock keeps the locked setting and says so in kept_by_lock. Nothing an entry " +
			"holds is changed or removed. Writing what is stored writes nothing.",
		SideEffects: "Writes the container's own structure, announces " +
			string(event.ContainerPoliciesUpdated) + " with the settings that moved, records the " +
			"structure in force for offline clients - for a hub, for each collection that follows " +
			"it too - and writes an audit entry.",
		TokenScope: containersWrite,
		Input: []usecase.Field{
			{
				Name: "container_id", Kind: usecase.KindID, Required: true,
				Description: "The hub or collection.",
			},
			{
				Name: "preset", Kind: usecase.KindString,
				Enum: []string{
					string(domain.PresetList), string(domain.PresetTasks),
					string(domain.PresetProject), string(domain.PresetFlow),
				},
				Description: "A preset to copy, replacing the container's own values.",
			},
			{
				Name: "settings", Kind: usecase.KindObject,
				Description: "Settings to set, each on its own, as StructureSettings: levels, fields, " +
					"views, counts, completion, capture, celebration, automation. Applied after the " +
					"preset.",
			},
			{
				Name: "reset", Kind: usecase.KindList,
				Description: "Names of settings that go back to the layer above. Applied last.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true,
				Description: "The version last read, from the If-Match header over REST.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ContainerStructureUpdatedAction, TargetType: containerTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a container is not an item, and the history is an item's: `ActivityEntry` is " +
				"keyed on `itemId` and `/items/{id}/activity` is its only reader. A container's " +
				"own history has nowhere to be read from.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SetContainerStructure) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return nil, err
	}
	cmd := SetContainerStructureCommand{
		ContainerID: containerID, Preset: domain.PresetKey(in.String("preset")),
		ExpectedVersion: in.Int("expected_version"),
	}
	if raw, held := in["settings"].(map[string]any); held {
		if cmd.Settings, err = structureValuesFrom(raw); err != nil {
			return nil, err
		}
	}
	if cmd.Reset, err = settingsResetFrom(in); err != nil {
		return nil, err
	}

	result, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return structureResultOutput(result), nil
}

// structureResultOutput is the container, its structure in force, and the locks that kept a value.
func structureResultOutput(result StructureResult) usecase.Output {
	out := containerOutput(result.Container)
	out["structure"] = effectiveStructureOutput(result.Structure)
	kept := make([]any, 0, len(result.KeptByLock))
	for _, key := range result.KeptByLock {
		kept = append(kept, string(key))
	}
	out["kept_by_lock"] = kept
	return out
}
