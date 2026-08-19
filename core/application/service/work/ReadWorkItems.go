// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	GetWorkItemName   = "GetWorkItem"
	ListWorkItemsName = "ListWorkItems"
	itemsRead         = "items:read"

	// ItemReadAction is the audit code of an attempted read, declared for the same reason
	// ContainerReadAction is: an ordinary read writes no entry, a refused one does.
	ItemReadAction audit.Action = "item.read"
)

// GetWorkItem reads one task, work package or activity.
type GetWorkItem struct {
	Items      repository.Items
	Containers repository.Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// GetWorkItemQuery is the input, typed.
type GetWorkItemQuery struct {
	ItemID shared.ID
}

// Execute returns the item.
//
// Two reads before the permission question, not one: the item names its collection, and the
// collection names the hub. Both are needed because a membership at either applies downwards, and a
// path that stopped at the collection would refuse somebody who holds the right at the hub
// (domain-model.md §3.2). They are one round trip apart and inside the same read-only transaction.
func (h GetWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, query GetWorkItemQuery,
) (domain.WorkItem, error) {
	var (
		item       domain.WorkItem
		collection domain.Container
	)

	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Items.Find(ctx, query.ItemID)
		if err != nil {
			return err
		}
		item = found

		collection, err = h.Containers.Find(ctx, item.CollectionID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				// The item's collection is gone while the item is not. Not the client's mistake and
				// not something it can act on, so it is an internal error rather than a 404 for the
				// item that does exist - a tenant-scoped foreign key makes this unreachable
				// (ADR-0024), which is exactly why reaching it is a defect worth reporting as one.
				return shared.ErrInternal.WithDetail("items.collection_missing").WithCause(err)
			}
			return err
		}
		return nil
	})
	if err != nil {
		return domain.WorkItem{}, err
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(collection),
		Action:     ItemReadAction,
		TokenScope: itemsRead,
		TargetType: itemTarget,
		TargetID:   item.ID,
	}); err != nil {
		return domain.WorkItem{}, err
	}
	return item, nil
}

// Descriptor is the catalogue entry.
func (h GetWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GetWorkItemName,
		Summary: "Reads one task, work package or activity by its identifier. An archived or " +
			"trashed item is returned as it is, with the timestamp saying so, rather than reported " +
			"as missing.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The item to read.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemReadAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}

	item, err := h.Execute(ctx, actor, GetWorkItemQuery{ItemID: itemID})
	if err != nil {
		return nil, err
	}
	return itemOutput(item), nil
}

// ListWorkItems reads one level of one collection.
type ListWorkItems struct {
	Items      repository.Items
	Containers repository.Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// ListWorkItemsQuery is the input, typed.
type ListWorkItemsQuery struct {
	// CollectionID is required. A list of every item in a tenant is an unindexed scan, and every
	// filter beyond one level is the query DSL's (B-12).
	CollectionID shared.ID
	// ParentID is the item whose children are wanted. Empty means the items directly in the
	// collection.
	ParentID        shared.ID
	IncludeArchived bool
	Cursor          string
	Size            int
}

// Execute returns one page of the level.
//
// One permission check for the whole page, unlike the hub level of ListContainers. This list is
// anchored: the client named a collection, so there is a single path to ask about, and the answer
// covers every row in it - a membership at the collection, or at its hub, applies to everything
// inside. A refusal is therefore a refusal rather than an empty page, which is the honest answer when
// the client named the container it cannot read.
func (h ListWorkItems) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListWorkItemsQuery,
) (repository.ItemPage, error) {
	if query.CollectionID.IsZero() {
		return repository.ItemPage{}, shared.ErrValidation.
			WithDetail("items.collection_id_required").
			WithFields(shared.FieldError{Path: "/collection_id", Code: "items.collection_id_required"})
	}

	// The collection is read before the check, because the path to it is what the check is about.
	var collection domain.Container
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Containers.Find(ctx, query.CollectionID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrNotFound.
					WithDetail("items.collection_not_found").
					WithParams(map[string]string{"collection_id": query.CollectionID.String()}).
					WithFields(shared.FieldError{
						Path: "/collection_id", Code: "items.collection_not_found",
					})
			}
			return err
		}
		collection = found
		return nil
	})
	if err != nil {
		return repository.ItemPage{}, err
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(collection),
		Action:     ItemReadAction,
		TokenScope: itemsRead,
		TargetType: containerTarget,
		TargetID:   collection.ID,
	}); err != nil {
		return repository.ItemPage{}, err
	}

	var page repository.ItemPage
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		page, err = h.Items.List(ctx, repository.ItemQuery{
			CollectionID:    collection.ID,
			ParentID:        query.ParentID,
			IncludeArchived: query.IncludeArchived,
			Page:            repository.Page{Cursor: query.Cursor, Size: PageSize(query.Size)},
		})
		return err
	})
	if err != nil {
		return repository.ItemPage{}, err
	}
	return page, nil
}

// Descriptor is the catalogue entry.
func (h ListWorkItems) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListWorkItemsName,
		Summary: "Lists one level of a collection in its manual order: the items directly in the " +
			"collection when no parent is given, that item's children when one is. A whole subtree, " +
			"and every other filter, is what the query operation is for. Paged with an opaque cursor.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "collection_id", Kind: usecase.KindID, Required: true,
				Description: "The collection the level belongs to.",
			},
			{
				Name: "parent_id", Kind: usecase.KindID,
				Description: "The item whose children are wanted. Omitted for the items directly in " +
					"the collection.",
			},
			{
				Name: "include_archived", Kind: usecase.KindBool,
				Description: "Keeps archived items in the page. Trashed ones are never in it.",
			},
			{
				Name: "cursor", Kind: usecase.KindString,
				Description: "The next_cursor of the previous page. Opaque: it is produced by this " +
					"server and is not to be constructed or parsed.",
			},
			{
				Name: "size", Kind: usecase.KindInt,
				Description: "Rows per page, 1 to 200. Defaults to 50.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemReadAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListWorkItems) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	collectionID, err := in.ID("collection_id")
	if err != nil {
		return nil, err
	}
	parentID, err := in.ID("parent_id")
	if err != nil {
		return nil, err
	}

	page, err := h.Execute(ctx, actor, ListWorkItemsQuery{
		CollectionID:    collectionID,
		ParentID:        parentID,
		IncludeArchived: in.Bool("include_archived"),
		Cursor:          in.String("cursor"),
		Size:            in.Int("size"),
	})
	if err != nil {
		return nil, err
	}

	data := make([]usecase.Output, 0, len(page.Items))
	for _, item := range page.Items {
		data = append(data, itemOutput(item))
	}
	return pageOutput(data, page.Info), nil
}
