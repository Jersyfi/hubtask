// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	EditCommentName   = "EditComment"
	DeleteCommentName = "DeleteComment"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	CommentUpdatedAction audit.Action = "comment.updated"
	CommentDeletedAction audit.Action = "comment.deleted"
)

// Moderation is the slice of the authorisation service the comment changes need beyond
// Authorizer: the actor's own effective role, for the rule the permission matrix cannot express -
// only the author or an administrator may change a comment (domain-model.md §3.5).
type Moderation interface {
	RoleAlong(
		ctx context.Context, actor appshared.ActorContext, path []identity.Scope,
	) (identity.Role, bool, error)
}

// EditComment rewrites a comment's text.
type EditComment struct {
	Writer CommentWriter
}

// DeleteComment turns a comment into its tombstone.
type DeleteComment struct {
	Writer CommentWriter
}

// ChangeCommentCommand is the input of both, typed. The body is the edit's; a deletion carries
// none, which is the one field the two do not share.
type ChangeCommentCommand struct {
	ItemID    shared.ID
	CommentID shared.ID
	Body      string
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read
	// none and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// Execute rewrites the text and returns the comment.
func (h EditComment) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ChangeCommentCommand,
) (domain.Comment, error) {
	return h.Writer.change(ctx, actor, cmd, editing)
}

// Execute writes the tombstone and returns it.
func (h DeleteComment) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ChangeCommentCommand,
) (domain.Comment, error) {
	return h.Writer.change(ctx, actor, cmd, deleting)
}

// commentChange is which change the caller asked for. Not a boolean, for the reason the other
// direction types are not: this is the parameter that decides which of two audit trails is
// written.
type commentChange bool

const (
	editing  commentChange = true
	deleting commentChange = false
)

func (c commentChange) action() audit.Action {
	if c == editing {
		return CommentUpdatedAction
	}
	return CommentDeletedAction
}

// change is the whole of both use cases.
func (w CommentWriter) change(
	ctx context.Context, actor appshared.ActorContext, cmd ChangeCommentCommand,
	want commentChange,
) (domain.Comment, error) {
	if cmd.ItemID.IsZero() {
		return domain.Comment{}, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}
	if cmd.CommentID.IsZero() {
		return domain.Comment{}, shared.ErrValidation.
			WithDetail("comments.comment_id_required").
			WithFields(shared.FieldError{Path: "/comment_id", Code: "comments.comment_id_required"})
	}

	// The comment, its entry and the collection are read before the permission question, because
	// the answer depends on the path (domain-model.md §3.2). Nothing read here is trusted
	// afterwards - the state that decides the write is read again inside the transaction.
	comment, subject, collection, err := w.readCommentScope(ctx, actor, cmd)
	if err != nil {
		return domain.Comment{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     want.action(),
		TokenScope: commentsWrite,
		TargetType: commentTarget,
		TargetID:   cmd.CommentID,
		On:         commenting(subject),
	}); err != nil {
		return domain.Comment{}, err
	}

	// The narrowing the permission cannot express (§3.5): only the author or an administrator.
	// Refused with the same code and status as any other missing permission - a third party
	// learns that they may not, not why - and recorded in the trail exactly as a permission
	// refusal is, because "who tried to change somebody else's words" is a question an auditor
	// asks (audit.md §4).
	if err := w.ensureAuthorOrAdministrator(ctx, actor, comment, collection, want); err != nil {
		return domain.Comment{}, err
	}

	var changed domain.Comment
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		current, err := w.findComment(ctx, cmd.CommentID)
		if err != nil {
			return err
		}
		item, err := findItem(ctx, w.Items, current.ItemID)
		if err != nil {
			return err
		}
		collection, err := findCollection(ctx, w.Containers, item.CollectionID)
		if err != nil {
			return err
		}
		// I-C3, I-W4: an archived or trashed entry is read-only, and its discussion inherits
		// that - for the deletion too, which is also a write. Removal for good has its own path
		// (the purge), and it does not run through here.
		if err := collection.EnsureAcceptsItems(); err != nil {
			return err
		}
		if err := item.EnsureEditable(); err != nil {
			return err
		}

		if want == deleting && current.DeletedAt != nil {
			// Already a tombstone. Nothing is written, no version is spent and nothing is
			// announced - the deletion is idempotent. The If-Match is still honoured: a caller
			// reasoning about a version that has moved on is told so.
			if err := ensureCommentVersion(current, cmd.ExpectedVersion); err != nil {
				return err
			}
			changed = current
			return nil
		}

		wanted, err := w.applyChange(current, cmd, want, now)
		if err != nil {
			return err
		}

		expected := cmd.ExpectedVersion
		if expected == 0 {
			// The caller read no version and accepts whatever is there. Not the same as skipping
			// the check: a concurrent write between the read and here is still caught.
			expected = current.Version
		}
		if err := w.store(ctx, wanted, expected, want); err != nil {
			return err
		}
		wanted.Version = expected + 1

		if err := w.announceChange(ctx, actor, wanted, item, want, now); err != nil {
			return err
		}
		changed = wanted
		return nil
	})
	if err != nil {
		return domain.Comment{}, err
	}
	return changed, nil
}

// applyChange asks the domain for the new state.
func (w CommentWriter) applyChange(
	current domain.Comment, cmd ChangeCommentCommand, want commentChange, now time.Time,
) (domain.Comment, error) {
	if want == editing {
		return current.Edited(cmd.Body, now)
	}
	return current.Removed(now), nil
}

// store writes the one column set the change owns.
func (w CommentWriter) store(
	ctx context.Context, comment domain.Comment, expected int, want commentChange,
) error {
	if want == editing {
		return w.Comments.SetBody(ctx, comment, expected)
	}
	return w.Comments.SetDeleted(ctx, comment, expected)
}

// announceChange records what the change owes: the event outwards, the change log entry for
// offline clients, and the audit entry - all inside the caller's transaction (test AT-5).
func (w CommentWriter) announceChange(
	ctx context.Context, actor appshared.ActorContext, comment domain.Comment,
	item domain.WorkItem, want commentChange, now time.Time,
) error {
	by := event.Actor{Kind: actor.Kind, ID: actor.AccountID}

	announcement, err := event.NewCommentUpdated(
		w.IDs.NewID(), comment, item.CollectionID, by, now, event.Cause{})
	if want == deleting {
		announcement, err = event.NewCommentDeleted(
			w.IDs.NewID(), comment, item.CollectionID, by, now, event.Cause{})
	}
	if err != nil {
		return err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return err
	}
	if err := w.recordChange(ctx, comment, item, actor, announcement.Payload); err != nil {
		return err
	}
	return w.recordAudit(ctx, comment, actor, want.action(), now)
}

// ensureAuthorOrAdministrator is the author-or-administrator rule, enforced where it is named.
func (w CommentWriter) ensureAuthorOrAdministrator(
	ctx context.Context, actor appshared.ActorContext, comment domain.Comment,
	collection domain.Container, want commentChange,
) error {
	if actor.AccountID == comment.AuthorID {
		return nil
	}

	role, found, err := w.Moderation.RoleAlong(ctx, actor, containerPath(collection))
	if err != nil {
		return err
	}
	if found && role.AtLeast(identity.RoleAdmin) {
		return nil
	}

	w.recordModerationRefusal(ctx, actor, comment, want)
	return shared.ErrForbidden.
		WithDetail("access.not_permitted").
		WithParams(map[string]string{"permission": string(service.PermissionWriteItems)})
}

// recordModerationRefusal writes the DENIED entry, in a transaction of its own for the reason the
// authorisation service writes its refusals that way: the refusal stands whether or not the entry
// could be written, and a gap in the trail is an operational problem rather than a different
// answer for the client.
func (w CommentWriter) recordModerationRefusal(
	ctx context.Context, actor appshared.ActorContext, comment domain.Comment, want commentChange,
) {
	entry := audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: w.Clock.Now(),
		Action:     want.action(),
		Outcome:    audit.OutcomeDenied,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: commentTarget,
		TargetID:   comment.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "denied_by", Classification: audit.Open, To: "moderation"},
			audit.Change{Field: "author_id", Classification: audit.Open, To: comment.AuthorID.String()},
		),
	}

	if err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return w.Audit.Append(ctx, entry)
	}); err != nil {
		slog.ErrorContext(ctx, "recording a denied moderation failed",
			slog.String("action", string(want.action())),
			slog.String("error", err.Error()))
	}
}

// findComment reads one comment with its not-found answer spelled for this resource.
func (w CommentWriter) findComment(ctx context.Context, id shared.ID) (domain.Comment, error) {
	comment, err := w.Comments.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.Comment{}, shared.ErrNotFound.
				WithDetail("comments.not_found").
				WithParams(map[string]string{"comment_id": id.String()})
		}
		return domain.Comment{}, err
	}
	return comment, nil
}

// readCommentScope reads the comment and the collection its entry lives in, outside the write
// transaction, because the permission question needs the path first. The route names the entry as
// well as the comment, and the two have to agree - a comment reached through somebody else's
// entry would be a second address for the same resource.
func (w CommentWriter) readCommentScope(
	ctx context.Context, actor appshared.ActorContext, cmd ChangeCommentCommand,
) (domain.Comment, domain.WorkItem, domain.Container, error) {
	var comment domain.Comment
	var item domain.WorkItem
	var collection domain.Container

	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.findComment(ctx, cmd.CommentID)
		if err != nil {
			return err
		}
		if found.ItemID != cmd.ItemID {
			// The same answer as a comment that does not exist: confirming that the identifier
			// exists under another entry would be existence disclosure across whatever boundary
			// kept the caller from reading it (T-04).
			return shared.ErrNotFound.
				WithDetail("comments.not_found").
				WithParams(map[string]string{"comment_id": cmd.CommentID.String()})
		}
		comment = found

		item, err = findItem(ctx, w.Items, found.ItemID)
		if err != nil {
			return err
		}
		collection, err = findCollection(ctx, w.Containers, item.CollectionID)
		return err
	})
	if err != nil {
		return domain.Comment{}, domain.WorkItem{}, domain.Container{}, err
	}
	return comment, item, collection, nil
}

// ensureCommentVersion honours the If-Match on a write that turned out to be a no-op.
func ensureCommentVersion(comment domain.Comment, expected int) error {
	if expected == 0 || expected == comment.Version {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("comments.version_conflict").
		WithParams(map[string]string{"comment_id": comment.ID.String()})
}

// Descriptor is the catalogue entry.
func (h EditComment) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: EditCommentName,
		Summary: "Rewrites a comment's text. Only its author or an administrator may; anybody " +
			"else is refused exactly as any other missing permission is. The displaced text is " +
			"not preserved, and a deleted comment cannot be edited back into existence.",
		SideEffects: "Writes the body, announces " + string(event.CommentUpdated) +
			", records the change for offline clients, and writes an audit entry.",
		TokenScope: commentsWrite,
		Input: append([]usecase.Field{{
			Name: "body", Kind: usecase.KindString, Required: true,
			Description: "The new text, up to 20000 characters.",
		}}, commentChangeInput("The comment to edit.")...),
		Audit: usecase.AuditDeclaration{
			Action: CommentUpdatedAction, TargetType: commentTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the comment carries its own edited_at and the thread is where an edit is " +
				"read; a history entry beside it would describe the same fact in a second place, " +
				"and the history's one comment verb is the arrival of a comment (item.commented).",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h DeleteComment) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteCommentName,
		Summary: "Removes a comment's text and marks it deleted, keeping the thread readable: " +
			"the tombstone answers with its identifier, author and timestamps, so a reply does " +
			"not dangle. Only the author or an administrator may. Idempotent: deleting a deleted " +
			"comment succeeds and announces nothing.",
		SideEffects: "Clears the body, marks the comment deleted, announces " +
			string(event.CommentDeleted) +
			", records the change for offline clients, and writes an audit entry.",
		TokenScope: commentsWrite,
		Input:      commentChangeInput("The comment to delete."),
		Audit: usecase.AuditDeclaration{
			Action: CommentDeletedAction, TargetType: commentTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the tombstone is its own record and the thread is where a deletion is " +
				"read; a history entry beside it would describe the same fact in a second place, " +
				"and the history's one comment verb is the arrival of a comment (item.commented).",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// commentChangeInput is what both changes take beyond the edit's body.
func commentChangeInput(commentDescription string) []usecase.Field {
	return []usecase.Field{
		{
			Name: "item_id", Kind: usecase.KindID, Required: true,
			Description: "The entry the comment is on.",
		},
		{Name: "comment_id", Kind: usecase.KindID, Required: true, Description: commentDescription},
		{
			Name: "expected_version", Kind: usecase.KindInt,
			Description: "The version last read, from the If-Match header over REST. Omitted " +
				"means the caller read none and accepts whatever is there; a version that has " +
				"moved on since is refused rather than overwritten.",
		},
	}
}

func (h EditComment) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := commentChangeCommand(in)
	if err != nil {
		return nil, err
	}
	cmd.Body = in.String("body")

	comment, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return commentOutput(comment), nil
}

func (h DeleteComment) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := commentChangeCommand(in)
	if err != nil {
		return nil, err
	}

	comment, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return commentOutput(comment), nil
}

// commentChangeCommand is the adapter between the catalogue's untyped input and the typed
// command, for both changes and all three channels.
func commentChangeCommand(in usecase.Input) (ChangeCommentCommand, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return ChangeCommentCommand{}, err
	}
	commentID, err := in.ID("comment_id")
	if err != nil {
		return ChangeCommentCommand{}, err
	}
	return ChangeCommentCommand{
		ItemID: itemID, CommentID: commentID, ExpectedVersion: in.Int("expected_version"),
	}, nil
}
