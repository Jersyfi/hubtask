// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

const (
	TrashWorkItemName   = "TrashWorkItem"
	RestoreWorkItemName = "RestoreWorkItem"

	// The audit codes. This is the first deletion path in the system, so the two directions are
	// separate codes rather than one with a flag: "what was deleted here" is the question an auditor
	// asks, and answering it must not require reading a change list (audit.md §2).
	ItemTrashedAction  audit.Action = "item.trashed"
	ItemRestoredAction audit.Action = "item.restored"
)

// TrashWorkItem moves an entry and everything under it into the trash.
//
// A soft delete: the rows stay, stamped with when they went and with the batch that names this one
// deletion (I-C2). What removes them for good is the retention job, once the tenant's period has
// run - and until then a restore brings back exactly this batch and nothing else.
//
// The subtree goes with it because a deletion that left an entry's children behind would leave them
// unreachable rather than deleted: their parent is gone from every list they would be read through.
type TrashWorkItem struct {
	Lifecycle LifecycleWriter
}

// RestoreWorkItem takes one deletion back out of the trash, whole.
type RestoreWorkItem struct {
	Lifecycle LifecycleWriter
}

// Execute puts the entry and its subtree into the trash and returns the entry as it now stands.
func (h TrashWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd LifecycleCommand,
) (domain.WorkItem, error) {
	return h.Lifecycle.change(ctx, actor, cmd, trashing)
}

// Execute restores the deletion the entry belongs to and returns the entry as it now stands.
func (h RestoreWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd LifecycleCommand,
) (domain.WorkItem, error) {
	return h.Lifecycle.change(ctx, actor, cmd, restoring)
}

var (
	trashing = itemVerb{
		action: ItemTrashedAction,
		step:   activity.ItemTrashed,
		// A fresh identifier per deletion, from the generator port rather than from time or chance
		// (arc42 §8.13). It is what the whole subtree is stamped with and what a restore is keyed on.
		batch: func(_ domain.WorkItem, ids clock.IDGenerator) shared.ID { return ids.NewID() },
		apply: func(item domain.WorkItem, now time.Time, batch shared.ID) (domain.WorkItem, []domain.FieldChange, error) {
			return item.Trashed(now, batch)
		},
		store: func(ctx context.Context, items repository.Items, trash repository.ItemTrash) (int, error) {
			return items.TrashSubtree(ctx, trash)
		},
		announce: announceSubtree(event.NewItemTrashed),
		// A deletion, so that an offline client removes its local copy rather than applying a state
		// change to something it should no longer show (offline-sync.md §3.1).
		op: changelog.Delete,
		// The clock starts here, so the sweep that will read it is asked for here.
		schedulesRetention: true,
	}
	restoring = itemVerb{
		action: ItemRestoredAction,
		step:   activity.ItemRestored,
		// The batch already on the row, read before the transition clears it. Restoring is an act on
		// the deletion rather than on the entry, and the deletion is what the batch names.
		batch: func(item domain.WorkItem, _ clock.IDGenerator) shared.ID { return item.TrashBatchID },
		apply: func(item domain.WorkItem, now time.Time, _ shared.ID) (domain.WorkItem, []domain.FieldChange, error) {
			return item.Restored(now)
		},
		store: func(ctx context.Context, items repository.Items, trash repository.ItemTrash) (int, error) {
			return items.RestoreBatch(ctx, trash)
		},
		announce: announceSubtree(event.NewItemRestored),
		op:       changelog.Upsert,
		guard:    ensureSomewhereToComeBackTo,
	}
)

// ensureSomewhereToComeBackTo refuses a restore that would put an entry under something that is
// itself in the trash.
//
// The collection is checked by the writer for every verb - a trashed collection is not editable -
// so what is left is the entry above this one. Without this, restoring a work package whose task
// was deleted separately would produce a live entry hanging off a deleted one: visible in no list,
// because every list it would be read through starts at its parent, and quietly resurrected the day
// somebody restored that parent.
//
// The answer names the parent, so that a client can offer to restore that instead.
func ensureSomewhereToComeBackTo(
	ctx context.Context, w LifecycleWriter, item domain.WorkItem,
) error {
	if item.ParentID.IsZero() {
		return nil
	}
	parent, err := findItem(ctx, w.Items, item.ParentID)
	if err != nil {
		return err
	}
	if !parent.IsTrashed() {
		return nil
	}
	return shared.ErrConflict.
		WithDetail("items.parent_trashed").
		WithParams(map[string]string{"parent_id": parent.ID.String()})
}

// Descriptor is the catalogue entry.
func (h TrashWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: TrashWorkItemName,
		Summary: "Moves a task, work package or activity and everything under it into the trash. A " +
			"soft delete: the entries stay for the tenant's retention period and can be restored as " +
			"one act until the retention job removes them for good. An entry already in the trash " +
			"from an earlier deletion keeps that deletion rather than joining this one. Idempotent.",
		SideEffects: "Writes the deletion stamp over the subtree, announces " +
			string(event.ItemTrashed) + ", records a deletion for offline clients, and writes an " +
			"audit entry.",
		TokenScope: itemsWrite,
		// The MCP annotation an agent client reads before asking a person to confirm
		// (ai-first.md §1.1). Reversible, and still destructive: what it takes with it is a subtree
		// somebody may not have realised was there.
		Destructive: true,
		Input:       lifecycleInput("The entry to move to the trash, with everything under it."),
		Audit: usecase.AuditDeclaration{
			Action: ItemTrashedAction, TargetType: itemTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemTrashed},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h RestoreWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RestoreWorkItemName,
		Summary: "Takes one deletion back out of the trash, whole. Exactly the entries that went in " +
			"together come back: a separate, younger deletion inside the same subtree stays where it " +
			"is. An entry that was archived when it was deleted comes back archived - restoring " +
			"undoes the deletion and nothing else. Refused while the entry above it is in the trash. " +
			"Idempotent.",
		SideEffects: "Clears the deletion stamp over the batch, announces " +
			string(event.ItemRestored) + ", records a change for offline clients, and writes an " +
			"audit entry.",
		TokenScope: itemsWrite,
		Input:      lifecycleInput("Any entry of the deletion to restore."),
		Audit: usecase.AuditDeclaration{
			Action: ItemRestoredAction, TargetType: itemTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemRestored},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h TrashWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	return invokeLifecycle(ctx, actor, in, h.Execute)
}

func (h RestoreWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	return invokeLifecycle(ctx, actor, in, h.Execute)
}
