// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

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
// before the caller has seen it; asked again with DropValues, the change clears those values in the
// same transaction. A completion is never dropped this way: reopening has its own consequences for
// the parent (CompleteWorkItem), so the entry is reopened first.
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
	// Gate answers the types the collection offers on the entry's level (UC-FLW-01 check 3).
	Gate EntryGate
	// Recurrences removes the series a confirmed change drops; unwired, a recurrence is never
	// dropped and its change stays refused.
	Recurrences repository.Recurrences
	// Workflow and Statuses settle the entry in the workflow in force for its new type, as a move
	// into another collection does (WorkflowGate.Settle). Unwired, the status stays as it was.
	Workflow WorkflowGate
	Statuses StatusMover
}

// SetItemEntryTypeCommand is the input, typed.
type SetItemEntryTypeCommand struct {
	ItemID          shared.ID
	EntryType       string
	ExpectedVersion int
	// DropValues confirms the values the new type does not carry, after a refusal named them.
	DropValues bool
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
		offer, err := h.Gate.placing(ctx, collection, parent)
		if err != nil {
			return err
		}
		offered := offer.entryTypesOn(current.Type)
		if cmd.EntryType != "" && cmd.EntryType == current.EntryType {
			// Keeping the type an entry has is no choice: a narrowed offer never takes it away.
			offered = nil
		}
		key, narrowed, err := chooseEntryType(ctx, h.EntryTypes, current.Type, cmd.EntryType, parent, profile, offered)
		if err != nil {
			return err
		}
		if key == current.EntryType {
			changed = current
			return nil
		}
		now := h.Clock.Now()
		dropped := current.DroppedBy(narrowed)
		if err := h.refuseDrop(cmd, dropped); err != nil {
			return err
		}
		if len(dropped) > 0 {
			if current, err = h.drop(ctx, actor, current, dropped, now); err != nil {
				return err
			}
		}
		before := current.EntryType
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
		if changed, err = h.settle(ctx, actor, collection, next, now); err != nil {
			return err
		}
		changes := []audit.Change{{
			Field: "entry_type", Classification: audit.Open,
			From: entryTypeKeyOf(domain.WorkItem{Type: current.Type, EntryType: before}), To: entryTypeKeyOf(next),
		}}
		for _, field := range dropped {
			// The field's name only: what it held is the entry's, and a count is not content.
			changes = append(changes, audit.Change{
				Field: "dropped." + field, Classification: audit.Open, From: "set", To: "",
			})
		}
		return h.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now, Action: ItemEntryTypeSetAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: itemTarget, TargetID: next.ID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(changes...),
		})
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return changed, nil
}

// settle moves the entry's status where the workflow in force for its new type stands: kept where
// that workflow knows it, entered otherwise, cleared where none runs. No step is taken - a change
// of type is not a transition (UC-FLW-05 check 6).
func (h SetItemEntryType) settle(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, next domain.WorkItem, now time.Time,
) (domain.WorkItem, error) {
	if h.Statuses.Workflows == nil {
		return next, nil
	}
	moves, err := h.Workflow.Settle(ctx, collection)
	if err != nil || len(moves) == 0 {
		return next, err
	}
	if _, err := h.Statuses.Move(ctx, actor, moves, now); err != nil {
		return next, err
	}
	return findItem(ctx, h.Items, next.ID)
}

// refuseDrop names the values a change would drop, unless the caller confirmed them - and a
// completion, or a series nothing here can remove, even then.
func (h SetItemEntryType) refuseDrop(cmd SetItemEntryTypeCommand, dropped []string) error {
	if len(dropped) == 0 {
		return nil
	}
	if cmd.DropValues {
		var kept []string
		if slices.Contains(dropped, "completion") {
			kept = append(kept, "completion")
		}
		if slices.Contains(dropped, "recurrence") && h.Recurrences == nil {
			kept = append(kept, "recurrence")
		}
		if len(kept) == 0 {
			return nil
		}
		return droppedRefusal("entry_types.value_not_droppable", kept)
	}
	return droppedRefusal("entry_types.values_dropped", dropped)
}

func droppedRefusal(code string, dropped []string) error {
	fields := make([]shared.FieldError, 0, len(dropped))
	for _, field := range dropped {
		fields = append(fields, shared.FieldError{Path: "/" + field, Code: code})
	}
	return shared.ErrConflict.WithDetail(code).
		WithParams(map[string]string{"fields": strings.Join(dropped, ", ")}).
		WithFields(fields...)
}

// drop clears the confirmed values in the change's transaction - the series first, then the due
// date - and answers the entry as it now stands, its version moved by each write.
func (h SetItemEntryType) drop(
	ctx context.Context, actor appshared.ActorContext, current domain.WorkItem, dropped []string,
	now time.Time,
) (domain.WorkItem, error) {
	if slices.Contains(dropped, "recurrence") {
		rule, err := h.Recurrences.FindForItem(ctx, current.ID)
		switch {
		case err == nil:
			if err := h.Recurrences.Delete(ctx, rule, rule.Version); err != nil {
				return domain.WorkItem{}, err
			}
			if err := h.Changes.Record(ctx, changelog.Change{
				TenantID: actor.TenantID, Entity: recurrenceEntity, EntityID: rule.ID,
				Op: changelog.Delete, ContainerID: current.CollectionID, ActorID: actor.AccountID,
				HLC: h.HLC.Next(),
			}); err != nil {
				return domain.WorkItem{}, err
			}
		case !errors.Is(err, shared.ErrNotFound):
			return domain.WorkItem{}, err
		}
		current.RecurrenceRuleID = ""
	}
	if slices.Contains(dropped, "due_at") {
		// Clearing asks no capability, so the profile is no question here.
		cleared, changes, err := current.WithDueDate(nil, domain.CapabilityProfile{}, now)
		if err != nil {
			return domain.WorkItem{}, err
		}
		if err := h.Items.SetDueDate(ctx, cleared, current.Version); err != nil {
			return domain.WorkItem{}, err
		}
		for _, change := range changes {
			if err := h.Changes.Record(ctx, changelog.Change{
				TenantID: actor.TenantID, Entity: itemTarget, EntityID: current.ID,
				Op: changelog.Upsert, ContainerID: current.CollectionID, ActorID: actor.AccountID,
				HLC: h.HLC.Next(), Field: change.Field, Payload: map[string]any{change.Field: change.To},
			}); err != nil {
				return domain.WorkItem{}, err
			}
		}
		cleared.Version = current.Version + 1
		current = cleared
	}
	return current, nil
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
		Name:       SetItemEntryTypeName,
		Permission: service.PermissionWriteItems,
		Summary: "Gives an entry another type of its level; the level's own key (task, work_package, " +
			"activity) gives it back its level's type. An archived type, another level's, or one the " +
			"parent's type does not allow underneath is refused. A type that would not carry a value " +
			"the entry holds is refused with entry_types.values_dropped, naming each field, unless " +
			"drop_values confirms them; one whose form " +
			"the entry does not meet is refused with forms.refused, naming each field.",
		SideEffects: "Writes the entry's type, settles its status in the workflow in force for the new " +
			"type, records a change for offline clients, and writes an audit entry.",
		TokenScope: itemsWrite,
		Module:     module.Workflows,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."},
			{Name: "entry_type", Kind: usecase.KindString, Required: true, Description: "The key of the type to give it."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read, for optimistic locking."},
			{
				Name: "drop_values", Kind: usecase.KindBool,
				Description: "Confirms the values the new type does not carry - a due date, a " +
					"series - after a refusal named them; they are cleared with the change. A " +
					"completion is never dropped: reopen the entry first.",
			},
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
				DropValues: in.Bool("drop_values"),
			})
			if err != nil {
				return nil, err
			}
			return ItemOutput(item), nil
		}),
	}
}
