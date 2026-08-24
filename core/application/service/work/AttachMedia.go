// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	AttachMediaName = "AttachMedia"
	DetachMediaName = "DetachMedia"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	ItemAttachmentAddedAction   audit.Action = "item.attachment_added"
	ItemAttachmentRemovedAction audit.Action = "item.attachment_removed"
)

// ItemAttachmentWriter is what both directions share.
//
// The tokens are the entry's rather than the media context's, for the reason the label pair's are
// the entry's: attaching a file to an entry is work on that entry, and somebody who may edit it may
// attach to it. Staging the file needed `media:write`; putting it on an entry needs the right to
// change the entry (security.md §5, domain-model.md §3.2).
type ItemAttachmentWriter struct {
	Items       repository.Items
	Containers  repository.Containers
	Profiles    metarepo.CapabilityProfiles
	Attachments mediarepo.Attachments
	Media       MediaReferences
	Authorizer  Authorizer
	Events      outbox.Events
	Changes     changelog.ChangeLog
	Audit       audit.Sink
	Activity    ActivityJournal
	UnitOfWork  persistence.UnitOfWork
	Clock       clock.Clock
	IDs         clock.IDGenerator
	HLC         clock.HLCSource
}

// AttachMedia puts a file on an entry.
//
// The bytes are shared, never copied: attaching the same object to three entries is three
// references to one file, which is what the reference count counts and what the reconciliation job
// decides deletion by (data-protection.md §5).
type AttachMedia struct {
	Writer ItemAttachmentWriter
}

// DetachMedia takes a file off an entry. The object itself stays - it may serve other entries, and
// what decides its life is the count rather than this call.
type DetachMedia struct {
	Writer ItemAttachmentWriter
}

// AttachmentCommand is the input of both directions, typed.
//
// No expected version. An attachment lives beside the entry rather than on its row, so there is no
// version of the entry that it would be racing - which is also why neither operation spends one.
// Two devices attaching two different files at once is the case the OR-set exists to serve, and an
// optimistic lock on the entry would make one of them fail for no reason (offline-sync.md §4.2).
type AttachmentCommand struct {
	ItemID  shared.ID
	MediaID shared.ID
}

// ItemAttachmentSet is what an entry carries, as every channel reports it.
type ItemAttachmentSet struct {
	ItemID   shared.ID
	MediaIDs []shared.ID
}

// Execute puts the file on the entry and returns what it now carries.
func (h AttachMedia) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd AttachmentCommand,
) (ItemAttachmentSet, error) {
	return h.Writer.change(ctx, actor, cmd, attaching)
}

// Execute takes the file off the entry and returns what it now carries.
func (h DetachMedia) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd AttachmentCommand,
) (ItemAttachmentSet, error) {
	return h.Writer.change(ctx, actor, cmd, detaching)
}

// attachmentDirection is which way the caller asked for. Not a boolean, for the reason
// labelDirection is not: this is the parameter that decides which of two audit trails is written.
type attachmentDirection bool

const (
	attaching attachmentDirection = true
	detaching attachmentDirection = false
)

func (d attachmentDirection) action() audit.Action {
	if d == attaching {
		return ItemAttachmentAddedAction
	}
	return ItemAttachmentRemovedAction
}

func (d attachmentDirection) verb() activity.Verb {
	if d == attaching {
		return activity.ItemAttachmentAdded
	}
	return activity.ItemAttachmentRemoved
}

// change is the whole of both use cases.
func (w ItemAttachmentWriter) change(
	ctx context.Context, actor appshared.ActorContext, cmd AttachmentCommand,
	want attachmentDirection,
) (ItemAttachmentSet, error) {
	if cmd.ItemID.IsZero() {
		return ItemAttachmentSet{}, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}
	if cmd.MediaID.IsZero() {
		return ItemAttachmentSet{}, shared.ErrValidation.
			WithDetail("media.media_id_required").
			WithFields(shared.FieldError{Path: "/media_id", Code: "media.media_id_required"})
	}

	// The entry and its collection are read before the permission question, because the answer
	// depends on the path: a membership held at the hub applies downwards (domain-model.md §3.2).
	// Nothing read here is trusted afterwards - the state that decides the write is read again
	// inside the transaction.
	subject, collection, err := readItemScope(
		ctx, w.UnitOfWork, w.Items, w.Containers, actor, cmd.ItemID)
	if err != nil {
		return ItemAttachmentSet{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     want.action(),
		TokenScope: itemsWrite,
		TargetType: itemTarget,
		TargetID:   cmd.ItemID,
		On:         changing(subject),
	}); err != nil {
		return ItemAttachmentSet{}, err
	}

	var carried ItemAttachmentSet
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
		// The capability before the state, exactly as the label pair asks it: "an activity carries
		// no files" is true of the type whatever state one particular activity is in, and
		// answering with the state first would send a client off to unarchive an entry whose
		// attachment would still be refused.
		if err := item.EnsureAttachable(profile); err != nil {
			return err
		}
		if want == attaching {
			if err := w.ensureFileUsable(ctx, cmd.MediaID); err != nil {
				return err
			}
		}

		// The tag is the clock reading the OR-set merges on. It is taken here rather than derived
		// from `now`, because a merge orders changes against other devices' readings and a wall
		// clock cannot do that (offline-sync.md §4.1).
		tag := w.HLC.Next()

		moved, err := w.apply(ctx, cmd, want, tag)
		if err != nil {
			return err
		}

		ids, err := w.Attachments.MediaIDs(ctx, cmd.ItemID)
		if err != nil {
			return err
		}
		carried = ItemAttachmentSet{ItemID: cmd.ItemID, MediaIDs: ids}

		if !moved {
			// The entry already carries the file, or already does not. The tag has been written
			// all the same - a device that decided this has made a decision another replica has to
			// merge against - but nothing is announced and no counter moves, because nothing about
			// the entry changed.
			return nil
		}
		return w.announce(ctx, actor, item, collection, cmd.MediaID, want, tag, now)
	})
	if err != nil {
		return ItemAttachmentSet{}, err
	}
	return carried, nil
}

// apply writes the link and its tag, moves the reference counter, and reports whether the set
// actually moved.
//
// The counter moves inside the same transaction as the link, which is what makes the two one fact
// rather than two that can disagree. A detachment never deletes the object: it may serve other
// entries, and one that has just lost its last reference is left to the reconciliation job - which
// is the ordering `ON DELETE RESTRICT` on `item_attachment` makes impossible to get wrong.
func (w ItemAttachmentWriter) apply(
	ctx context.Context, cmd AttachmentCommand, want attachmentDirection, tag shared.HLC,
) (bool, error) {
	if want == detaching {
		removed, err := w.Attachments.Remove(ctx, cmd.ItemID, cmd.MediaID, tag)
		if err != nil || !removed {
			return false, err
		}
		return true, w.Media.AdjustRefCount(ctx, cmd.MediaID, -1)
	}

	added, err := w.Attachments.Add(ctx, cmd.ItemID, cmd.MediaID, tag)
	if err != nil || !added {
		return false, err
	}
	return true, w.Media.AdjustRefCount(ctx, cmd.MediaID, 1)
}

// ensureFileUsable refuses a media object the media context will not stand behind.
//
// The judgement is the media domain's: READY, staged as an attachment, and not on its way out
// (media.Object.Attachable). The tenant is not a question at all - row level security answers it by
// not returning another tenant's row (ADR-0010).
func (w ItemAttachmentWriter) ensureFileUsable(ctx context.Context, mediaID shared.ID) error {
	object, err := w.Media.Find(ctx, mediaID)
	if err != nil {
		return mediaOrNotFound(err, mediaID)
	}
	return object.Attachable(media.UsageAttachment)
}

// announce records what a change to an entry's attachments owes: the event outwards, the change log
// for offline clients, the audit entry, and the step of the entry's own history - all inside the
// caller's transaction (test AT-5).
func (w ItemAttachmentWriter) announce(
	ctx context.Context, actor appshared.ActorContext, item domain.WorkItem,
	collection domain.Container, mediaID shared.ID, want attachmentDirection,
	tag shared.HLC, now time.Time,
) error {
	by := event.Actor{Kind: actor.Kind, ID: actor.AccountID}

	announcement, err := event.NewAttachmentAdded(w.IDs.NewID(), item, mediaID, by, now, event.Cause{})
	if want == detaching {
		announcement, err = event.NewAttachmentRemoved(
			w.IDs.NewID(), item, mediaID, by, now, event.Cause{})
	}
	if err != nil {
		return err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return err
	}
	if err := w.recordChange(ctx, item, collection, actor, mediaID, want, tag); err != nil {
		return err
	}
	if err := w.recordAudit(ctx, item, actor, mediaID, want, now); err != nil {
		return err
	}
	return w.recordActivity(ctx, item, actor, mediaID, want, now)
}

// recordActivity writes the step of the entry's own history.
//
// The file travels as the side it moved to: `to` for an attachment, `from` for a detachment, so
// that the change set reads the same way round as the verb. By identifier and never by name - a
// file name is user content, and rule 10 keeps it out of the record.
func (w ItemAttachmentWriter) recordActivity(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	mediaID shared.ID, want attachmentDirection, now time.Time,
) error {
	field := activity.Field{Name: "media_id", Detail: activity.WithValues, To: mediaID.String()}
	if want == detaching {
		field = activity.Field{
			Name: "media_id", Detail: activity.WithValues, From: mediaID.String(),
		}
	}

	return w.Activity.record(ctx, actor, item, want.verb(),
		activity.ChangeSet(activity.Full, field), now)
}

// recordChange writes what an offline client has to be told.
//
// The payload is the one element that moved and the tag that decides it, not the whole set. That is
// the merge rule for a set written down: an entry carrying `["a"]` and one carrying `["b"]` merge
// to both, and a payload naming the whole array would let the later of the two writers erase the
// other's file (offline-sync.md §4.2). The shape is the label pair's, because it is the same shape.
func (w ItemAttachmentWriter) recordChange(
	ctx context.Context, item domain.WorkItem, collection domain.Container,
	actor appshared.ActorContext, mediaID shared.ID, want attachmentDirection, tag shared.HLC,
) error {
	operation := "add"
	if want == detaching {
		operation = "remove"
	}

	return w.Changes.Record(ctx, changelog.Change{
		TenantID: item.TenantID,
		Entity:   itemTarget,
		EntityID: item.ID,
		Op:       changelog.Upsert,
		// The visibility filter a pull applies: the hub above the collection, so that a device
		// subscribed to the hub sees the change (offline-sync.md §3.1).
		ContainerID: firstNonZero(collection.ParentID, item.CollectionID),
		ActorID:     actor.AccountID,
		HLC:         tag,
		Payload: map[string]any{
			"set":        string(domain.SetAttachments),
			"element_id": mediaID.String(),
			"op":         operation,
		},
	})
}

// recordAudit writes the evidence.
//
// The file is recorded by identifier rather than by name, for the reason a label is: a file name is
// user content and stays out of the trail, and an identifier is what an auditor needs in order to
// ask which file it was (rule 10, audit.md §4).
func (w ItemAttachmentWriter) recordAudit(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	mediaID shared.ID, want attachmentDirection, now time.Time,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   item.TenantID,
		OccurredAt: now,
		Action:     want.action(),
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: itemTarget,
		TargetID:   item.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "media_id", Classification: audit.Open, To: mediaID.String()},
			audit.Change{
				Field: "collection_id", Classification: audit.Open, To: item.CollectionID.String(),
			},
		),
	})
}

// attachmentSetOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema ItemAttachments).
func attachmentSetOutput(set ItemAttachmentSet) usecase.Output {
	ids := make([]string, 0, len(set.MediaIDs))
	for _, id := range set.MediaIDs {
		ids = append(ids, id.String())
	}
	return usecase.Output{"item_id": set.ItemID.String(), "media_ids": ids}
}

// Descriptor is the catalogue entry.
func (h AttachMedia) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AttachMediaName,
		Summary: "Attaches a media object to an entry. The object has to be READY, staged as an " +
			"attachment and of this workspace, and the entry's type has to carry attachments at " +
			"all: an activity does not. The bytes are shared, never copied - attaching raises the " +
			"object's reference count. Idempotent: an entry that already carries it succeeds and " +
			"announces nothing.",
		SideEffects: "Writes the link and its merge tag, raises the media reference count, " +
			"announces " + string(event.AttachmentAdded) + ", records the change for offline " +
			"clients, writes an audit entry and a step of the entry's history.",
		TokenScope: itemsWrite,
		Input:      attachmentInput("The entry to attach to.", "The media object to attach."),
		Audit: usecase.AuditDeclaration{
			Action: ItemAttachmentAddedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemAttachmentAdded},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h DetachMedia) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DetachMediaName,
		Summary: "Takes a media object off an entry. Idempotent: an entry that does not carry it " +
			"succeeds and announces nothing. The object itself stays - it may serve other entries - " +
			"and one that loses its last reference is removed by the reconciliation job rather " +
			"than by this call.",
		SideEffects: "Removes the link, writes its merge tag, lowers the media reference count, " +
			"announces " + string(event.AttachmentRemoved) + ", records the change for offline " +
			"clients, writes an audit entry and a step of the entry's history.",
		TokenScope: itemsWrite,
		Input:      attachmentInput("The entry to detach from.", "The media object to detach."),
		Audit: usecase.AuditDeclaration{
			Action: ItemAttachmentRemovedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemAttachmentRemoved},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// attachmentInput is what both directions take. One list, so that a client which learned one of
// them from /meta/capabilities does not find the other spelled differently.
func attachmentInput(itemDescription, mediaDescription string) []usecase.Field {
	return []usecase.Field{
		{Name: "item_id", Kind: usecase.KindID, Required: true, Description: itemDescription},
		{Name: "media_id", Kind: usecase.KindID, Required: true, Description: mediaDescription},
	}
}

func (h AttachMedia) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := attachmentCommand(in)
	if err != nil {
		return nil, err
	}

	set, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return attachmentSetOutput(set), nil
}

func (h DetachMedia) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := attachmentCommand(in)
	if err != nil {
		return nil, err
	}

	set, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return attachmentSetOutput(set), nil
}

// attachmentCommand is the adapter between the catalogue's untyped input and the typed command, for
// both directions and all three channels.
func attachmentCommand(in usecase.Input) (AttachmentCommand, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return AttachmentCommand{}, err
	}
	mediaID, err := in.ID("media_id")
	if err != nil {
		return AttachmentCommand{}, err
	}
	return AttachmentCommand{ItemID: itemID, MediaID: mediaID}, nil
}
