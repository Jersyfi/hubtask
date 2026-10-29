// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names.
const (
	AddPageCommentName    = "AddPageComment"
	ListPageCommentsName  = "ListPageComments"
	ChangePageCommentName = "ChangePageComment"
	DeletePageCommentName = "DeletePageComment"

	maxCommentsListed = 1000
)

// CommentOutput is a comment as every door returns it (schema PageComment). A comment taken back
// answers no words.
func CommentOutput(c domain.Comment) usecase.Output {
	out := usecase.Output{
		"id": c.ID.String(), "page_id": c.PageID.String(), "parent_id": idOrNil(c.ParentID),
		"author_id": idOrNil(c.AuthorID), "body": c.Body, "anchor": nilIfEmpty(c.Anchor),
		"quote": nilIfEmpty(c.Quote), "resolved": c.ResolvedAt != nil, "resolved_at": timeOrNil(c.ResolvedAt),
		"resolved_by": idOrNil(c.ResolvedBy), "created_at": c.CreatedAt, "edited_at": timeOrNil(c.EditedAt),
		"deleted": c.IsDeleted(), "version": c.Version,
	}
	if c.IsDeleted() {
		out["body"] = ""
	}
	return out
}

// AddPageComment writes a comment on a page, inline on a passage, or a reply in a thread.
type AddPageComment struct{ Space Space }

// AddPageCommentCommand is the input, typed.
type AddPageCommentCommand struct {
	PageID, ParentID    shared.ID
	Body, Anchor, Quote string
}

// Execute writes the comment.
func (h AddPageComment) Execute(ctx context.Context, actor appshared.ActorContext, cmd AddPageCommentCommand) (domain.Comment, error) {
	place, err := h.Space.placeOf(ctx, actor, cmd.PageID)
	if err != nil {
		return domain.Comment{}, err
	}
	if err := h.Space.authorizeWrite(ctx, actor, place.HubID, place.ID, PageCommentAddedAction, commentTarget); err != nil {
		return domain.Comment{}, err
	}
	var comment domain.Comment
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var parent *domain.Comment
		if !cmd.ParentID.IsZero() {
			found, err := h.Space.Pages.FindComment(ctx, cmd.ParentID)
			if err != nil {
				return err
			}
			parent = &found
		}
		now := h.Space.Clock.Now()
		var err error
		comment, err = domain.NewComment(domain.NewCommentInput{
			ID: h.Space.IDs.NewID(), TenantID: actor.TenantID, PageID: place.ID, AuthorID: actor.AccountID,
			Parent: parent, Body: cmd.Body, Anchor: cmd.Anchor, Quote: cmd.Quote, Now: now, Text: h.Space.Text,
		})
		if err != nil {
			return err
		}
		if err := h.Space.Pages.InsertComment(ctx, comment); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageCommentAddedAction, commentTarget, comment.ID, []audit.Change{
			{Field: "page_id", Classification: audit.Open, To: place.ID.String()},
			{Field: "body", Classification: audit.Sensitive, To: comment.Body},
		}, now)
	})
	return comment, err
}

// Descriptor is the catalogue entry.
func (h AddPageComment) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: AddPageCommentName, Permission: service.PermissionWriteItems,
		Summary: "Writes a comment on a page: on the page as a whole, inline on a block (`anchor`, the " +
			"block's identifier, with the words it is about as `quote`), or as a reply to a thread's " +
			"first comment. One level of replies only.",
		SideEffects: "Writes the comment and an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "body", Kind: usecase.KindString, Required: true, Description: "The comment in Markdown."},
			{Name: "anchor", Kind: usecase.KindString, Description: "The block the comment stands at."},
			{Name: "quote", Kind: usecase.KindString, Description: "The words of the block it is about."},
			{Name: "parent_id", Kind: usecase.KindID, Description: "The thread's first comment, for a reply."},
		},
		Audit: usecase.AuditDeclaration{Action: PageCommentAddedAction, TargetType: commentTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			parent, err := optionalID(in, "parent_id")
			if err != nil {
				return nil, err
			}
			body := ""
			if raw := rawText(in, "body"); raw != nil {
				body = *raw
			}
			comment, err := h.Execute(ctx, actor, AddPageCommentCommand{
				PageID: id, ParentID: parent, Body: body, Anchor: in.String("anchor"), Quote: in.String("quote"),
			})
			if err != nil {
				return nil, err
			}
			return CommentOutput(comment), nil
		}),
	})
}

// ListPageComments reads a page's comments, oldest first.
type ListPageComments struct{ Space Space }

// Execute reads the comments.
func (h ListPageComments) Execute(ctx context.Context, actor appshared.ActorContext, pageID shared.ID) ([]domain.Comment, error) {
	page, err := h.Space.readable(ctx, actor, pageID, PageReadAction)
	if err != nil {
		return nil, err
	}
	var comments []domain.Comment
	err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		comments, err = h.Space.Pages.Comments(ctx, page.ID, maxCommentsListed)
		return err
	})
	return comments, err
}

// Descriptor is the catalogue entry.
func (h ListPageComments) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListPageCommentsName, Permission: service.PermissionRead,
		Summary: "Reads a page's comments oldest first, threads and their replies, inline ones with the " +
			"block and words they are about, resolved ones marked. A comment taken back answers no words.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead,
		ReadOnly:    true,
		Input:       []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."}},
		Audit:       usecase.AuditDeclaration{Action: PageReadAction, TargetType: commentTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			comments, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(comments))
			for _, comment := range comments {
				data = append(data, CommentOutput(comment))
			}
			return usecase.Output{"data": data}, nil
		}),
	})
}

// ChangePageCommentCommand is the input, typed. A nil member is left alone.
type ChangePageCommentCommand struct {
	CommentID       shared.ID
	Body            *string
	Resolved        *bool
	Delete          bool
	ExpectedVersion int
}

// ChangePageComment rewrites a comment's words (its author only), resolves or reopens a thread
// (whoever may write the page), or takes a comment back (its author only).
type ChangePageComment struct{ Space Space }

// Execute changes the comment.
func (h ChangePageComment) Execute(ctx context.Context, actor appshared.ActorContext, cmd ChangePageCommentCommand) (domain.Comment, error) {
	action := PageCommentChangeAction
	if cmd.Delete {
		action = PageCommentDeleteAction
	}
	var current domain.Comment
	var page domain.Page
	err := h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if current, err = h.Space.Pages.FindComment(ctx, cmd.CommentID); err != nil {
			return err
		}
		page, err = h.Space.find(ctx, current.PageID, false)
		return err
	})
	if err != nil {
		return domain.Comment{}, err
	}
	if err := h.Space.authorizeWrite(ctx, actor, page.HubID, current.ID, action, commentTarget); err != nil {
		return domain.Comment{}, err
	}
	if current.IsDeleted() {
		return domain.Comment{}, shared.ErrNotFound.WithDetail("pages.comment_not_found")
	}
	if (cmd.Body != nil || cmd.Delete) && current.AuthorID != actor.AccountID {
		return domain.Comment{}, shared.ErrForbidden.WithDetail("pages.comment_not_own")
	}
	if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
		return domain.Comment{}, shared.ErrVersionConflict.WithDetail("pages.comment_version_conflict")
	}
	now := h.Space.Clock.Now()
	next := current
	changes := []audit.Change{}
	switch {
	case cmd.Delete:
		at := now
		next.Body, next.DeletedAt = "", &at
		changes = append(changes, audit.Change{Field: "deleted", Classification: audit.Open, To: true})
	default:
		if cmd.Body != nil {
			body, err := domain.CheckCommentBody(*cmd.Body, h.Space.Text)
			if err != nil {
				return domain.Comment{}, err
			}
			at := now
			next.Body, next.EditedAt = body, &at
			changes = append(changes, audit.Change{Field: "body", Classification: audit.Sensitive, To: body})
		}
		if cmd.Resolved != nil {
			resolved, err := next.Resolved(*cmd.Resolved, actor.AccountID, now)
			if err != nil {
				return domain.Comment{}, err
			}
			next = resolved
			changes = append(changes, audit.Change{Field: "resolved", Classification: audit.Open, To: *cmd.Resolved})
		}
	}
	if len(changes) == 0 {
		return current, nil
	}
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Space.Pages.UpdateComment(ctx, next, current.Version); err != nil {
			return err
		}
		next.Version = current.Version + 1
		return h.Space.record(ctx, actor, action, commentTarget, current.ID, changes, now)
	})
	return next, err
}

// Descriptor is the catalogue entry.
func (h ChangePageComment) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ChangePageCommentName, Permission: service.PermissionWriteItems,
		Summary: "Rewrites a page comment's words - its author only (pages.comment_not_own) - or resolves " +
			"or reopens its thread, which whoever may write the page may do. Only a thread's first " +
			"comment resolves.",
		SideEffects: "Writes the comment and an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "comment_id", Kind: usecase.KindID, Required: true, Description: "The comment."},
			{Name: "body", Kind: usecase.KindString, Description: "The new words."},
			{Name: "resolved", Kind: usecase.KindBool, Description: "Resolve (true) or reopen (false) the thread."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read."},
		},
		Audit: usecase.AuditDeclaration{Action: PageCommentChangeAction, TargetType: commentTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("comment_id")
			if err != nil {
				return nil, err
			}
			comment, err := h.Execute(ctx, actor, ChangePageCommentCommand{
				CommentID: id, Body: rawText(in, "body"), Resolved: optionalBool(in, "resolved"),
				ExpectedVersion: in.Int("expected_version"),
			})
			if err != nil {
				return nil, err
			}
			return CommentOutput(comment), nil
		}),
	})
}

// DeletePageComment takes one's own comment back: the thread reads on, the words go.
type DeletePageComment struct{ Space Space }

// Descriptor is the catalogue entry.
func (h DeletePageComment) Descriptor() usecase.Descriptor {
	change := ChangePageComment(h)
	return descriptor(usecase.Descriptor{
		Name: DeletePageCommentName, Permission: service.PermissionWriteItems,
		Summary:     "Takes one's own page comment back. Its place in the thread stays, its words go.",
		SideEffects: "Clears the comment's words and writes an audit entry.",
		TokenScope:  pagesWrite,
		Destructive: true,
		Input:       []usecase.Field{{Name: "comment_id", Kind: usecase.KindID, Required: true, Description: "The comment."}},
		Audit:       usecase.AuditDeclaration{Action: PageCommentDeleteAction, TargetType: commentTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("comment_id")
			if err != nil {
				return nil, err
			}
			if _, err := change.Execute(ctx, actor, ChangePageCommentCommand{CommentID: id, Delete: true}); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	})
}
