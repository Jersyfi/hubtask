// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package workflow holds the use cases of a workspace's own kinds of work: the type registry
// (UC-FLW-01) and, in later steps, the workflows its types move through. Every operation here
// belongs to the workflows module, so the registry's gate refuses it where the module is off
// (UC-FLW-01 check 8).
package workflow

import (
	"context"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
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
	ListEntryTypesName  = "ListEntryTypes"
	CreateEntryTypeName = "CreateEntryType"
	ChangeEntryTypeName = "ChangeEntryType"
	DeleteEntryTypeName = "DeleteEntryType"

	// EntryTypeTarget is the entity name of the audit trail and the change log.
	EntryTypeTarget = "entry_type"

	// Stable audit codes: an auditor filters on them (audit.md §2).
	EntryTypeCreatedAction audit.Action = "entry_type.created"
	EntryTypeChangedAction audit.Action = "entry_type.changed"
	EntryTypeDeletedAction audit.Action = "entry_type.deleted"
	EntryTypesReadAction   audit.Action = "entry_type.read"

	// A type is the workspace's shape, so it shares the scopes of the workspace's configuration.
	typesWrite = "workspace:manage"
	// Reading the registry is what every create needs to offer a type, so an entry's read scope.
	typesRead = "items:read"
)

// Authorizer is the one authorisation question this package asks.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Registry is what every type use case shares.
type Registry struct {
	Types      repository.EntryTypes
	Authorizer Authorizer
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
	// Text brings the names people type to normal form C (i18n-l10n.md §5).
	Text text.Normalizer
}

// Read answers the workspace's registry: the system types as it shaped them and its own.
func (r Registry) Read(ctx context.Context) (domain.EntryTypes, error) {
	rows, err := r.Types.List(ctx)
	if err != nil {
		return nil, err
	}
	return domain.NewEntryTypes(rows), nil
}

// authorizeWrite asks for the workspace's structure permission, before any transaction so that a
// refusal's audit entry is not rolled back with it (audit.md §7).
func (r Registry) authorizeWrite(ctx context.Context, actor appshared.ActorContext, action audit.Action) error {
	return r.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: []identity.Scope{identity.TenantScope()},
		Action: action, TokenScope: typesWrite, TargetType: EntryTypeTarget, TargetID: actor.TenantID,
	})
}

// record writes the change for offline clients and the evidence, in the write's transaction. A
// type is workspace-wide, so its change names no container (offline-sync.md §3.1). The name is a
// person's words and goes to the trail as a fingerprint (rule 10).
func (r Registry) record(
	ctx context.Context, actor appshared.ActorContext, entryType domain.EntryType, op changelog.Operation,
	action audit.Action, changes []domain.FieldChange, now time.Time,
) error {
	var payload map[string]any
	if op != changelog.Delete {
		payload = map[string]any(EntryTypeOutput(entryType))
	}
	if err := r.Changes.Record(ctx, changelog.Change{
		TenantID: actor.TenantID, Entity: EntryTypeTarget, EntityID: entryType.ID, Op: op,
		ActorID: actor.AccountID, HLC: r.HLC.Next(), Payload: payload,
	}); err != nil {
		return err
	}
	trail := make([]audit.Change, 0, len(changes)+1)
	trail = append(trail, audit.Change{Field: "key", Classification: audit.Open, To: entryType.Key})
	for _, change := range changes {
		classification := audit.Open
		if change.Field == domain.FieldName {
			classification = audit.Sensitive
		}
		entry := audit.Change{Field: change.Field, Classification: classification, To: change.To}
		if change.From != "" {
			entry.From = change.From
		}
		trail = append(trail, entry)
	}
	return r.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now, Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: EntryTypeTarget, TargetID: entryType.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(trail...),
	})
}

// EntryTypeOutput is the shape every door returns: the field names of the contract
// (api/openapi.yaml, schema EntryType). An unnamed system type answers a null name, which a client
// fills with its own word for the level (rule 8).
func EntryTypeOutput(t domain.EntryType) usecase.Output {
	children := t.AllowedChildren
	if children == nil {
		children = []string{}
	}
	out := usecase.Output{
		"key": t.Key, "level": string(t.Level), "system": t.System, "name": nil,
		"icon": t.Icon, "color_token": t.ColorToken,
		"completable": t.Behaviours.Completable, "workflow": t.Behaviours.Workflow,
		"record": t.Behaviours.Record, "schedulable": t.Behaviours.Schedulable,
		"estimable": t.Behaviours.Estimable, "allowed_children": children,
		"position": t.Position, "archived": t.IsArchived(), "version": t.Version,
		"form": FormOutput(t.Form),
	}
	if t.Name != "" {
		out["name"] = t.Name
	}
	return out
}

// ListEntryTypes reads the workspace's registry.
type ListEntryTypes struct{ Registry Registry }

// Execute answers every type, archived ones included and marked, in a picker's order. Any signed-in
// member of the workspace may read it: every create offers from it (UC-FLW-01 check 3).
func (h ListEntryTypes) Execute(ctx context.Context, actor appshared.ActorContext) (domain.EntryTypes, error) {
	if err := actor.RequireScope(typesRead); err != nil {
		return nil, err
	}
	var registry domain.EntryTypes
	err := h.Registry.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := h.Registry.Read(ctx)
		registry = read
		return err
	})
	return registry, err
}

// Descriptor is the catalogue entry.
func (h ListEntryTypes) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListEntryTypesName,
		Summary: "Lists the workspace's entry types - the three every workspace has, one per level, " +
			"and its own - by level and order, archived ones included and marked. An entry names its " +
			"type by key; a type of the level the entry is on is what a create may choose.",
		SideEffects: "None. Reads only.",
		TokenScope:  typesRead,
		ReadOnly:    true,
		Module:      module.Workflows,
		Audit: usecase.AuditDeclaration{
			Action: EntryTypesReadAction, TargetType: EntryTypeTarget, Severity: audit.SeverityInfo,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			registry, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(registry))
			for _, t := range registry {
				data = append(data, EntryTypeOutput(t))
			}
			return usecase.Output{"data": data}, nil
		}),
	}
}

// CreateEntryTypeCommand is the input, typed.
type CreateEntryTypeCommand struct {
	Key             string
	Level           domain.ItemType
	Name            string
	Icon            string
	ColorToken      string
	Behaviours      domain.EntryBehaviours
	AllowedChildren []string
	Position        int
}

// CreateEntryType adds a type of the workspace's own (UC-FLW-01 check 1).
type CreateEntryType struct{ Registry Registry }

// Execute creates the type and returns it.
func (h CreateEntryType) Execute(ctx context.Context, actor appshared.ActorContext, cmd CreateEntryTypeCommand) (domain.EntryType, error) {
	if err := h.Registry.authorizeWrite(ctx, actor, EntryTypeCreatedAction); err != nil {
		return domain.EntryType{}, err
	}
	now := h.Registry.Clock.Now()
	created, err := domain.NewEntryType(domain.NewEntryTypeInput{
		ID: h.Registry.IDs.NewID(), TenantID: actor.TenantID, Key: cmd.Key, Level: cmd.Level,
		Name: cmd.Name, Icon: cmd.Icon, ColorToken: cmd.ColorToken, Behaviours: cmd.Behaviours,
		AllowedChildren: cmd.AllowedChildren, Position: cmd.Position, Now: now, Text: h.Registry.Text,
	})
	if err != nil {
		return domain.EntryType{}, err
	}
	err = h.Registry.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		registry, err := h.Registry.Read(ctx)
		if err != nil {
			return err
		}
		if err := requireChildren(registry, created); err != nil {
			return err
		}
		if err := h.Registry.Types.Insert(ctx, created); err != nil {
			return err
		}
		return h.Registry.record(ctx, actor, created, changelog.Upsert, EntryTypeCreatedAction, []domain.FieldChange{
			{Field: domain.FieldName, To: created.Name}, {Field: "level", To: string(created.Level)},
		}, now)
	})
	if err != nil {
		return domain.EntryType{}, err
	}
	return created, nil
}

// requireChildren refuses allowed children that are not types of the level below.
func requireChildren(registry domain.EntryTypes, t domain.EntryType) error {
	for _, key := range t.AllowedChildren {
		child, found := registry.Find(key)
		if !found || child.Level == t.Level || child.Level == domain.ItemTask ||
			(t.Level == domain.ItemTask && child.Level != domain.ItemWorkPackage) {
			return shared.ErrValidation.WithDetail("entry_types.unknown").
				WithParams(map[string]string{"key": key}).
				WithFields(shared.FieldError{Path: "/allowed_children", Code: "entry_types.unknown"})
		}
	}
	return nil
}

func behaviourFields() []usecase.Field {
	return []usecase.Field{
		{Name: "completable", Kind: usecase.KindBool, Description: "Its entries are done or open. Defaults to true; false for a record."},
		{Name: "workflow", Kind: usecase.KindBool, Description: "Its entries move through the type's statuses."},
		{Name: "record", Kind: usecase.KindBool, Description: "Its entries are kept rather than done - a configuration item, a risk, a contact - and have no completion."},
		{Name: "schedulable", Kind: usecase.KindBool, Description: "Its entries carry a due date, reminders and a recurrence. Defaults to true."},
		{Name: "estimable", Kind: usecase.KindBool, Description: "Its entries carry an estimate."},
		{Name: "allowed_children", Kind: usecase.KindList, Description: "The keys of the types of the level below that may sit underneath; empty allows every one."},
		{Name: "position", Kind: usecase.KindInt, Description: "Its place among the types of its level."},
	}
}

// Descriptor is the catalogue entry.
func (h CreateEntryType) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateEntryTypeName,
		Summary: "Adds an entry type of the workspace's own on one level - task, work package or " +
			"activity - with a key, a name, an icon and a colour token, and the behaviours of its " +
			"entries. A record is never completable. The key is free in the workspace and is not one " +
			"of the three system types' keys (task, work_package, activity).",
		SideEffects: "Writes the type, records a change for offline clients, and writes an audit entry.",
		TokenScope:  typesWrite,
		Module:      module.Workflows,
		Input: append([]usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "Small letters, digits and underscores, starting with a letter, at most 50."},
			{Name: "level", Kind: usecase.KindString, Required: true, Enum: []string{string(domain.ItemTask), string(domain.ItemWorkPackage), string(domain.ItemActivity)}, Description: "The level the type sits on."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "Up to 120 characters, on one line."},
			{Name: "icon", Kind: usecase.KindString, Required: true, Description: "The name of an icon of the design system's set."},
			{Name: "color_token", Kind: usecase.KindString, Required: true, Description: "One of the ten label colour tokens."},
		}, behaviourFields()...),
		Audit: usecase.AuditDeclaration{
			Action: EntryTypeCreatedAction, TargetType: EntryTypeTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a type is the workspace's configuration, not an entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			children, err := in.StringList("allowed_children")
			if err != nil {
				return nil, err
			}
			completable, schedulable := true, true
			if in.Present("completable") {
				completable = in.Bool("completable")
			}
			if in.Present("schedulable") {
				schedulable = in.Bool("schedulable")
			}
			if in.Bool("record") && !in.Present("completable") {
				completable = false
			}
			created, err := h.Execute(ctx, actor, CreateEntryTypeCommand{
				Key: in.String("key"), Level: domain.ItemType(in.String("level")), Name: in.String("name"),
				Icon: in.String("icon"), ColorToken: in.String("color_token"),
				Behaviours: domain.EntryBehaviours{
					Completable: completable, Workflow: in.Bool("workflow"), Record: in.Bool("record"),
					Schedulable: schedulable, Estimable: in.Bool("estimable"),
				},
				AllowedChildren: children, Position: in.Int("position"),
			})
			if err != nil {
				return nil, err
			}
			return EntryTypeOutput(created), nil
		}),
	}
}

// ChangeEntryTypeCommand is the input, typed: the key and what to change.
type ChangeEntryTypeCommand struct {
	Key             string
	Change          domain.EntryTypeChange
	ExpectedVersion int
}

// ChangeEntryType renames, recolours, reorders, archives or brings back a type, or changes its
// behaviours (UC-FLW-01 checks 1, 5). A system type is changed through its first row.
type ChangeEntryType struct{ Registry Registry }

// Execute applies the change and returns the type as it now is.
func (h ChangeEntryType) Execute(ctx context.Context, actor appshared.ActorContext, cmd ChangeEntryTypeCommand) (domain.EntryType, error) {
	if cmd.Key == "" {
		return domain.EntryType{}, shared.ErrValidation.WithDetail("entry_types.key_required")
	}
	if err := h.Registry.authorizeWrite(ctx, actor, EntryTypeChangedAction); err != nil {
		return domain.EntryType{}, err
	}
	var changed domain.EntryType
	err := h.Registry.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		registry, err := h.Registry.Read(ctx)
		if err != nil {
			return err
		}
		current, found := registry.Find(cmd.Key)
		if !found {
			return shared.ErrNotFound.WithDetail("entry_types.not_found")
		}
		if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
			return shared.ErrVersionConflict.WithDetail("entry_types.version_conflict")
		}
		now := h.Registry.Clock.Now()
		next, changes, err := current.Changed(cmd.Change, now, h.Registry.Text)
		if err != nil {
			return err
		}
		if err := requireChildren(registry, next); err != nil {
			return err
		}
		changed = next
		if len(changes) == 0 {
			return nil
		}
		if current.Stored() {
			if err := h.Registry.Types.Update(ctx, next, current.Version); err != nil {
				return err
			}
			changed.Version = current.Version + 1
		} else {
			// A system type's first change gives it its row.
			changed.ID, changed.TenantID, changed.CreatedAt, changed.Version = h.Registry.IDs.NewID(), actor.TenantID, now, 1
			if err := h.Registry.Types.Insert(ctx, changed); err != nil {
				return err
			}
		}
		return h.Registry.record(ctx, actor, changed, changelog.Upsert, EntryTypeChangedAction, changes, now)
	})
	if err != nil {
		return domain.EntryType{}, err
	}
	return changed, nil
}

// Descriptor is the catalogue entry.
func (h ChangeEntryType) Descriptor() usecase.Descriptor {
	fields := append([]usecase.Field{
		{Name: "key", Kind: usecase.KindString, Required: true, Description: "The type to change."},
		{Name: "name", Kind: usecase.KindString, Description: "Up to 120 characters; empty on a system type gives it back the level's own word."},
		{Name: "icon", Kind: usecase.KindString, Description: "The name of an icon of the design system's set."},
		{Name: "color_token", Kind: usecase.KindString, Description: "One of the ten label colour tokens."},
		{Name: "archived", Kind: usecase.KindBool, Description: "True archives the type: not offered for new entries, readable on old ones. False brings it back. A system type is never archived."},
		{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read, for optimistic locking."},
	}, behaviourFields()...)
	return usecase.Descriptor{
		Name: ChangeEntryTypeName,
		Summary: "Changes an entry type: its name, icon, colour, behaviours, allowed children or place, " +
			"or archives it or brings it back. Only what is sent changes. A record is never " +
			"completable, and a system type never becomes a record and is never archived.",
		SideEffects: "Writes the type, records a change for offline clients, and writes an audit entry.",
		TokenScope:  typesWrite,
		Module:      module.Workflows,
		Input:       fields,
		Audit: usecase.AuditDeclaration{
			Action: EntryTypeChangedAction, TargetType: EntryTypeTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a type is the workspace's configuration, not an entry."},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h ChangeEntryType) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	var change domain.EntryTypeChange
	change.Name, change.Icon, change.ColorToken = in.OptionalString("name"), in.OptionalString("icon"), in.OptionalString("color_token")
	if in.Present("archived") {
		archived := in.Bool("archived")
		change.Archived = &archived
	}
	if in.Present("position") {
		position := in.Int("position")
		change.Position = &position
	}
	if in.Present("allowed_children") {
		children, err := in.StringList("allowed_children")
		if err != nil {
			return nil, err
		}
		change.AllowedChildren = &children
	}
	key := in.String("key")
	if anyPresent(in, "completable", "workflow", "record", "schedulable", "estimable") {
		// The behaviours are one value: what is not sent keeps what the type has.
		var current domain.EntryType
		err := h.Registry.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			registry, err := h.Registry.Read(ctx)
			current, _ = registry.Find(key)
			return err
		})
		if err != nil {
			return nil, err
		}
		behaviours := current.Behaviours
		set := func(field string, into *bool) {
			if in.Present(field) {
				*into = in.Bool(field)
			}
		}
		set("completable", &behaviours.Completable)
		set("workflow", &behaviours.Workflow)
		set("record", &behaviours.Record)
		set("schedulable", &behaviours.Schedulable)
		set("estimable", &behaviours.Estimable)
		if in.Bool("record") && !in.Present("completable") {
			behaviours.Completable = false
		}
		change.Behaviours = &behaviours
	}
	changed, err := h.Execute(ctx, actor, ChangeEntryTypeCommand{
		Key: key, Change: change, ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return EntryTypeOutput(changed), nil
}

func anyPresent(in usecase.Input, fields ...string) bool {
	for _, field := range fields {
		if in.Present(field) {
			return true
		}
	}
	return false
}

// DeleteEntryType removes a type of the workspace's own that no entry carries; one in use can only
// be archived (UC-FLW-01 check 5).
type DeleteEntryType struct{ Registry Registry }

// Execute deletes the type.
func (h DeleteEntryType) Execute(ctx context.Context, actor appshared.ActorContext, key string) error {
	if key == "" {
		return shared.ErrValidation.WithDetail("entry_types.key_required")
	}
	if err := h.Registry.authorizeWrite(ctx, actor, EntryTypeDeletedAction); err != nil {
		return err
	}
	return h.Registry.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Registry.Types.Find(ctx, key)
		if err != nil {
			return err
		}
		if current.System {
			return shared.ErrConflict.WithDetail("entry_types.system_not_deletable")
		}
		inUse, err := h.Registry.Types.InUse(ctx, key)
		if err != nil {
			return err
		}
		if inUse {
			return shared.ErrConflict.WithDetail("entry_types.in_use")
		}
		if err := h.Registry.Types.Delete(ctx, key); err != nil {
			return err
		}
		return h.Registry.record(ctx, actor, current, changelog.Delete, EntryTypeDeletedAction, nil, h.Registry.Clock.Now())
	})
}

// Descriptor is the catalogue entry.
func (h DeleteEntryType) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteEntryTypeName,
		Summary: "Deletes an entry type of the workspace's own that no entry carries, trashed ones " +
			"included. A type in use is refused with entry_types.in_use and can only be archived; a " +
			"system type is never deleted.",
		SideEffects: "Removes the type, records a deletion for offline clients, and writes an audit entry.",
		TokenScope:  typesWrite,
		Module:      module.Workflows,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The type to delete."},
		},
		Audit: usecase.AuditDeclaration{
			Action: EntryTypeDeletedAction, TargetType: EntryTypeTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a type is the workspace's configuration, not an entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Execute(ctx, actor, in.String("key")); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	}
}
