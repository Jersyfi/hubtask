// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The installation's presets (UC-INS-18): what an operator offers every workspace for its hubs and
// collections, the default a hub starts from where its workspace chose none, and the settings the
// default locks. Beside the plans, and kept like them: behind `admin:tenants` and the operator
// register, in multi mode only - in a single installation the workspace's own presets are the one
// level (P-10) - and written by the instance file alone while it enforces the level.
//
// The operator never sees a workspace's hubs or collections, so a preset starts from one of
// Hubtask's or from Hubtask's default, never from a container.

const (
	ListInstallationPresetsName    = "ListInstallationPresets"
	CreateInstallationPresetName   = "CreateInstallationPreset"
	ChangeInstallationPresetName   = "ChangeInstallationPreset"
	WithdrawInstallationPresetName = "WithdrawInstallationPreset"

	installationPresetTarget = "installation_preset"

	installationPresetsReadAction     audit.Action = "instance.presets_read"
	installationPresetCreatedAction   audit.Action = "instance.preset_created"
	installationPresetChangedAction   audit.Action = "instance.preset_changed"
	installationPresetWithdrawnAction audit.Action = "instance.preset_withdrawn"
)

// InstallationPresetWriter is what the preset use cases share: the instance writer for the
// register, the level and the journal; the presets; the profiles they are held to; the mode.
type InstallationPresetWriter struct {
	Instance InstanceWriter
	Presets  workrepo.InstallationPresets
	Profiles metarepo.CapabilityProfiles
	Tenancy  env.TenancyMode
	Text     text.Normalizer
}

func (w InstallationPresetWriter) authorize(ctx context.Context, actor appshared.ActorContext) error {
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return err
	}
	return requireMulti(w.Tenancy)
}

// InstallationPresetCommand is what a create or a change carries. A field left nil or unset is
// left as it is; a create starts from Hubtask's default.
type InstallationPresetCommand struct {
	Name *string
	// From is one of Hubtask's presets whose values the preset takes, before Settings.
	From     domain.PresetKey
	Settings domain.StructureValues
	// IsDefault makes the preset the default, or none the default.
	IsDefault *bool
	// Locks is the whole set of locks, where LocksSet says the caller named it.
	Locks    []string
	LocksSet bool
}

// ListInstallationPresets answers the installation's presets.
type ListInstallationPresets struct{ Writer InstallationPresetWriter }

// Execute lists them.
func (h ListInstallationPresets) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.InstallationPreset, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor); err != nil {
		return nil, err
	}
	var found []domain.InstallationPreset
	err := w.Instance.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			read, err := w.Presets.List(ctx)
			found = read
			return err
		})
	return found, err
}

// CreateInstallationPreset adds one.
type CreateInstallationPreset struct{ Writer InstallationPresetWriter }

// Execute creates it.
func (h CreateInstallationPreset) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd InstallationPresetCommand,
) (domain.InstallationPreset, error) {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return domain.InstallationPreset{}, err
	}
	return h.Writer.create(ctx, actor, cmd, false)
}

func (w InstallationPresetWriter) create(
	ctx context.Context, actor appshared.ActorContext, cmd InstallationPresetCommand, fromFile bool,
) (domain.InstallationPreset, error) {
	var created domain.InstallationPreset
	err := w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		if err := w.notEnforced(ctx, fromFile); err != nil {
			return err
		}
		count, err := w.Presets.Count(ctx)
		if err != nil {
			return err
		}
		if count >= domain.MaxInstallationPresets {
			return domain.ErrPresetsLimitReached(domain.PresetOriginInstallation)
		}
		bound, err := w.bound(ctx)
		if err != nil {
			return err
		}
		name := ""
		if cmd.Name != nil {
			name = *cmd.Name
		}
		saved, err := domain.NewSavedPreset(domain.NewSavedPresetInput{
			ID: w.Instance.IDs.NewID(), Origin: domain.PresetOriginInstallation, Name: name,
			Values: bound.Default(), Text: w.Text,
		})
		if err != nil {
			return err
		}
		preset, err := w.apply(bound, domain.InstallationPreset{Preset: saved}, cmd)
		if err != nil {
			return err
		}
		now := w.Instance.Clock.Now()
		if preset.IsDefault {
			if err := w.Presets.ClearDefault(ctx, preset.ID, actor.AccountID, now); err != nil {
				return err
			}
		}
		if err := w.Presets.Insert(ctx, preset, actor.AccountID, now); err != nil {
			return err
		}
		created = preset
		return w.record(ctx, actor, installationPresetCreatedAction, preset, now)
	})
	return created, err
}

// ChangeInstallationPreset changes one.
type ChangeInstallationPreset struct{ Writer InstallationPresetWriter }

// Execute changes it. With `expected`, a preset somebody changed since it was read is refused.
func (h ChangeInstallationPreset) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, cmd InstallationPresetCommand, expected *int,
) (domain.InstallationPreset, error) {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return domain.InstallationPreset{}, err
	}
	return h.Writer.change(ctx, actor, id, cmd, expected, false)
}

func (w InstallationPresetWriter) change(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, cmd InstallationPresetCommand,
	expected *int, fromFile bool,
) (domain.InstallationPreset, error) {
	var changed domain.InstallationPreset
	err := w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		if err := w.notEnforced(ctx, fromFile); err != nil {
			return err
		}
		found, err := w.Presets.Find(ctx, id)
		if err != nil {
			return err
		}
		bound, err := w.bound(ctx)
		if err != nil {
			return err
		}
		preset, err := w.apply(bound, found, cmd)
		if err != nil {
			return err
		}
		now := w.Instance.Clock.Now()
		if preset.IsDefault && !found.IsDefault {
			if err := w.Presets.ClearDefault(ctx, preset.ID, actor.AccountID, now); err != nil {
				return err
			}
		}
		if err := w.Presets.Update(ctx, preset, expected, actor.AccountID, now); err != nil {
			return err
		}
		preset.Version = found.Version + 1
		changed = preset
		return w.record(ctx, actor, installationPresetChangedAction, preset, now)
	})
	return changed, err
}

// WithdrawInstallationPreset removes one. Hubs and collections made from it keep their settings.
type WithdrawInstallationPreset struct{ Writer InstallationPresetWriter }

// Execute withdraws it; the default is refused until another preset, or none, is the default.
func (h WithdrawInstallationPreset) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, expected *int,
) error {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return err
	}
	return h.Writer.withdraw(ctx, actor, id, expected, false)
}

func (w InstallationPresetWriter) withdraw(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, expected *int, fromFile bool,
) error {
	return w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		if err := w.notEnforced(ctx, fromFile); err != nil {
			return err
		}
		found, err := w.Presets.Find(ctx, id)
		if err != nil {
			return err
		}
		if found.IsDefault {
			return domain.ErrInstallationDefault()
		}
		if err := w.Presets.Delete(ctx, id, expected); err != nil {
			return err
		}
		return w.record(ctx, actor, installationPresetWithdrawnAction, found, w.Instance.Clock.Now())
	})
}

// apply is a command on a preset, in the contract's order: the name, Hubtask's preset, the
// settings, the default, the locks. Every value is held to the system profile, which is the most
// any workspace is offered; a workspace narrower still is offered it clipped.
func (w InstallationPresetWriter) apply(
	bound domain.StructureBound, preset domain.InstallationPreset, cmd InstallationPresetCommand,
) (domain.InstallationPreset, error) {
	if cmd.Name != nil {
		name, err := domain.PresetName(*cmd.Name, w.Text)
		if err != nil {
			return domain.InstallationPreset{}, err
		}
		preset.Name = name
	}
	if cmd.From != "" {
		if !cmd.From.BuiltIn() {
			return domain.InstallationPreset{}, domain.ErrPresetUnknown(cmd.From)
		}
		from, err := domain.FindPreset(cmd.From, bound)
		if err != nil {
			return domain.InstallationPreset{}, err
		}
		preset.Values = bound.Default().Overlay(from.Values)
	}
	if len(cmd.Settings.Decided()) > 0 {
		validated, err := bound.Validate(cmd.Settings, w.Text)
		if err != nil {
			return domain.InstallationPreset{}, err
		}
		preset.Values = preset.Values.Overlay(validated)
	}
	preset.Values = domain.WithoutCategories(preset.Values)
	if cmd.IsDefault != nil {
		preset.IsDefault = *cmd.IsDefault
		if !preset.IsDefault {
			preset.Locks = nil
		}
	}
	if cmd.LocksSet {
		locks, err := domain.ParseLockKeys(cmd.Locks, bound)
		if err != nil {
			return domain.InstallationPreset{}, err
		}
		if len(locks) > 0 && !preset.IsDefault {
			return domain.InstallationPreset{}, domain.ErrLocksNeedDefault()
		}
		preset.Locks = locks
	}
	return preset, nil
}

// bound is the system profile: what an installation preset may offer at most.
func (w InstallationPresetWriter) bound(ctx context.Context) (domain.StructureBound, error) {
	system, err := w.Profiles.ListSystem(ctx)
	if err != nil {
		return domain.StructureBound{}, err
	}
	return domain.NewStructureBound(system, system), nil
}

func (w InstallationPresetWriter) notEnforced(ctx context.Context, fromFile bool) error {
	level, err := w.Instance.Settings.Read(ctx)
	if err != nil {
		return err
	}
	if level.IsEnforcedFromFile && !fromFile {
		return enforcedFromFile(level)
	}
	return nil
}

// record writes the journal entry: the preset's id, whether it is the default and what it locks.
// The name is the operator's text and stays out of a place nothing ever deletes from (rule 10).
func (w InstallationPresetWriter) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, preset domain.InstallationPreset,
	at time.Time,
) error {
	locks := make([]string, 0, len(preset.Locks))
	for _, key := range preset.Locks {
		locks = append(locks, string(key))
	}
	return w.Instance.Journal.Record(ctx, adminrepo.InstanceEvent{
		ID: w.Instance.IDs.NewID(), OccurredAt: at, Action: string(action), ActorLabel: actor.AccountName,
		Details: map[string]any{"preset_id": preset.ID.String(), "is_default": preset.IsDefault, "locks": locks},
	})
}

// installationPresetCommandFrom reads a command from a document: the REST body, a tool call, or
// the instance file.
func installationPresetCommandFrom(in usecase.Input) (InstallationPresetCommand, error) {
	cmd := InstallationPresetCommand{From: domain.PresetKey(in.String("preset"))}
	if _, named := in["name"]; named {
		name := in.String("name")
		cmd.Name = &name
	}
	if raw, held := in["settings"].(map[string]any); held {
		settings, err := workservice.StructureValuesFrom(raw)
		if err != nil {
			return InstallationPresetCommand{}, err
		}
		cmd.Settings = settings
	}
	if value, named := in["is_default"]; named && value != nil {
		isDefault := in.Bool("is_default")
		cmd.IsDefault = &isDefault
	}
	if value, named := in["locks"]; named && value != nil {
		locks, err := in.StringList("locks")
		if err != nil {
			return InstallationPresetCommand{}, err
		}
		cmd.Locks, cmd.LocksSet = locks, true
	}
	return cmd, nil
}

func installationPresetOutput(preset domain.InstallationPreset) usecase.Output {
	locks := make([]any, 0, len(preset.Locks))
	for _, key := range preset.Locks {
		locks = append(locks, string(key))
	}
	return usecase.Output{
		"id": preset.ID.String(), "name": preset.Name, "version": preset.Version,
		"settings":   workservice.StructureValuesOutput(preset.Values),
		"is_default": preset.IsDefault, "locks": locks,
	}
}

var installationPresetActivityExempt = usecase.ActivityDeclaration{
	Exempt: "The installation's configuration is not an entry, and the item history is keyed on an entry.",
}

// installationPresetFields are the values a create or a change carries.
func installationPresetFields() []usecase.Field {
	return []usecase.Field{
		{Name: "name", Kind: usecase.KindString, Description: "The name every workspace sees, up to 100 characters."},
		{Name: "preset", Kind: usecase.KindString,
			Description: "One of Hubtask's presets - LIST, TASKS, PROJECT or FLOW - whose settings it takes."},
		{Name: "settings", Kind: usecase.KindObject,
			Description: "Settings to set on top, each on its own, as StructureSettings."},
		{Name: "is_default", Kind: usecase.KindBool,
			Description: "Makes it the default a hub starts from where its workspace chose none, or none the default."},
		{Name: "locks", Kind: usecase.KindList,
			Description: "The whole set of what the default locks for every workspace: a setting (views) " +
				"or one capability of one level (fields.TASK.DUE_DATE). Only the default locks."},
	}
}

// Descriptor is the catalogue entry.
func (h ListInstallationPresets) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListInstallationPresetsName,
		Summary: "The presets this installation offers every workspace, by name, with their eight " +
			"settings, which one is the default a hub starts from where its workspace chose none, " +
			"and what the default locks. Multi mode only.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: installationPresetsReadAction, TargetType: installationPresetTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListInstallationPresets) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	found, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(found))
	for _, preset := range found {
		rows = append(rows, installationPresetOutput(preset))
	}
	return usecase.Output{"data": rows}, nil
}

// Descriptor is the catalogue entry.
func (h CreateInstallationPreset) Descriptor() usecase.Descriptor {
	fields := installationPresetFields()
	fields[0].Required = true
	return usecase.Descriptor{
		Name: CreateInstallationPresetName,
		Summary: "Adds a preset every workspace of this installation is offered, marked as the " +
			"installation's. Its settings start from one of Hubtask's presets or from Hubtask's " +
			"default, with settings on top, held to what the installation offers. Multi mode only; " +
			"refused while a file enforces the installation's level.",
		SideEffects: "Writes the preset and one journal entry; making it the default lifts the former " +
			"default and its locks, which then bind every workspace at its next request.",
		TokenScope: adminTenantsScope,
		Input:      fields,
		Audit: usecase.AuditDeclaration{
			Action: installationPresetCreatedAction, TargetType: installationPresetTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: installationPresetActivityExempt,
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateInstallationPreset) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := installationPresetCommandFrom(in)
	if err != nil {
		return nil, err
	}
	preset, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return installationPresetOutput(preset), nil
}

// Descriptor is the catalogue entry.
func (h ChangeInstallationPreset) Descriptor() usecase.Descriptor {
	fields := append([]usecase.Field{
		{Name: "preset_id", Kind: usecase.KindID, Required: true, Description: "The installation's preset."},
	}, installationPresetFields()...)
	fields = append(fields, usecase.Field{
		Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true,
		Description: "The version last read, from the If-Match header over REST.",
	})
	return usecase.Descriptor{
		Name: ChangeInstallationPresetName,
		Summary: "Renames a preset of the installation, copies one of Hubtask's into it again, sets " +
			"settings on top, makes it the default or not, and sets what the default locks for every " +
			"workspace. Hubs and collections made from it keep their settings. Multi mode only.",
		SideEffects: "Writes the preset and one journal entry; a change of the default or its locks " +
			"binds every workspace at its next request.",
		TokenScope: adminTenantsScope,
		Input:      fields,
		Audit: usecase.AuditDeclaration{
			Action: installationPresetChangedAction, TargetType: installationPresetTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: installationPresetActivityExempt,
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h ChangeInstallationPreset) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("preset_id")
	if err != nil {
		return nil, err
	}
	cmd, err := installationPresetCommandFrom(in)
	if err != nil {
		return nil, err
	}
	preset, err := h.Execute(ctx, actor, id, cmd, expectedVersionOf(in))
	if err != nil {
		return nil, err
	}
	return installationPresetOutput(preset), nil
}

// Descriptor is the catalogue entry.
func (h WithdrawInstallationPreset) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: WithdrawInstallationPresetName,
		Summary: "Withdraws a preset of the installation: no workspace is offered it any more, and " +
			"hubs and collections made from it keep their settings. The default is refused until " +
			"another preset, or none, is the default. Multi mode only.",
		SideEffects: "Deletes the preset and writes one journal entry.",
		TokenScope:  adminTenantsScope,
		Destructive: true,
		Input: []usecase.Field{
			{Name: "preset_id", Kind: usecase.KindID, Required: true, Description: "The installation's preset."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true,
				Description: "The version last read, from the If-Match header over REST."},
		},
		Audit: usecase.AuditDeclaration{
			Action: installationPresetWithdrawnAction, TargetType: installationPresetTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: installationPresetActivityExempt,
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h WithdrawInstallationPreset) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("preset_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, id, expectedVersionOf(in)); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// expectedVersionOf is the version a caller named, nil where it named none.
func expectedVersionOf(in usecase.Input) *int {
	if _, named := in["expected_version"]; !named {
		return nil
	}
	version := in.Int("expected_version")
	if version == 0 {
		return nil
	}
	return &version
}
