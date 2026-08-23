// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
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
	ListAttachmentsName = "ListAttachments"

	// itemTarget is what the trail calls an entry. The same word the work package uses, because
	// an auditor filtering on a target type filters across both.
	itemTarget = "item"

	// AttachmentsReadAction is the audit code of an attempted read, declared for the reason
	// MediaReadAction is: an ordinary read writes no entry, a refused one does.
	AttachmentsReadAction audit.Action = "item.attachments_read"

	// The page size limits of api-guidelines.md §4. Repeated here rather than imported from the
	// work package, for the reason containerPath is: the two are siblings in the application layer
	// and neither is the other's dependency. The numbers are the contract's, and the contract is
	// what keeps them the same.
	defaultPageSize = 50
	maxPageSize     = 200
)

// ListAttachments reads what an entry carries, as media objects.
//
// In the media package rather than beside AttachMedia, because what it answers is a page of media
// records: the projection is the one GetMedia answers with, and a second copy of it in the work
// package is a second place for the contract's MediaObject schema to drift. What it borrows from
// the other side is only the permission question, which is the entry's - whoever may read the
// entry may read the list of what is on it.
type ListAttachments struct {
	Objects    repository.Objects
	Items      workrepo.Items
	Containers workrepo.Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// AttachmentsQuery is the input, typed.
type AttachmentsQuery struct {
	ItemID shared.ID
	Cursor string
	Size   int
}

// Execute returns one page of the entry's attachments.
//
// No download target per row, deliberately. A page of twenty attachments would mean twenty
// capabilities minted for bytes the caller may not be about to fetch, each one valid for as long as
// the window lasts; a client that wants one reads it through GetMedia, which is one request for one
// capability (T-11).
func (h ListAttachments) Execute(
	ctx context.Context, actor appshared.ActorContext, query AttachmentsQuery,
) (repository.ObjectPage, error) {
	if query.ItemID.IsZero() {
		return repository.ObjectPage{}, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}

	// The entry and its collection are read before the permission question, because the answer
	// depends on the path: a membership held at the hub applies downwards (domain-model.md §3.2).
	item, collection, err := h.scopeOf(ctx, actor, query.ItemID)
	if err != nil {
		return repository.ObjectPage{}, err
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(collection),
		Action:     AttachmentsReadAction,
		TokenScope: mediaRead,
		TargetType: itemTarget,
		TargetID:   item.ID,
		On: access.ItemSubject{
			Does: service.ItemRead, ID: item.ID, Assignee: item.AssigneeID,
		},
	}); err != nil {
		return repository.ObjectPage{}, err
	}

	var page repository.ObjectPage
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		page, err = h.Objects.ListForItem(ctx, query.ItemID, workrepo.Page{
			Cursor: query.Cursor, Size: pageSize(query.Size),
		})
		return err
	})
	if err != nil {
		return repository.ObjectPage{}, err
	}
	return page, nil
}

// scopeOf reads the entry and its collection, read-only and outside any write transaction, because
// the permission check needs both first (multi-tenancy.md §7).
func (h ListAttachments) scopeOf(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) (domain.WorkItem, domain.Container, error) {
	var (
		item       domain.WorkItem
		collection domain.Container
	)

	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Items.Find(ctx, itemID)
		if err != nil {
			return err
		}
		item = found

		collection, err = h.Containers.Find(ctx, found.CollectionID)
		if err != nil && errors.Is(err, shared.ErrNotFound) {
			// The entry's collection is gone while the entry is not. A tenant-scoped foreign key
			// makes this unreachable (ADR-0024), so it is a defect rather than a 404 for an entry
			// that does exist.
			return shared.ErrInternal.WithDetail("items.collection_missing").WithCause(err)
		}
		return err
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// The same answer the authorisation service gives for an entry out of reach: two
			// spellings of "not found" would be an oracle for which identifiers exist (T-04).
			return domain.WorkItem{}, domain.Container{}, appshared.ItemNotFound(itemID)
		}
		return domain.WorkItem{}, domain.Container{}, err
	}
	return item, collection, nil
}

// pageSize clamps a requested size into the contract's range. Bulk export goes through an :export
// job instead, which is why this is a clamp rather than a refusal: a client asking for 500 rows
// wants as many as it can have, and 200 of them is a better answer than an error.
func pageSize(requested int) int {
	switch {
	case requested < 1:
		return defaultPageSize
	case requested > maxPageSize:
		return maxPageSize
	default:
		return requested
	}
}

// pageOutput is the response shape of every paged read: `{ "data": [...], "page": {...} }`
// (api-guidelines.md §4).
func pageOutput(data []usecase.Output, info workrepo.PageInfo) usecase.Output {
	page := map[string]any{"next_cursor": nil, "has_more": info.HasMore}
	if info.NextCursor != "" {
		page["next_cursor"] = info.NextCursor
	}
	return usecase.Output{"data": data, "page": page}
}

// Descriptor is the catalogue entry.
func (h ListAttachments) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListAttachmentsName,
		Summary: "Reads an entry's attachments as media objects, oldest upload first. Readable by " +
			"whoever may read the entry. No download target per row - a client that wants the " +
			"bytes of one reads it through GetMedia, which mints one capability rather than a " +
			"page of them.",
		SideEffects: "None. Reads only.",
		TokenScope:  mediaRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry whose attachments are wanted.",
			},
			{
				Name: "cursor", Kind: usecase.KindString,
				Description: "The opaque cursor of the previous page. Omitted starts at the " +
					"oldest upload.",
			},
			{
				Name: "size", Kind: usecase.KindInt,
				Description: "How many attachments to return. Clamped to the contract's maximum.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: AttachmentsReadAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A read writes no history. What the entry carries is recorded when it is " +
				"attached and when it is detached, and an entry for somebody having looked at the " +
				"list would be a history of readers rather than of the work.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListAttachments) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}

	page, err := h.Execute(ctx, actor, AttachmentsQuery{
		ItemID: itemID, Cursor: in.String("cursor"), Size: in.Int("size"),
	})
	if err != nil {
		return nil, err
	}

	objects := make([]usecase.Output, 0, len(page.Objects))
	for _, object := range page.Objects {
		objects = append(objects, mediaOutput(object))
	}
	return pageOutput(objects, page.Info), nil
}
