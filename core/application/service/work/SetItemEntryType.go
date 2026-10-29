// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"strings"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	SetItemEntryTypeName = "SetItemEntryType"

	// ItemEntryTypeSetAction is the audit code of an entry's change of type. Stable (audit.md §2).
	ItemEntryTypeSetAction audit.Action = "item.entry_type_set"
)

// SetItemEntryType gives an entry another type of its level (UC-FLW-01 check 4). A type that would
// not carry a value the entry holds is refused, naming each such field, so that nothing is lost
// before the caller has seen it.
type SetItemEntryType struct {
	Items      repository.Items
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
	EntryTypes repository.EntryTypes
	Authorizer Authorizer
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	HLC        clock.HLCSource
	// Forms holds the entry to its new type's form as a creation of the type would be: a required
	// field it lacks or a validation it fails refuses the change, by field.
	Forms FormGate
}

// SetItemEntryTypeCommand is the input, typed.
type SetItemEntryTypeCommand struct {
	ItemID          shared.ID
	EntryType       string
	ExpectedVersion int
}

// Execute changes the entry's type and returns the entry.
func (h SetItemEntryType) Execute(ctx context.Context, actor appshared.ActorContext, cmd SetItemEntryTypeCommand) (domain.WorkItem, error) {
	if cmd.ItemID.IsZero() {
		return domain.WorkItem{}, itemIDRequired()
	}
	var (
		item       domain.WorkItem
		collection domain.Container
	)
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := findItem(ctx, h.Items, cmd.ItemID)
		if err != nil {
			return err
		}
		item = found
		collection, err = h.Containers.Find(ctx, found.CollectionID)
		return err
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	// Before the transaction, so a refusal's audit entry survives it (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems, Path: containerPath(collection),
		Action: ItemEntryTypeSetAction, TokenScope: itemsWrite, TargetType: itemTarget,
		TargetID: cmd.ItemID, On: changing(item),
	}); err != nil {
		return domain.WorkItem{}, err
	}

	var changed domain.WorkItem
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := findItem(ctx, h.Items, cmd.ItemID)
		if err != nil {
			return err
		}
		if err := current.EnsureEditable(); err != nil {
			return err
		}
		if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
			return shared.ErrVersionConflict.WithDetail("items.version_conflict")
		}
		var parent *domain.WorkItem
		if !current.ParentID.IsZero() {
			found, err := findItem(ctx, h.Items, current.ParentID)
			if err != nil {
				return err
			}
			parent = &found
		}
		profile, err := profileOf(ctx, h.Profiles, current.Type)
		if err != nil {
			return err
		}
		key, narrowed, err := chooseEntryType(ctx, h.EntryTypes, current.Type, cmd.EntryType, parent, profile)
		if err != nil {
			return err
		}
		if key == current.EntryType {
			changed = current
			return nil
		}
		if dropped := current.DroppedBy(narrowed); len(dropped) > 0 {
			fields := make([]shared.FieldError, 0, len(dropped))
			for _, field := range dropped {
				fields = append(fields, shared.FieldError{Path: "/" + field, Code: "entry_types.values_dropped"})
			}
			return shared.ErrConflict.WithDetail("entry_types.values_dropped").
				WithParams(map[string]string{"fields": strings.Join(dropped, ", ")}).
				WithFields(fields...)
		}
		before := current.EntryType
		now := h.Clock.Now()
		next := current
		next.EntryType, next.UpdatedAt = key, now
		if err := h.Forms.checkCreate(ctx, actor, collection, next); err != nil {
			return err
		}
		if err := h.EntryTypes.SetOf(ctx, next, current.Version); err != nil {
			return err
		}
		next.Version = current.Version + 1
		changed = next
		if err := h.Changes.Record(ctx, changelog.Change{
			TenantID: actor.TenantID, Entity: "item", EntityID: next.ID, Op: changelog.Upsert,
			ContainerID: next.CollectionID, ActorID: actor.AccountID, HLC: h.HLC.Next(),
			Field: "entry_type", Payload: map[string]any{"entry_type": entryTypeKeyOf(next)},
		}); err != nil {
			return err
		}
		return h.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now, Action: ItemEntryTypeSetAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: itemTarget, TargetID: next.ID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(audit.Change{
				Field: "entry_type", Classification: audit.Open,
				From: entryTypeKeyOf(domain.WorkItem{Type: current.Type, EntryType: before}), To: entryTypeKeyOf(next),
			}),
		})
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return changed, nil
}

// entryTypeKeyOf is the key every door answers for an entry's type: its own, or its level's.
func entryTypeKeyOf(item domain.WorkItem) string {
	if item.EntryType != "" {
		return item.EntryType
	}
	return domain.SystemEntryTypeKey(item.Type)
}

// Descriptor is the catalogue entry.
func (h SetItemEntryType) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetItemEntryTypeName,
		Summary: "Gives an entry another type of its level; the level's own key (task, work_package, " +
			"activity) gives it back its level's type. An archived type, another level's, or one the " +
			"parent's type does not allow underneath is refused. A type that would not carry a value " +
			"the entry holds is refused with entry_types.values_dropped, naming each field; one whose form " +
			"the entry does not meet is refused with forms.refused, naming each field.",
		SideEffects: "Writes the entry's type, records a change for offline clients, and writes an audit entry.",
		TokenScope:  itemsWrite,
		Module:      module.Workflows,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."},
			{Name: "entry_type", Kind: usecase.KindString, Required: true, Description: "The key of the type to give it."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read, for optimistic locking."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemEntryTypeSetAction, TargetType: itemTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the history of an entry's type is the audit trail's until the workflow step brings its activity verb.",
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			item, err := h.Execute(ctx, actor, SetItemEntryTypeCommand{
				ItemID: itemID, EntryType: in.String("entry_type"), ExpectedVersion: in.Int("expected_version"),
			})
			if err != nil {
				return nil, err
			}
			return ItemOutput(item), nil
		}),
	}
}
