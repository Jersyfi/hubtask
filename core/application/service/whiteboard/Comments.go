// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"context"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The use case names (UC-CNV-09).
const (
	AddWhiteboardCommentName    = "AddWhiteboardComment"
	ListWhiteboardCommentsName  = "ListWhiteboardComments"
	ChangeWhiteboardCommentName = "ChangeWhiteboardComment"
	DeleteWhiteboardCommentName = "DeleteWhiteboardComment"

	maxCommentsListed = 1000
	commentTarget     = "whiteboard_comment"
)

// The audit codes of a board's comments. Their words go to the trail as fingerprints (rule 10).
const (
	BoardCommentAddedAction   audit.Action = "whiteboard_comment.added"
	BoardCommentChangedAction audit.Action = "whiteboard_comment.changed"
	BoardCommentDeletedAction audit.Action = "whiteboard_comment.deleted"
)

// People answers whether another person reads a place - who a comment may tell.
type People interface {
	CanSee(ctx context.Context, actor appshared.ActorContext, accountID shared.ID, path []identity.Scope) (bool, error)
}

// MentionRecorder tells the people a comment named, inside the comment's transaction.
type MentionRecorder interface {
	Mentioned(ctx context.Context, tenant, board, actor shared.ID, people []shared.ID, at time.Time) error
}

// CommentOutput is a comment as every door returns it (schema WhiteboardComment). A comment taken
// back answers no words.
func CommentOutput(c domain.Comment) usecase.Output {
	out := usecase.Output{
		"id": c.ID.String(), "whiteboard_id": c.BoardID.String(), "parent_id": id(c.ParentID),
		"author_id": id(c.AuthorID), "body": c.Body, "element_id": nilIfEmpty(c.Element),
		"x": c.X, "y": c.Y, "resolved": c.ResolvedAt != nil, "resolved_at": timeOrNil(c.ResolvedAt),
		"resolved_by": id(c.ResolvedBy), "created_at": c.CreatedAt, "edited_at": timeOrNil(c.EditedAt),
		"deleted": c.IsDeleted(), "version": c.Version,
	}
	if c.IsDeleted() {
		out["body"] = ""
	}
	return out
}

func id(value shared.ID) any {
	if value.IsZero() {
		return nil
	}
	return value.String()
}

func optionalID(in usecase.Input, field string) (shared.ID, error) {
	if in.String(field) == "" {
		return "", nil
	}
	return in.ID(field)
}

// pinOf reads a thread's place: absent is the board's origin, anything but a number malformed.
func pinOf(in usecase.Input) (x, y float64, err error) {
	for _, member := range []struct {
		name string
		into *float64
	}{{"x", &x}, {"y", &y}} {
		raw, present := in[member.name]
		if !present || raw == nil {
			continue
		}
		value, ok := asNumber(raw)
		if !ok {
			return 0, 0, shared.ErrValidation.WithDetail("whiteboards.comment_pin_malformed").
				WithFields(shared.FieldError{Path: "/" + member.name, Code: "whiteboards.comment_pin_malformed"})
		}
		*member.into = value
	}
	return x, y, nil
}

func timeOrNil(at *time.Time) any {
	if at == nil {
		return nil
	}
	return *at
}

// tell records a notification for each person newly named who reads the board's hub; the others
// are passed over without a word, so a comment cannot learn who exists outside the board (T-04).
func (s Studio) tell(ctx context.Context, actor appshared.ActorContext, board domain.Board, people []shared.ID, at time.Time) error {
	if s.Mentions == nil || s.People == nil || len(people) == 0 {
		return nil
	}
	readers := make([]shared.ID, 0, len(people))
	for _, person := range people {
		if person == actor.AccountID {
			continue
		}
		sees, err := s.People.CanSee(ctx, actor, person, hubPath(board.HubID))
		if err != nil {
			return err
		}
		if sees {
			readers = append(readers, person)
		}
	}
	if len(readers) == 0 {
		return nil
	}
	return s.Mentions.Mentioned(ctx, actor.TenantID, board.ID, actor.AccountID, readers, at)
}

func (s Studio) recordComment(ctx context.Context, actor appshared.ActorContext, action audit.Action, id shared.ID, changes []audit.Change, at time.Time) error {
	return s.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: at, Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: commentTarget, TargetID: id,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// AddWhiteboardComment writes a thread pinned to an object or a place, or a reply in a thread, and
// tells the people it names.
type AddWhiteboardComment struct{ Studio Studio }

// AddWhiteboardCommentCommand is the input, typed.
type AddWhiteboardCommentCommand struct {
	BoardID, ParentID shared.ID
	Body, Element     string
	X, Y              float64
}

// Execute writes the comment.
func (h AddWhiteboardComment) Execute(ctx context.Context, actor appshared.ActorContext, cmd AddWhiteboardCommentCommand) (domain.Comment, error) {
	if err := actor.RequireScope(boardsWrite); err != nil {
		return domain.Comment{}, err
	}
	board, err := h.Studio.placeOf(ctx, actor, cmd.BoardID, false)
	if err != nil {
		return domain.Comment{}, err
	}
	if err := h.Studio.authorizeWrite(ctx, actor, board.HubID, board.ID, BoardCommentAddedAction); err != nil {
		return domain.Comment{}, err
	}
	var comment domain.Comment
	err = h.Studio.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var parent *domain.Comment
		if !cmd.ParentID.IsZero() {
			found, err := h.Studio.Boards.FindComment(ctx, cmd.ParentID)
			if err != nil {
				return err
			}
			parent = &found
		}
		now := h.Studio.Clock.Now()
		var err error
		comment, err = domain.NewComment(domain.NewCommentInput{
			ID: h.Studio.IDs.NewID(), TenantID: actor.TenantID, BoardID: board.ID, AuthorID: actor.AccountID,
			Parent: parent, Body: cmd.Body, Element: cmd.Element, X: cmd.X, Y: cmd.Y, Now: now, Text: h.Studio.Text,
		})
		if err != nil {
			return err
		}
		if err := h.Studio.Boards.InsertComment(ctx, comment); err != nil {
			return err
		}
		if err := h.Studio.tell(ctx, actor, board, domain.Mentions(comment.Body), now); err != nil {
			return err
		}
		return h.Studio.recordComment(ctx, actor, BoardCommentAddedAction, comment.ID, []audit.Change{
			{Field: "whiteboard_id", Classification: audit.Open, To: board.ID.String()},
			{Field: "body", Classification: audit.Sensitive, To: comment.Body},
		}, now)
	})
	return comment, err
}

// Descriptor is the catalogue entry.
func (h AddWhiteboardComment) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: AddWhiteboardCommentName, Permission: service.PermissionWriteItems,
		Summary: "Writes a comment on a whiteboard: a thread pinned at a place (`x`, `y`) and, when it is " +
			"about an object, at that object (`element_id`), or a reply to a thread's first comment. A " +
			"person named as `[@Name](hubtask:person/<id>)` who reads the board is told.",
		SideEffects: "Writes the comment, a notification to each person it names who reads the board, and an audit entry.",
		TokenScope:  boardsWrite,
		Input: []usecase.Field{
			{Name: "whiteboard_id", Kind: usecase.KindID, Required: true, Description: "The board."},
			{Name: "body", Kind: usecase.KindString, Required: true, Description: "The comment in Markdown."},
			{Name: "element_id", Kind: usecase.KindString, Description: "The object the thread is about."},
			{Name: "x", Kind: usecase.KindAny, Description: "Where the thread is pinned on the board."},
			{Name: "y", Kind: usecase.KindAny, Description: "Where the thread is pinned on the board."},
			{Name: "parent_id", Kind: usecase.KindID, Description: "The thread's first comment, for a reply."},
		},
		Audit: usecase.AuditDeclaration{Action: BoardCommentAddedAction, TargetType: commentTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			board, err := in.ID("whiteboard_id")
			if err != nil {
				return nil, err
			}
			parent, err := optionalID(in, "parent_id")
			if err != nil {
				return nil, err
			}
			x, y, err := pinOf(in)
			if err != nil {
				return nil, err
			}
			comment, err := h.Execute(ctx, actor, AddWhiteboardCommentCommand{
				BoardID: board, ParentID: parent, Body: in.String("body"), Element: in.String("element_id"), X: x, Y: y,
			})
			if err != nil {
				return nil, err
			}
			return CommentOutput(comment), nil
		}),
	})
}

// ListWhiteboardComments reads a board's comments, oldest first.
type ListWhiteboardComments struct{ Studio Studio }

// Execute reads the comments.
func (h ListWhiteboardComments) Execute(ctx context.Context, actor appshared.ActorContext, boardID shared.ID) ([]domain.Comment, error) {
	board, err := h.Studio.readable(ctx, actor, boardID, BoardReadAction)
	if err != nil {
		return nil, err
	}
	var comments []domain.Comment
	err = h.Studio.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		comments, err = h.Studio.Boards.Comments(ctx, board.ID, maxCommentsListed)
		return err
	})
	return comments, err
}

// Descriptor is the catalogue entry.
func (h ListWhiteboardComments) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListWhiteboardCommentsName, Permission: service.PermissionRead,
		Summary: "Reads a whiteboard's comments oldest first: threads with the object and place they are " +
			"pinned to, their replies, resolved ones marked. A comment taken back answers no words.",
		SideEffects: "None. Reads only.",
		TokenScope:  boardsRead,
		ReadOnly:    true,
		Input:       []usecase.Field{{Name: "whiteboard_id", Kind: usecase.KindID, Required: true, Description: "The board."}},
		Audit:       usecase.AuditDeclaration{Action: BoardReadAction, TargetType: boardTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			board, err := in.ID("whiteboard_id")
			if err != nil {
				return nil, err
			}
			comments, err := h.Execute(ctx, actor, board)
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

// ChangeWhiteboardCommentCommand is the input, typed. A nil member is left alone.
type ChangeWhiteboardCommentCommand struct {
	CommentID       shared.ID
	Body            *string
	Resolved        *bool
	Delete          bool
	ExpectedVersion int
}

// ChangeWhiteboardComment rewrites a comment's words (its author only, telling the people the new
// words add), resolves or reopens a thread (whoever may draw on the board), or takes a comment back
// (its author only).
type ChangeWhiteboardComment struct{ Studio Studio }

// Execute changes the comment.
func (h ChangeWhiteboardComment) Execute(ctx context.Context, actor appshared.ActorContext, cmd ChangeWhiteboardCommentCommand) (domain.Comment, error) {
	if err := actor.RequireScope(boardsWrite); err != nil {
		return domain.Comment{}, err
	}
	action := BoardCommentChangedAction
	if cmd.Delete {
		action = BoardCommentDeletedAction
	}
	var current domain.Comment
	var board domain.Board
	err := h.Studio.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if current, err = h.Studio.Boards.FindComment(ctx, cmd.CommentID); err != nil {
			return err
		}
		board, err = h.Studio.find(ctx, current.BoardID, false, false)
		return err
	})
	if err != nil {
		return domain.Comment{}, err
	}
	if err := h.Studio.authorizeWrite(ctx, actor, board.HubID, current.ID, action); err != nil {
		return domain.Comment{}, err
	}
	if current.IsDeleted() {
		return domain.Comment{}, shared.ErrNotFound.WithDetail("whiteboards.comment_not_found")
	}
	if (cmd.Body != nil || cmd.Delete) && current.AuthorID != actor.AccountID {
		return domain.Comment{}, shared.ErrForbidden.WithDetail("whiteboards.comment_not_own")
	}
	if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
		return domain.Comment{}, shared.ErrVersionConflict.WithDetail("whiteboards.comment_version_conflict")
	}
	now := h.Studio.Clock.Now()
	next := current
	var changes []audit.Change
	switch {
	case cmd.Delete:
		at := now
		next.Body, next.DeletedAt = "", &at
		changes = append(changes, audit.Change{Field: "deleted", Classification: audit.Open, To: true})
	default:
		if cmd.Body != nil {
			body, err := domain.CheckCommentBody(*cmd.Body, h.Studio.Text)
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
	err = h.Studio.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Studio.Boards.UpdateComment(ctx, next, current.Version); err != nil {
			return err
		}
		next.Version = current.Version + 1
		if cmd.Body != nil {
			if err := h.Studio.tell(ctx, actor, board, domain.NewlyMentioned(current.Body, next.Body), now); err != nil {
				return err
			}
		}
		return h.Studio.recordComment(ctx, actor, action, current.ID, changes, now)
	})
	return next, err
}

// Descriptor is the catalogue entry.
func (h ChangeWhiteboardComment) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ChangeWhiteboardCommentName, Permission: service.PermissionWriteItems,
		Summary: "Rewrites a whiteboard comment's words - its author only (whiteboards.comment_not_own), " +
			"telling the people the new words name - or resolves or reopens its thread, which whoever " +
			"may draw on the board may do. Only a thread's first comment resolves.",
		SideEffects: "Writes the comment, a notification to each person newly named, and an audit entry.",
		TokenScope:  boardsWrite,
		Input: []usecase.Field{
			{Name: "comment_id", Kind: usecase.KindID, Required: true, Description: "The comment."},
			{Name: "body", Kind: usecase.KindString, Description: "The new words."},
			{Name: "resolved", Kind: usecase.KindBool, Description: "Resolve (true) or reopen (false) the thread."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read."},
		},
		Audit: usecase.AuditDeclaration{Action: BoardCommentChangedAction, TargetType: commentTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			commentID, err := in.ID("comment_id")
			if err != nil {
				return nil, err
			}
			cmd := ChangeWhiteboardCommentCommand{CommentID: commentID, ExpectedVersion: in.Int("expected_version")}
			if raw, ok := in["body"].(string); ok {
				cmd.Body = &raw
			}
			if raw, ok := in["resolved"].(bool); ok {
				cmd.Resolved = &raw
			}
			comment, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			return CommentOutput(comment), nil
		}),
	})
}

// DeleteWhiteboardComment takes one's own comment back: the thread reads on, the words go.
type DeleteWhiteboardComment struct{ Studio Studio }

// Descriptor is the catalogue entry.
func (h DeleteWhiteboardComment) Descriptor() usecase.Descriptor {
	change := ChangeWhiteboardComment(h)
	return descriptor(usecase.Descriptor{
		Name: DeleteWhiteboardCommentName, Permission: service.PermissionWriteItems,
		Summary:     "Takes one's own whiteboard comment back. Its place in the thread stays, its words go.",
		SideEffects: "Clears the comment's words and writes an audit entry.",
		TokenScope:  boardsWrite,
		Destructive: true,
		Input:       []usecase.Field{{Name: "comment_id", Kind: usecase.KindID, Required: true, Description: "The comment."}},
		Audit:       usecase.AuditDeclaration{Action: BoardCommentDeletedAction, TargetType: commentTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			commentID, err := in.ID("comment_id")
			if err != nil {
				return nil, err
			}
			if _, err := change.Execute(ctx, actor, ChangeWhiteboardCommentCommand{CommentID: commentID, Delete: true}); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	})
}
