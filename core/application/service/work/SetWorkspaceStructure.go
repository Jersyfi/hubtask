// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"strconv"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	SetWorkspaceStructureName = "SetWorkspaceStructure"
	workspaceManageScope      = "workspace:manage"
	workspaceStructureTarget  = "workspace"
)

// WorkspaceStructureUpdatedAction is the audit code of a change to the workspace's structure: its
// values, its locks or its default preset.
const WorkspaceStructureUpdatedAction audit.Action = "workspace.structure_updated"

// SetWorkspaceStructure changes the workspace's own structure (UC-WRK-23 checks 2, 8; the lock
// writer UX-10's check 7 needs): values every hub and collection inherits unless it has its own,
// locks that bind them all, and the preset a new hub starts from.
//
// No step-up: a structure narrows what is offered and never who gets in (identity.md §5 asks one
// for the sign-in rule alone). Not destructive, for SetContainerStructure's reason.
type SetWorkspaceStructure struct {
	Structure  StructureResolver
	Authorizer Authorizer
	Changes    changelog.ChangeLog
	Epochs     changelog.Epochs
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	HLC        clock.HLCSource
	Text       text.Normalizer
}

// SetWorkspaceStructureCommand is the input, typed.
type SetWorkspaceStructureCommand struct {
	Preset   domain.PresetKey
	Settings domain.StructureValues
	Reset    []domain.StructureSetting
	// Locks is the whole set of locks after the change; nil leaves them as they are.
	Locks []string
	// DefaultPreset sets the preset a new hub starts from; ClearDefaultPreset removes it.
	DefaultPreset      domain.PresetKey
	ClearDefaultPreset bool
	ExpectedVersion    int
}

// WorkspaceStructureResult is the workspace's structure after the change, with the tenant row's
// version for the next If-Match.
type WorkspaceStructureResult struct {
	Workspace domain.WorkspaceStructure
	Structure domain.EffectiveStructure
	Version   int
}

// Execute changes the workspace's structure.
func (h SetWorkspaceStructure) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd SetWorkspaceStructureCommand,
) (WorkspaceStructureResult, error) {
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     WorkspaceStructureUpdatedAction,
		TokenScope: workspaceManageScope,
		TargetType: workspaceStructureTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return WorkspaceStructureResult{}, err
	}
	if !h.Structure.Wired() {
		return WorkspaceStructureResult{}, shared.ErrInternal.WithDetail("containers.structure_unwired")
	}

	var result WorkspaceStructureResult
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		written, err := h.write(ctx, actor, cmd)
		result = written
		return err
	})
	if err != nil {
		return WorkspaceStructureResult{}, err
	}
	return result, nil
}

func (h SetWorkspaceStructure) write(
	ctx context.Context, actor appshared.ActorContext, cmd SetWorkspaceStructureCommand,
) (WorkspaceStructureResult, error) {
	now := h.Clock.Now()
	// The tenant row is held for the whole change, so every hub's and collection's write - which
	// holds it shared - waits, and the fan-out below reads their layers as they stand.
	reading := h.Structure.within(repository.LayerExclusive)
	current, version, err := reading.Workspace(ctx)
	if err != nil {
		return WorkspaceStructureResult{}, err
	}
	if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != version {
		return WorkspaceStructureResult{}, shared.ErrVersionConflict.
			WithDetail("workspace.version_conflict").
			WithParams(map[string]string{"current_version": strconv.Itoa(version)})
	}
	bound, err := reading.Bound(ctx)
	if err != nil {
		return WorkspaceStructureResult{}, err
	}

	next, err := h.decide(bound, current, actor, cmd)
	if err != nil {
		return WorkspaceStructureResult{}, err
	}
	before := domain.ResolveStructure(bound, current)
	after := domain.ResolveStructure(bound, next)
	moved := current.Layer.Values.Moved(next.Layer.Values)
	locksMoved := !sameLocks(current.Locks, next.Locks)
	if len(moved) == 0 && !locksMoved && current.Layer.Preset == next.Layer.Preset &&
		current.DefaultPreset == next.DefaultPreset {
		return WorkspaceStructureResult{Workspace: current, Structure: before, Version: version}, nil
	}

	if err := h.Structure.Structures.SetWorkspace(ctx, next, version, now); err != nil {
		return WorkspaceStructureResult{}, err
	}
	reading.workspace = &next
	reading.version = version + 1

	if len(moved) > 0 || locksMoved || current.Layer.Preset != next.Layer.Preset {
		if err := fanOut(ctx, h.Structure, h.Epochs, h.Changes, h.HLC, reading, actor, ""); err != nil {
			return WorkspaceStructureResult{}, err
		}
	}
	if err := h.recordAudit(ctx, actor, current, next, moved, now); err != nil {
		return WorkspaceStructureResult{}, err
	}
	return WorkspaceStructureResult{Workspace: next, Structure: after, Version: version + 1}, nil
}

// decide is the workspace's next layer. A lock already there keeps its author; a new one is the
// actor's.
func (h SetWorkspaceStructure) decide(
	bound domain.StructureBound, current domain.WorkspaceStructure, actor appshared.ActorContext,
	cmd SetWorkspaceStructureCommand,
) (domain.WorkspaceStructure, error) {
	next := domain.WorkspaceStructure{
		Layer: current.Layer, Locks: current.Locks, DefaultPreset: current.DefaultPreset,
	}
	if cmd.Preset != "" {
		preset, err := domain.FindPreset(cmd.Preset, bound)
		if err != nil {
			return domain.WorkspaceStructure{}, err
		}
		next.Layer = domain.StructureLayer{Preset: cmd.Preset, Values: preset.Values}
	}
	if len(cmd.Settings.Decided()) > 0 {
		validated, err := bound.Validate(cmd.Settings, h.Text)
		if err != nil {
			return domain.WorkspaceStructure{}, err
		}
		next.Layer.Values = next.Layer.Values.Overlay(validated)
	}
	for _, setting := range cmd.Reset {
		next.Layer.Values = next.Layer.Values.Without(setting)
	}
	if cmd.Locks != nil {
		locks := make(map[domain.LockKey]shared.ID, len(cmd.Locks))
		for _, raw := range cmd.Locks {
			key, err := domain.ParseLockKey(raw, bound)
			if err != nil {
				return domain.WorkspaceStructure{}, err
			}
			if by, held := current.Locks[key]; held {
				locks[key] = by
			} else {
				locks[key] = actor.AccountID
			}
		}
		next.Locks = locks
	}
	if cmd.ClearDefaultPreset {
		next.DefaultPreset = ""
	} else if cmd.DefaultPreset != "" {
		if _, err := domain.FindPreset(cmd.DefaultPreset, bound); err != nil {
			return domain.WorkspaceStructure{}, err
		}
		next.DefaultPreset = cmd.DefaultPreset
	}
	return next, nil
}

func sameLocks(a, b map[domain.LockKey]shared.ID) bool {
	if len(a) != len(b) {
		return false
	}
	for key := range a {
		if _, held := b[key]; !held {
			return false
		}
	}
	return true
}

// recordAudit writes the evidence: the settings that moved (the levels hashed), the locks and the
// default preset.
func (h SetWorkspaceStructure) recordAudit(
	ctx context.Context, actor appshared.ActorContext, from, to domain.WorkspaceStructure,
	moved []domain.StructureSetting, now time.Time,
) error {
	changes := make([]audit.Change, 0, len(moved)+3)
	for _, setting := range moved {
		classification := audit.Open
		if setting == domain.SettingLevels {
			classification = audit.Sensitive
		}
		changes = append(changes, audit.Change{
			Field: "structure." + string(setting), Classification: classification,
			From: settingText(setting, from.Layer.Values), To: settingText(setting, to.Layer.Values),
		})
	}
	if from.Layer.Preset != to.Layer.Preset {
		changes = append(changes, audit.Change{
			Field: "preset", Classification: audit.Open,
			From: presetOrNil(from.Layer.Preset), To: presetOrNil(to.Layer.Preset),
		})
	}
	if !sameLocks(from.Locks, to.Locks) {
		changes = append(changes, audit.Change{
			Field: "locks", Classification: audit.Open, From: lockNames(from.Locks), To: lockNames(to.Locks),
		})
	}
	if from.DefaultPreset != to.DefaultPreset {
		changes = append(changes, audit.Change{
			Field: "default_preset", Classification: audit.Open,
			From: presetOrNil(from.DefaultPreset), To: presetOrNil(to.DefaultPreset),
		})
	}
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: now,
		Action:     WorkspaceStructureUpdatedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: workspaceStructureTarget,
		TargetID:   actor.TenantID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	})
}

func lockNames(locks map[domain.LockKey]shared.ID) []string {
	names := make([]string, 0, len(locks))
	for key := range locks {
		names = append(names, string(key))
	}
	slices.Sort(names)
	return names
}

// Descriptor is the catalogue entry.
func (h SetWorkspaceStructure) Descriptor() usecase.Descriptor {
	presets := []string{
		string(domain.PresetList), string(domain.PresetTasks),
		string(domain.PresetProject), string(domain.PresetFlow),
	}
	return usecase.Descriptor{
		Name: SetWorkspaceStructureName,
		Summary: "Changes the workspace's own structure: the values every hub and collection " +
			"inherits unless it has its own (a preset copied, settings set, settings reset), the " +
			"locks that bind every hub and collection (a setting such as views, or one capability " +
			"of one type such as fields.TASK.DUE_DATE), and the preset a new hub starts from. A " +
			"value beyond what the installation offers is refused as capability_not_supported.",
		SideEffects: "Writes the workspace's structure, records the structure in force of every hub " +
			"and collection for offline clients (past a thousand, advances the workspace's " +
			"synchronisation epoch instead), and writes an audit entry.",
		TokenScope: workspaceManageScope,
		Input: []usecase.Field{
			{Name: "preset", Kind: usecase.KindString, Enum: presets,
				Description: "A preset to copy into the workspace's values, replacing them."},
			{Name: "settings", Kind: usecase.KindObject,
				Description: "Settings to set, each on its own, as StructureSettings."},
			{Name: "reset", Kind: usecase.KindList,
				Description: "Names of settings the workspace leaves to Hubtask's default again."},
			{Name: "locks", Kind: usecase.KindList,
				Description: "The whole set of locks after the change: settings, or fields.<TYPE>.<CAPABILITY>. " +
					"Omitted leaves the locks as they are; an empty list removes them all."},
			{Name: "default_preset", Kind: usecase.KindString, Enum: presets,
				Description: "The preset a hub created without one starts from."},
			{Name: "clear_default_preset", Kind: usecase.KindBool,
				Description: "Removes the default preset: new hubs then start from the workspace's values."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true,
				Description: "The workspace's version last read, from the If-Match header over REST."},
		},
		Audit: usecase.AuditDeclaration{
			Action: WorkspaceStructureUpdatedAction, TargetType: workspaceStructureTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the workspace is not an item, and the history is an item's: `ActivityEntry` is " +
				"keyed on `itemId`. A change of the workspace's structure is in the audit trail.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SetWorkspaceStructure) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd := SetWorkspaceStructureCommand{
		Preset:             domain.PresetKey(in.String("preset")),
		DefaultPreset:      domain.PresetKey(in.String("default_preset")),
		ClearDefaultPreset: in.Bool("clear_default_preset"),
		ExpectedVersion:    in.Int("expected_version"),
	}
	var err error
	if raw, held := in["settings"].(map[string]any); held {
		if cmd.Settings, err = structureValuesFrom(raw); err != nil {
			return nil, err
		}
	}
	if cmd.Reset, err = settingsResetFrom(in); err != nil {
		return nil, err
	}
	if in.Present("locks") {
		if cmd.Locks, err = in.StringList("locks"); err != nil {
			return nil, err
		}
		if cmd.Locks == nil {
			cmd.Locks = []string{}
		}
	}

	result, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	out := usecase.Output(workspaceStructureOutput(result.Structure, result.Workspace))
	out["version"] = result.Version
	return out, nil
}
