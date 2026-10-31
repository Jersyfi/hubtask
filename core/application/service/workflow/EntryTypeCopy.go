// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package workflow

import (
	"context"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// CopyEntryTypeHereName is the catalogue name.
const CopyEntryTypeHereName = "CopyEntryTypeHere"

// CopyEntryTypeHereCommand is the input, typed.
type CopyEntryTypeHereCommand struct {
	SourceKey string
	HubID     shared.ID
	Key       string
	// Name is the copy's own name; nil keeps the source's.
	Name *string
}

// CopyEntryTypeHere makes a hub's own copy of a type shared with it (UC-FLW-01 check 11): a
// shared type is read-only at the hub, and a hub that needs it different copies it.
type CopyEntryTypeHere struct{ Registry Registry }

// Execute makes the copy and returns it.
func (h CopyEntryTypeHere) Execute(ctx context.Context, actor appshared.ActorContext, cmd CopyEntryTypeHereCommand) (domain.EntryType, error) {
	if cmd.SourceKey == "" {
		return domain.EntryType{}, shared.ErrValidation.WithDetail("entry_types.key_required")
	}
	if err := h.Registry.authorizeAt(ctx, actor, EntryTypeCreatedAction, cmd.HubID); err != nil {
		return domain.EntryType{}, err
	}
	var copied domain.EntryType
	err := h.Registry.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Registry.requireHub(ctx, cmd.HubID); err != nil {
			return err
		}
		read, err := h.Registry.Read(ctx)
		if err != nil {
			return err
		}
		// Only what the hub may use is a source: another hub's type is not this one's to see.
		registry := read.Within(cmd.HubID)
		source, found := registry.Find(cmd.SourceKey)
		if !found {
			return shared.ErrNotFound.WithDetail("entry_types.not_found")
		}
		now := h.Registry.Clock.Now()
		copied, err = source.CopyAt(domain.EntryTypeCopy{
			ID: h.Registry.IDs.NewID(), TenantID: actor.TenantID, HubID: cmd.HubID, Key: cmd.Key,
			Name: cmd.Name, Now: now, Text: h.Registry.Text,
		})
		if err != nil {
			return err
		}
		if err := requireChildren(registry, copied); err != nil {
			return err
		}
		if err := h.Registry.Types.Insert(ctx, copied); err != nil {
			return err
		}
		return h.Registry.record(ctx, actor, copied, changelog.Upsert, EntryTypeCreatedAction, []domain.FieldChange{
			{Field: domain.FieldName, To: copied.Name}, {Field: "level", To: string(copied.Level)},
			{Field: "copied_from", To: copied.CopiedFrom},
		}, now)
	})
	if err != nil {
		return domain.EntryType{}, err
	}
	return copied, nil
}

// Descriptor is the catalogue entry.
func (h CopyEntryTypeHere) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionStructure,
		Name:       CopyEntryTypeHereName,
		Summary: "Makes a hub's own copy of an entry type shared with it - the workspace's or the " +
			"hub's own - under a new key: the level, icon, colour, behaviours, allowed children and " +
			"form come along, and the copy names the key it came from. The copy is offered only " +
			"inside the hub; the source does not change.",
		SideEffects: "Writes the copy, records a change for offline clients, and writes an audit entry.",
		TokenScope:  typesWrite,
		Module:      module.Workflows,
		Input: []usecase.Field{
			{Name: "source_key", Kind: usecase.KindString, Required: true, Description: "The key of the type to copy."},
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub the copy belongs to."},
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The copy's own key, free in the workspace."},
			{Name: "name", Kind: usecase.KindString, Description: "The copy's name; unset keeps the source's."},
		},
		Audit: usecase.AuditDeclaration{
			Action: EntryTypeCreatedAction, TargetType: EntryTypeTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a type is the workspace's configuration, not an entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			copied, err := h.Execute(ctx, actor, CopyEntryTypeHereCommand{
				SourceKey: in.String("source_key"), HubID: hub, Key: in.String("key"),
				Name: in.OptionalString("name"),
			})
			if err != nil {
				return nil, err
			}
			return EntryTypeOutput(copied), nil
		}),
	}
}
