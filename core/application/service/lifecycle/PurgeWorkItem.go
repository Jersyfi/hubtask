// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	work "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	PurgeWorkItemName = "PurgeWorkItem"
	EmptyTrashName    = "EmptyTrash"

	itemsWrite      = "items:write"
	containersWrite = "containers:write"

	// The audit codes. Both are warnings where the rest of the lifecycle is an info or a notice:
	// nothing else in this system destroys data, and this is the entry somebody is looking for when
	// work has gone and nobody knows where (audit.md §2).
	ItemPurgedAction   audit.Action = "item.purged"
	TrashEmptiedAction audit.Action = "trash.emptied"
)

// Authorizer is the slice of the authorisation service these use cases need. Declared here rather
// than imported from the work context, so that a use case in this package depends on the question
// it asks and not on a neighbouring context's interface list.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// PurgeWorkItem removes one entry and everything under it for good, before its period is up.
//
// The right to write entries, not the owner's right to delete a container. The trash already
// accepted this deletion under that right, and purging only skips the waiting - what it does not do
// is take anything with it that the deletion did not already take. Emptying a whole trash is a
// different question and is asked differently below.
type PurgeWorkItem struct {
	Items      workrepo.Items
	Containers workrepo.Containers
	Purger     Purger
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// EmptyTrash removes everything in the trash for good, in one pass.
//
// The owner's right to delete a container, because that is what it may reach: a trash holds hubs and
// collections as readily as entries, and somebody who may not delete a hub must not be able to
// delete one by emptying a trash it happens to be in (domain-model.md §3.2).
type EmptyTrash struct {
	Purger     Purger
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// PurgeWorkItemCommand is the input, typed.
type PurgeWorkItemCommand struct {
	ItemID shared.ID
}

// Execute removes the entry and its subtree, and reports how many rows went.
func (h PurgeWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd PurgeWorkItemCommand,
) (int, error) {
	if cmd.ItemID.IsZero() {
		return 0, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}

	// The entry and its collection are read before the permission question, because the answer
	// depends on the path: a membership at the hub applies downwards (domain-model.md §3.2).
	var item work.WorkItem
	var collection work.Container
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if item, err = h.find(ctx, cmd.ItemID); err != nil {
			return err
		}
		collection, err = h.Containers.Find(ctx, item.CollectionID)
		return err
	})
	if err != nil {
		return 0, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and one written inside
	// this transaction would be rolled back together with the refusal (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       pathOf(collection),
		Action:     ItemPurgedAction,
		TokenScope: itemsWrite,
		TargetType: itemTarget,
		TargetID:   cmd.ItemID,
	}); err != nil {
		return 0, err
	}

	var removed int
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := h.Purger.Clock.Now()

		item, err := h.find(ctx, cmd.ItemID)
		if err != nil {
			return err
		}
		// Only out of the trash. A live entry is deleted by deleting it, which is reversible and is
		// what a client should be doing - and an endpoint that removed a live entry for good would
		// make the trash something a caller could skip rather than something it goes through.
		if !item.IsTrashed() {
			return shared.ErrConflict.
				WithDetail("items.not_trashed").
				WithParams(map[string]string{"item_id": item.ID.String()})
		}

		removed, err = h.Purger.Subtree(
			ctx, actor, item, collection.ParentID, domain.DeletedByUser, now)
		if err != nil {
			return err
		}
		return h.Purger.RecordAudit(ctx, actor, ItemPurgedAction, itemTarget, item.ID,
			Outcome{Matched: removed, Removed: removed}, domain.DeletedByUser, now)
	})
	if err != nil {
		return 0, err
	}
	return removed, nil
}

// Execute empties the trash and reports what it removed.
func (h EmptyTrash) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (Outcome, error) {
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionDeleteContainer,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     TrashEmptiedAction,
		TokenScope: containersWrite,
		TargetType: trashTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return Outcome{}, err
	}

	var outcome Outcome
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := h.Purger.Clock.Now()

		var err error
		outcome, err = h.Purger.Sweep(ctx, actor, Selection{
			// Everything in it. The cutoff is now rather than a period, because a person emptying a
			// trash is not asking for the old part of it.
			Cutoff: now,
			Reason: domain.DeletedByUser,
			// An explicit act: see Selection.ObserveTombstoneWindow.
			ObserveTombstoneWindow: false,
		}, now)
		if err != nil {
			return err
		}
		return h.Purger.RecordAudit(ctx, actor, TrashEmptiedAction, trashTarget, actor.TenantID,
			outcome, domain.DeletedByUser, now)
	})
	if err != nil {
		return Outcome{}, err
	}
	return outcome, nil
}

// find reads an entry, or says it does not exist in the words a client can act on.
func (h PurgeWorkItem) find(ctx context.Context, id shared.ID) (work.WorkItem, error) {
	item, err := h.Items.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return work.WorkItem{}, shared.ErrNotFound.
				WithDetail("items.not_found").
				WithParams(map[string]string{"item_id": id.String()})
		}
		return work.WorkItem{}, err
	}
	return item, nil
}

// pathOf is the scope chain a collection is judged against: the tenant, the hub above it, and the
// collection itself. A membership held at any of the three applies downwards.
func pathOf(collection work.Container) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !collection.ParentID.IsZero() {
		path = append(path, identity.HubScope(collection.ParentID))
	}
	if collection.Type == work.ContainerHub {
		return append(path, identity.HubScope(collection.ID))
	}
	return append(path, identity.CollectionScope(collection.ID))
}

// Descriptor is the catalogue entry.
func (h PurgeWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: PurgeWorkItemName,
		Summary: "Removes an entry in the trash and everything under it for good, before its " +
			"retention period is up. Irreversible: no restore brings it back, and a backup taken " +
			"before it does not either. Refused for an entry that is not in the trash - delete it " +
			"first, which is reversible - and refused while a legal hold is in force over it.",
		SideEffects: "Removes the rows, writes a deletion journal entry and a tombstone for each, " +
			"announces one purge event per row, and writes an audit entry.",
		TokenScope:  itemsWrite,
		Destructive: true,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry in the trash to remove for good.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemPurgedAction, TargetType: itemTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the history goes with what it describes. `activity_entry` cascades on the item, so " +
				"a purge removes an entry's history in the same statement - there is nothing left " +
				"to write a step to.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h EmptyTrash) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: EmptyTrashName,
		Summary: "Removes everything in the trash for good, in one pass, without waiting out the " +
			"retention period. Irreversible. Anything under a legal hold stays and is counted in " +
			"the answer rather than failing the call. Needs the owner's right to delete a " +
			"container, because a trash holds hubs and collections as readily as entries. Large " +
			"trashes take more than one pass: the answer says how many were matched, and calling " +
			"again continues.",
		SideEffects: "Removes the rows, writes a deletion journal entry and a tombstone for each, " +
			"announces one purge event per entry removed, and writes one audit entry summarising " +
			"the pass.",
		TokenScope:  containersWrite,
		Destructive: true,
		Input:       nil,
		Audit: usecase.AuditDeclaration{
			Action: TrashEmptiedAction, TargetType: trashTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the history goes with what it describes. `activity_entry` cascades on the item, so " +
				"a purge removes an entry's history in the same statement - there is nothing left " +
				"to write a step to.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h PurgeWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	removed, err := h.Execute(ctx, actor, PurgeWorkItemCommand{ItemID: itemID})
	if err != nil {
		return nil, err
	}
	return usecase.Output{"removed": removed}, nil
}

func (h EmptyTrash) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	outcome, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}

	// The blocked counts travel as strings keyed by reason, which is the shape
	// `retention_run.blocked_reasons` records and the shape a client renders: "3 kept by a legal
	// hold" is a sentence a client can build, and a bare total is not.
	blocked := map[string]any{}
	for reason, count := range outcome.Blocked {
		blocked[reason] = count
	}
	return usecase.Output{
		"matched": outcome.Matched, "removed": outcome.Removed, "blocked": blocked,
	}, nil
}
