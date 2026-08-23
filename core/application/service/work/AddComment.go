// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"time"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	AddCommentName = "AddComment"
	commentTarget  = "comment"
	commentsWrite  = "comments:write"

	// CommentCreatedAction is the audit code. Stable: an auditor filters on it and a SIEM rule
	// matches on it (audit.md §2).
	CommentCreatedAction audit.Action = "comment.created"
)

// CommentWriter is what every use case that writes the discussion shares: the same reads, the
// same permission question, and the same records - the event outwards, the change log entry for
// offline clients, and the audit entry. What differs between the verbs is which domain method
// decides the new state, and - for an edit and a deletion - the author-or-administrator rule.
type CommentWriter struct {
	Comments   repository.Comments
	Items      repository.Items
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
	Authorizer Authorizer
	// Moderation answers the one question the permission cannot: which role the actor holds.
	// Only the edit and the deletion consult it - "only the author or an administrator may
	// change a comment" is their rule, and it is named where it is enforced (Authorization.go).
	Moderation Moderation
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	Activity   ActivityJournal
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
}

// AddComment puts a contribution on an entry's discussion.
type AddComment struct {
	Writer CommentWriter
}

// AddCommentCommand is the input, typed.
type AddCommentCommand struct {
	ItemID shared.ID
	// ParentCommentID is the comment being replied to, empty for a top-level comment.
	ParentCommentID shared.ID
	Body            string
}

// Execute writes the comment and returns it.
func (h AddComment) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd AddCommentCommand,
) (domain.Comment, error) {
	w := h.Writer
	if cmd.ItemID.IsZero() {
		return domain.Comment{}, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}

	// The entry and its collection are read before the permission question, because the answer
	// depends on the path: a membership held at the hub applies downwards (domain-model.md §3.2).
	// Nothing read here is trusted afterwards - the state that decides the write is read again
	// inside the transaction.
	collection, err := w.readCollectionOf(ctx, actor, cmd.ItemID)
	if err != nil {
		return domain.Comment{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	//
	// WRITE_ITEMS, until C-04: the matrix gives a guest the right to comment without it, and that
	// is exactly the kind of qualifier the permission deliberately does not fold in - C-04 is the
	// task that builds the decision point the qualifiers consult (Authorization.go).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     CommentCreatedAction,
		TokenScope: commentsWrite,
		TargetType: commentTarget,
		// The comment does not exist yet, so the refusal names the entry it would have been
		// written on.
		TargetID: cmd.ItemID,
	}); err != nil {
		return domain.Comment{}, err
	}

	var created domain.Comment
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		item, err := findItem(ctx, w.Items, cmd.ItemID)
		if err != nil {
			return err
		}
		collection, err := findCollection(ctx, w.Containers, item.CollectionID)
		if err != nil {
			return err
		}
		// I-C3: an archived or trashed collection is read-only, and its entries inherit that.
		if err := collection.EnsureAcceptsItems(); err != nil {
			return err
		}
		profile, err := profileOf(ctx, w.Profiles, item.Type)
		if err != nil {
			return err
		}
		if err := item.EnsureCommentable(profile); err != nil {
			return err
		}

		parent, err := w.parentOf(ctx, cmd.ParentCommentID, item.ID)
		if err != nil {
			return err
		}

		comment, err := domain.NewComment(domain.NewCommentInput{
			ID:       w.IDs.NewID(),
			TenantID: actor.TenantID,
			ItemID:   item.ID,
			AuthorID: actor.AccountID,
			Parent:   parent,
			Body:     cmd.Body,
			Now:      now,
		})
		if err != nil {
			return err
		}
		if err := w.Comments.Insert(ctx, comment); err != nil {
			return err
		}

		announcement, err := event.NewCommentCreated(
			w.IDs.NewID(), comment, item.CollectionID,
			event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
		if err != nil {
			return err
		}
		if err := w.Events.Append(ctx, announcement); err != nil {
			return err
		}
		if err := w.recordChange(ctx, comment, item, actor, announcement.Payload); err != nil {
			return err
		}
		if err := w.recordAudit(ctx, comment, actor, CommentCreatedAction, now); err != nil {
			return err
		}
		// The one comment verb of the item's history (C-03). An edit and a deletion write none:
		// the comment carries its own stamps, and the thread is where both are read.
		if err := w.Activity.record(ctx, actor, item, activity.ItemCommented,
			activity.ChangeSet(historyForm(profile), activity.Field{
				Name: "comment_id", Detail: activity.WithValues, To: comment.ID.String(),
			}), now); err != nil {
			return err
		}

		created = comment
		return nil
	})
	if err != nil {
		return domain.Comment{}, err
	}
	return created, nil
}

// parentOf reads the comment being replied to, or nil for a top-level comment. What the parent
// has to be - on the same entry, not deleted, not itself a reply - is the domain's question
// (NewComment); this only fetches it.
func (w CommentWriter) parentOf(
	ctx context.Context, parentID, itemID shared.ID,
) (*domain.Comment, error) {
	if parentID.IsZero() {
		return nil, nil
	}

	parent, err := w.Comments.Find(ctx, parentID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, shared.ErrNotFound.
				WithDetail("comments.parent_not_found").
				WithParams(map[string]string{"parent_comment_id": parentID.String()}).
				WithFields(shared.FieldError{
					Path: "/parent_comment_id", Code: "comments.parent_not_found",
				})
		}
		return nil, err
	}
	if parent.ItemID != itemID {
		// The same answer as a parent that does not exist would get a different code here: this
		// one names the real problem, and there is nothing to disclose - the caller can read the
		// entry either comment is on, or they could not have reached this line.
		return nil, shared.ErrValidation.
			WithDetail("comments.parent_not_on_item").
			WithFields(shared.FieldError{
				Path: "/parent_comment_id", Code: "comments.parent_not_on_item",
			})
	}
	return &parent, nil
}

// readCollectionOf reads the collection an entry belongs to, outside the write transaction,
// because the permission check needs its path first. Read-only, so it may be served by a replica.
func (w CommentWriter) readCollectionOf(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) (domain.Container, error) {
	var collection domain.Container

	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		item, err := findItem(ctx, w.Items, itemID)
		if err != nil {
			return err
		}
		found, err := findCollection(ctx, w.Containers, item.CollectionID)
		collection = found
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	return collection, nil
}

// recordChange writes what an offline client has to be told.
//
// One entry carrying the whole comment, which is the merge rule for an appending list written
// down: comments append and never merge, so a new one is not a field of anything - it is a new
// record of its own entity (offline-sync.md §4.2). The payload is the event's, deliberately: the
// two recipients describe the same state, and building it twice is how they come to disagree.
func (w CommentWriter) recordChange(
	ctx context.Context, comment domain.Comment, item domain.WorkItem,
	actor appshared.ActorContext, snapshot map[string]any,
) error {
	return w.Changes.Record(ctx, changelog.Change{
		TenantID: comment.TenantID,
		Entity:   commentTarget,
		EntityID: comment.ID,
		Op:       changelog.Upsert,
		// The visibility filter a pull applies: a comment is visible to a device subscribed to
		// the collection its entry is in, exactly as the entry itself is.
		ContainerID: item.CollectionID,
		ActorID:     actor.AccountID,
		HLC:         w.HLC.Next(),
		Payload:     snapshot,
	})
}

// recordAudit writes the evidence, inside the same transaction as the change (test AT-5).
//
// The body is user content and is recorded as a fingerprint rather than as itself (rule 10,
// audit.md §4): enough to see that two entries concern the same words, not enough to read them.
func (w CommentWriter) recordAudit(
	ctx context.Context, comment domain.Comment, actor appshared.ActorContext,
	action audit.Action, now time.Time,
) error {
	var parent any
	if !comment.ParentCommentID.IsZero() {
		parent = comment.ParentCommentID.String()
	}

	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   comment.TenantID,
		OccurredAt: now,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: commentTarget,
		TargetID:   comment.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "item_id", Classification: audit.Open, To: comment.ItemID.String()},
			audit.Change{Field: "author_id", Classification: audit.Open, To: comment.AuthorID.String()},
			audit.Change{Field: "parent_comment_id", Classification: audit.Open, To: parent},
			audit.Change{Field: "body", Classification: audit.Sensitive, To: comment.Body},
		),
	})
}

// commentOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema Comment) - and the event's payload, which is the same shape minus the
// collection a comment event carries for its consumers' filtering.
func commentOutput(comment domain.Comment) usecase.Output {
	out := usecase.Output{
		"id":                comment.ID.String(),
		"item_id":           comment.ItemID.String(),
		"author_id":         comment.AuthorID.String(),
		"parent_comment_id": nil,
		"body":              nil,
		"created_at":        comment.CreatedAt,
		"edited_at":         timeOrNil(comment.EditedAt),
		"deleted_at":        timeOrNil(comment.DeletedAt),
		"version":           comment.Version,
	}
	if !comment.ParentCommentID.IsZero() {
		out["parent_comment_id"] = comment.ParentCommentID.String()
	}
	if comment.DeletedAt == nil {
		out["body"] = comment.Body
	}
	return out
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h AddComment) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AddCommentName,
		Summary: "Adds a comment to an entry's discussion. A reply names the comment it answers " +
			"with parent_comment_id; threading is one level, so a reply cannot itself be replied " +
			"to. A type without the COMMENTS capability - an activity - carries no discussion " +
			"and is refused. The body counts at most 20000 characters.",
		SideEffects: "Writes the comment, announces " + string(event.CommentCreated) +
			", records the change for offline clients, writes an audit entry and a step of the " +
			"entry's history.",
		TokenScope: commentsWrite,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry to comment on.",
			},
			{
				Name: "body", Kind: usecase.KindString, Required: true,
				Description: "The text, up to 20000 characters. Markdown, not rendered by the server.",
			},
			{
				Name: "parent_comment_id", Kind: usecase.KindID,
				Description: "The comment this one replies to. Omitted for a top-level comment; " +
					"a reply cannot itself be replied to.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: CommentCreatedAction, TargetType: commentTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemCommented},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// invoke is the adapter between the catalogue's untyped input and the typed command, for all
// three channels.
func (h AddComment) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	parentID, err := in.ID("parent_comment_id")
	if err != nil {
		return nil, err
	}

	comment, err := h.Execute(ctx, actor, AddCommentCommand{
		ItemID:          itemID,
		ParentCommentID: parentID,
		Body:            in.String("body"),
	})
	if err != nil {
		return nil, err
	}
	return commentOutput(comment), nil
}
