// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	activityrepo "github.com/Jersyfi/hubtask/core/application/repository/activity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ListActivityName = "ListActivity"

	// ActivityReadAction is the audit code of an attempted read, declared for the reason
	// ItemReadAction is: an ordinary read writes no entry, a refused one does. Its own code rather
	// than the entry's, because "who tried to read the history of this" is a question an auditor
	// asks separately from "who tried to read this" (audit.md §4).
	ActivityReadAction audit.Action = "item.activity_read"
)

// ListActivity reads one entry's history, newest first.
//
// The right to read the entry is the right to read what happened to it. There is no narrower
// permission and there should not be: the history says who moved a thing somebody can already see,
// and a separate right would be one more thing to get wrong for no protection gained
// (domain-model.md §3.2).
//
// Read-only throughout, which is not a detail: the transaction may be served by a read replica
// (multi-tenancy.md §7), and a read that opened a write transaction would pin it to the primary.
type ListActivity struct {
	History    activityrepo.History
	Items      repository.Items
	Containers repository.Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// ListActivityQuery is the input, typed.
type ListActivityQuery struct {
	ItemID shared.ID
	Cursor string
	Size   int
}

// Execute returns one page of the entry's history.
//
// Two transactions, and the permission question between them. It has to be between them: a refusal
// writes an audit entry, and an entry written inside a read-only transaction cannot be written at
// all - which would turn "you may not read this" into "the database is unavailable" (audit.md §7).
func (h ListActivity) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListActivityQuery,
) (activityrepo.EntryPage, error) {
	if query.ItemID.IsZero() {
		return activityrepo.EntryPage{}, itemIDRequired()
	}

	collection, err := h.collectionOf(ctx, actor, query.ItemID)
	if err != nil {
		return activityrepo.EntryPage{}, err
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(collection),
		Action:     ActivityReadAction,
		TokenScope: itemsRead,
		TargetType: itemTarget,
		TargetID:   query.ItemID,
	}); err != nil {
		return activityrepo.EntryPage{}, err
	}

	var page activityrepo.EntryPage
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		page, err = h.History.List(ctx, query.ItemID, activityrepo.Page{
			Cursor: query.Cursor, Size: PageSize(query.Size),
		})
		return err
	})
	if err != nil {
		return activityrepo.EntryPage{}, err
	}
	return page, nil
}

// collectionOf reads the collection the entry lives in, which is what the permission question is
// asked against.
//
// Both reads rather than one: the entry names its collection and the collection names its hub, and
// a membership held at either applies downwards - a path that stopped at the collection would
// refuse somebody who holds the right above it (domain-model.md §3.2).
func (h ListActivity) collectionOf(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) (domain.Container, error) {
	var collection domain.Container

	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		item, err := findItem(ctx, h.Items, itemID)
		if err != nil {
			return err
		}
		// A missing collection under an entry that exists is a defect rather than a 404 for the
		// entry that does exist: a tenant-scoped foreign key makes it unreachable (ADR-0024). That
		// distinction is findCollection's, so that this read and every writer's answer the same way.
		collection, err = findCollection(ctx, h.Containers, item.CollectionID)
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	return collection, nil
}

// Descriptor is the catalogue entry.
func (h ListActivity) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListActivityName,
		Summary: "Reads what has happened to a task, work package or activity: who did what, and " +
			"when, newest first. This is the product's history rather than the audit trail - it is " +
			"readable by whoever may read the entry, and it goes when the entry does. Each step " +
			"carries a message code and the fields that moved, never a finished sentence. For an " +
			"activity the history is compact: the code, the actor and the time, without the fields.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry whose history is wanted.",
			},
			{
				Name: "cursor", Kind: usecase.KindString,
				Description: "The opaque cursor of the previous page. Omitted starts at the newest step.",
			},
			{
				Name: "size", Kind: usecase.KindInt,
				Description: "How many steps to return. Clamped to the contract's maximum.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ActivityReadAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListActivity) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}

	page, err := h.Execute(ctx, actor, ListActivityQuery{
		ItemID: itemID, Cursor: in.String("cursor"), Size: in.Int("size"),
	})
	if err != nil {
		return nil, err
	}

	steps := make([]usecase.Output, 0, len(page.Entries))
	for _, entry := range page.Entries {
		steps = append(steps, activityOutput(entry))
	}
	return pageOutput(steps, repository.PageInfo(page.Info)), nil
}

// activityOutput is the projection as every channel renders it.
//
// The code rather than the verb: a client renders `activity.item_completed` out of the catalogue,
// and the stored verb is this system's own spelling of the same thing (ADR-0011, i18n-l10n.md §1).
// No actor label - the account is one request away, and a copy of somebody's name in a record that
// is deleted with its entry would have nothing to outlive.
func activityOutput(entry activity.Entry) usecase.Output {
	changeSet := entry.ChangeSet
	if changeSet == nil {
		// An empty object rather than null. A compact step and a step that moved no field both have
		// one, and a client should not have to tell them from a missing member.
		changeSet = map[string]any{}
	}

	return usecase.Output{
		"id":      entry.ID.String(),
		"item_id": entry.ItemID.String(),
		"code":    entry.Verb.MessageCode(),
		"actor": map[string]any{
			"type": string(entry.Actor.Kind),
			"id":   idOrNil(entry.Actor.ID),
		},
		"occurred_at": entry.OccurredAt,
		"change_set":  changeSet,
	}
}
