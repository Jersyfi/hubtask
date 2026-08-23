// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

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
	ListCommentsName = "ListComments"

	// CommentsReadAction is the audit code of an attempted read, declared for the reason
	// ActivityReadAction is: an ordinary read writes no entry, a refused one does.
	CommentsReadAction audit.Action = "comment.list_read"
)

// ListComments reads one entry's discussion, oldest first - a conversation reads top down.
//
// The right to read the entry is the right to read what was said about it, for the reason the
// history draws the same line: a separate right would be one more thing to get wrong for no
// protection gained (domain-model.md §3.2). Tombstones are in the page - that is what keeps a
// thread with deletions readable - and they travel without their body, which is already gone from
// the row rather than hidden by this reader.
type ListComments struct {
	Comments   repository.Comments
	Items      repository.Items
	Containers repository.Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// ListCommentsQuery is the input, typed.
type ListCommentsQuery struct {
	ItemID shared.ID
	Cursor string
	Size   int
}

// Execute returns one page of the entry's discussion.
//
// Two transactions, and the permission question between them, for the reason ListActivity splits
// there: a refusal writes an audit entry, and an entry written inside a read-only transaction
// cannot be written at all (audit.md §7).
func (h ListComments) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListCommentsQuery,
) (repository.CommentPage, error) {
	if query.ItemID.IsZero() {
		return repository.CommentPage{}, itemIDRequired()
	}

	collection, err := h.collectionOf(ctx, actor, query.ItemID)
	if err != nil {
		return repository.CommentPage{}, err
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(collection),
		Action:     CommentsReadAction,
		TokenScope: itemsRead,
		TargetType: itemTarget,
		TargetID:   query.ItemID,
	}); err != nil {
		return repository.CommentPage{}, err
	}

	var page repository.CommentPage
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		page, err = h.Comments.List(ctx, query.ItemID, repository.Page{
			Cursor: query.Cursor, Size: PageSize(query.Size),
		})
		return err
	})
	if err != nil {
		return repository.CommentPage{}, err
	}
	return page, nil
}

// collectionOf reads the collection the entry lives in, which is what the permission question is
// asked against - both levels, because a membership held at either applies downwards.
func (h ListComments) collectionOf(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) (domain.Container, error) {
	var collection domain.Container

	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		item, err := findItem(ctx, h.Items, itemID)
		if err != nil {
			return err
		}
		collection, err = findCollection(ctx, h.Containers, item.CollectionID)
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	return collection, nil
}

// Descriptor is the catalogue entry.
func (h ListComments) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListCommentsName,
		Summary: "Reads an entry's discussion, oldest first. Deleted comments are in it as " +
			"tombstones - identifier, author and timestamps with a null body - so a reply never " +
			"dangles. Readable by whoever may read the entry.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry whose discussion is wanted.",
			},
			{
				Name: "cursor", Kind: usecase.KindString,
				Description: "The opaque cursor of the previous page. Omitted starts at the oldest comment.",
			},
			{
				Name: "size", Kind: usecase.KindInt,
				Description: "How many comments to return. Clamped to the contract's maximum.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: CommentsReadAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListComments) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}

	page, err := h.Execute(ctx, actor, ListCommentsQuery{
		ItemID: itemID, Cursor: in.String("cursor"), Size: in.Int("size"),
	})
	if err != nil {
		return nil, err
	}

	comments := make([]usecase.Output, 0, len(page.Comments))
	for _, comment := range page.Comments {
		comments = append(comments, commentOutput(comment))
	}
	return pageOutput(comments, page.Info), nil
}
