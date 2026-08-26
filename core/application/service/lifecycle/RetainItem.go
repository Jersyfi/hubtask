// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	syncrepo "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	RetainItemName = "RetainItem"

	itemsWriteScope = "items:write"

	// RetainedAction is somebody taking an entry out of a running retention period. A warning
	// rather than an info: it is a decision that keeps data the workspace's own rule said should
	// go, and data-retention.md §5 makes taking an object out part of the procedure rather than an
	// exception to it - which means it has to be visible.
	RetainedAction audit.Action = "lifecycle.retained"
)

// RetainItem takes one entry out of the retention period running against it (data-retention.md §5).
//
// The third of the three ways §5 names - "anyone with permission can take the object out by editing
// it, moving it, or issuing a `:retain` command" - and the only one that says so out loud. The
// other two fall out of the anchor: an entry somebody completes again has a new completion date,
// and one moved into another collection meets that collection's rule.
type RetainItem struct {
	Items      workrepo.Items
	Containers workrepo.Containers
	Marking    repository.Marking
	Authorizer Authorizer
	Audit      audit.Sink
	Changes    syncrepo.ChangeLog
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	HLC        clock.HLCSource
}

// Execute takes the entry out and answers it as it now stands.
func (h RetainItem) Execute(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) (work.WorkItem, error) {
	if itemID.IsZero() {
		return work.WorkItem{}, shared.ErrValidation.WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}

	// The entry and its collection before the permission question, because the answer depends on
	// the path: a membership at the hub applies downwards (domain-model.md §3.2).
	var item work.WorkItem
	var collection work.Container
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if item, err = h.Items.Find(ctx, itemID); err != nil {
			return err
		}
		collection, err = h.Containers.Find(ctx, item.CollectionID)
		return err
	})
	if err != nil {
		return work.WorkItem{}, err
	}

	// Before the transaction: a refusal writes an audit entry, and one written inside this
	// transaction would be rolled back together with the refusal (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       pathOf(collection),
		Action:     RetainedAction,
		TokenScope: itemsWriteScope,
		TargetType: itemTarget,
		TargetID:   itemID,
		// Narrowed like any other write on the entry (C-04): a role that reaches only what is
		// assigned to it does not get to keep somebody else's work out of the workspace's rule.
		On: access.ItemSubject{
			Does: service.ItemChange, ID: item.ID, Assignee: item.AssigneeID,
		},
	}); err != nil {
		return work.WorkItem{}, err
	}

	var retained work.WorkItem
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := h.Clock.Now()

		marking, err := h.Marking.Marking(ctx, itemID)
		if err != nil {
			return err
		}
		if marking.Pending.IsZero() {
			// Nothing announced, so there is nothing to take out. A conflict rather than a quiet
			// success: a client that got 200 for this would show a button that does nothing.
			return shared.ErrConflict.WithDetail(domain.CodeNotMarked).
				WithParams(map[string]string{"item_id": itemID.String()})
		}

		// The rule's claim ends with the marking. Taking an entry out means the rule no longer owns
		// it - the next pass judges it afresh against whatever rule then applies, which is what
		// makes "edit it, move it, or retain it" three ways of doing the same thing.
		if _, err := h.Marking.Clear(ctx, []shared.ID{itemID}, false, now); err != nil {
			return err
		}

		if retained, err = h.Items.Find(ctx, itemID); err != nil {
			return err
		}

		// Offline clients are told, because what changed is a field they show: an entry that still
		// said "goes on the fourteenth" on a device somebody had retained on another is the
		// surprise §6 exists to prevent.
		if err := h.Changes.Record(ctx, syncrepo.Change{
			TenantID: actor.TenantID, Entity: itemEntity, EntityID: itemID,
			Op: syncrepo.Upsert, ContainerID: item.CollectionID, ActorID: actor.AccountID,
			HLC: h.HLC.Next(), Payload: map[string]any{"retention": nil},
		}); err != nil {
			return err
		}

		return h.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now,
			Action: RetainedAction, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: itemTarget, TargetID: itemID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{
					Field: "retention_action", Classification: audit.Open,
					From: string(marking.Action),
				},
				audit.Change{
					Field: "policy_id", Classification: audit.Open, From: marking.Rule.String(),
				},
			),
		})
	})
	if err != nil {
		return work.WorkItem{}, err
	}
	return retained, nil
}

// itemEntity is what the change log calls an entry. The schema's word, because a device matches it
// against what a pull hands back.
const itemEntity = "work_item"

// Descriptor registers taking an entry out in all three channels.
func (h RetainItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RetainItemName,
		Summary: "Takes one entry out of the retention period running against it. The rule stops " +
			"owning the entry, so the next pass judges it afresh against whatever rule then " +
			"applies - which is what makes editing it, moving it and this command three ways of " +
			"doing the same thing. An entry nothing has announced anything about is refused, " +
			"rather than answered as if something had happened.",
		SideEffects: "Clears the announcement, tells offline clients, and writes an audit entry. " +
			"Nothing about the entry itself changes.",
		TokenScope: itemsWriteScope,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "Which entry."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RetainedAction, TargetType: itemTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Nothing about the entry changed. What was taken out is an announcement the " +
				"workspace's rule made about it, and the entry's history is a record of what " +
				"people did to the entry itself - a line saying its retention was cleared would " +
				"be a line about the machinery rather than about the work. The audit trail " +
				"carries it, which is where a decision to keep data belongs.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RetainItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	item, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return workservice.ItemOutput(item), nil
}
