// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ConvertJumbleEntryName = "ConvertJumbleEntry"
	DismissJumbleEntryName = "DismissJumbleEntry"

	// CreateWorkItemName is the use case the conversion performs its item write through. The name
	// rather than an import: the work service is a sibling, and the registry is the one place a
	// use case reaches another (the bulk precedent).
	createWorkItemName = "CreateWorkItem"

	// EntryConvertedAction and EntryDismissedAction are settlements: somebody - or a rule, as its
	// account - decided what an arrival becomes. The item create inside a conversion writes its
	// own entry too; this one is about the entry.
	EntryConvertedAction audit.Action = "jumble.entry_converted"
	EntryDismissedAction audit.Action = "jumble.entry_dismissed"

	// maxDerivedTitle bounds the title taken from an entry's body: the first line, but never a
	// paragraph. The item's own validation bounds the rest.
	maxDerivedTitle = 120
)

// Catalogue is the slice of the use case registry the conversion needs: perform one named use
// case, as the given actor. Narrow, so the conversion can create an item and nothing else it did
// not name.
type Catalogue interface {
	Invoke(
		ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input,
	) (usecase.Output, error)
}

// ItemOrigins writes the provenance a conversion records: which entry an item came from. Its own
// narrow port rather than the item repository, for the reason every slice here is.
type ItemOrigins interface {
	// RecordOrigin sets the item's origin exactly once. False means another conversion, or
	// nothing, already owns the row.
	RecordOrigin(ctx context.Context, itemID, entryID shared.ID) (bool, error)
}

// ConvertJumbleEntry turns an entry into work.
//
// The item write goes through CreateWorkItem - the same use case a plain create goes through, as
// the same actor, with the destination's rights checked there (rule 2). What this use case owns
// is the settlement: the entry is decided about exactly once, the provenance lands on both sides,
// and everything commits together - a settlement that lost its race rolls the created item back
// with it.
type ConvertJumbleEntry struct {
	Writer    Writer
	Catalogue Catalogue
	Origins   ItemOrigins
}

// ConvertCommand names the entry and its destination.
type ConvertCommand struct {
	EntryID      shared.ID
	CollectionID shared.ID
	BucketID     shared.ID
	Title        string
	Type         string
}

// Execute converts.
func (h ConvertJumbleEntry) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ConvertCommand,
) (domain.Entry, error) {
	w := h.Writer

	if cmd.CollectionID.IsZero() {
		return domain.Entry{}, shared.ErrValidation.
			WithDetail("jumble.destination_required").
			WithFields(shared.FieldError{Path: "/collection_id", Code: "jumble.destination_required"})
	}

	// The entry's own permission, at the tenant: settling the inbox. The destination's rights
	// are the item create's question, asked inside the transaction below with the caller's own
	// actor - a rule's run_as account converts with its real rights (automation.md §2).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     EntryConvertedAction,
		TokenScope: itemsWriteScope,
		TargetType: entryTarget,
		TargetID:   cmd.EntryID,
	}); err != nil {
		return domain.Entry{}, err
	}

	now := w.Clock.Now()
	var converted domain.Entry
	err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		entry, err := w.Entries.Find(ctx, cmd.EntryID)
		if err != nil {
			return err
		}

		title, err := titleFor(entry, cmd.Title)
		if err != nil {
			return err
		}
		kind := strings.TrimSpace(cmd.Type)
		if kind == "" {
			kind = "TASK"
		}

		input := usecase.Input{
			"type": kind, "title": title, "collection_id": cmd.CollectionID.String(),
		}
		if !cmd.BucketID.IsZero() {
			input["bucket_id"] = cmd.BucketID.String()
		}
		// A mail sorted in keeps what it said and what it carried, as one delivered straight to
		// the collection would (ADR-0092 decision 6).
		if entry.Channel == domain.ChannelEmail {
			notes := strings.TrimSpace(domain.CutQuotedReply(entry.RawBody))
			if entry.Sender != "" {
				// Who sent it, above what they wrote: the entry is the settler's, the words are not.
				notes = strings.TrimSpace("From: " + entry.Sender + "\n\n" + notes)
			}
			if notes != "" {
				input["notes"] = notes
			}
		}
		out, err := h.Catalogue.Invoke(ctx, createWorkItemName, actor, input)
		if err != nil {
			return err
		}
		itemID, err := shared.ParseID(out.String("id"))
		if err != nil {
			return shared.ErrInternal.WithDetail("jumble.entry_incomplete").WithCause(err)
		}
		if entry.Channel == domain.ChannelEmail {
			if err := attachAll(ctx, h.Catalogue, actor, itemID, entry.Attachments); err != nil {
				return err
			}
		}

		if converted, err = entry.Convert(itemID, now); err != nil {
			return err
		}
		decided, err := w.Entries.Settle(ctx, converted)
		if err != nil {
			return err
		}
		if !decided {
			// Another settlement got there first. The refusal rolls the transaction back, and
			// the item this call created goes with it - one entry, one item, ever.
			return shared.ErrConflict.
				WithDetail("jumble.entry_settled").
				WithParams(map[string]string{"entry_id": entry.ID.String()})
		}

		if _, err := h.Origins.RecordOrigin(ctx, itemID, entry.ID); err != nil {
			return err
		}
		return w.announceConversion(ctx, converted, cmd.CollectionID,
			event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now)
	})
	if err != nil {
		return domain.Entry{}, err
	}

	w.record(ctx, actor, EntryConvertedAction, converted, now)
	return converted, nil
}

// titleFor derives the item's title: the caller's, the subject, or the first line of the body -
// and a refusal for an entry with no text at all, because inventing a title would put words in
// the workspace nobody wrote.
func titleFor(entry domain.Entry, requested string) (string, error) {
	if title := strings.TrimSpace(requested); title != "" {
		return title, nil
	}
	if entry.RawSubject != "" {
		return entry.RawSubject, nil
	}

	body := strings.TrimSpace(entry.RawBody)
	if line, _, cut := strings.Cut(body, "\n"); cut {
		body = strings.TrimSpace(line)
	}
	if body != "" {
		runes := []rune(body)
		if len(runes) > maxDerivedTitle {
			body = string(runes[:maxDerivedTitle])
		}
		return body, nil
	}
	return "", shared.ErrValidation.
		WithDetail("jumble.title_required").
		WithFields(shared.FieldError{Path: "/title", Code: "jumble.title_required"})
}

// announceConversion publishes jumble.entry_converted beside the settlement.
func (w Writer) announceConversion(
	ctx context.Context, entry domain.Entry, collectionID shared.ID,
	actor event.Actor, now time.Time,
) error {
	if w.Events == nil {
		return nil
	}
	envelope, err := event.NewJumbleEntryConverted(
		w.IDs.NewID(), entry, collectionID, actor, now, event.Cause{})
	if err != nil {
		return err
	}
	return w.Events.Append(ctx, envelope)
}

// DismissJumbleEntry decides against an entry. A state, not a deletion: the entry stays
// readable, and the retention engine ages it out by rule rather than by hand.
type DismissJumbleEntry struct{ Writer Writer }

// Execute dismisses.
func (h DismissJumbleEntry) Execute(
	ctx context.Context, actor appshared.ActorContext, entryID shared.ID,
) (domain.Entry, error) {
	w := h.Writer

	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     EntryDismissedAction,
		TokenScope: itemsWriteScope,
		TargetType: entryTarget,
		TargetID:   entryID,
	}); err != nil {
		return domain.Entry{}, err
	}

	now := w.Clock.Now()
	var dismissed domain.Entry
	err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		entry, err := w.Entries.Find(ctx, entryID)
		if err != nil {
			return err
		}
		if dismissed, err = entry.Dismiss(now); err != nil {
			return err
		}
		decided, err := w.Entries.Settle(ctx, dismissed)
		if err != nil {
			return err
		}
		if !decided {
			return shared.ErrConflict.
				WithDetail("jumble.entry_settled").
				WithParams(map[string]string{"entry_id": entry.ID.String()})
		}
		return nil
	})
	if err != nil {
		return domain.Entry{}, err
	}

	w.record(ctx, actor, EntryDismissedAction, dismissed, now)
	return dismissed, nil
}

// attachAll puts an entry's files on the item it became, as the settling actor.
func attachAll(
	ctx context.Context, catalogue Catalogue, actor appshared.ActorContext, itemID shared.ID,
	files []shared.ID,
) error {
	for _, file := range files {
		if _, err := catalogue.Invoke(ctx, attachMediaName, actor, usecase.Input{
			"item_id": itemID.String(), "media_id": file.String(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// CommentJumbleEntryName is the settlement that answers an entry with a comment.
const CommentJumbleEntryName = "CommentJumbleEntry"

// CommentJumbleEntry settles a jumble entry as a comment on an entry - what a mail meant for an
// entry becomes once somebody sorts it in (ADR-0092 decision 5). The comment goes through
// AddComment and the files through AttachMedia, as the settling actor.
type CommentJumbleEntry struct {
	Writer    Writer
	Catalogue Catalogue
}

// Execute settles.
func (h CommentJumbleEntry) Execute(
	ctx context.Context, actor appshared.ActorContext, entryID, itemID shared.ID,
) (domain.Entry, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     EntryConvertedAction,
		TokenScope: itemsWriteScope,
		TargetType: entryTarget,
		TargetID:   entryID,
	}); err != nil {
		return domain.Entry{}, err
	}

	now := w.Clock.Now()
	var settled domain.Entry
	err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		entry, err := w.Entries.Find(ctx, entryID)
		if err != nil {
			return err
		}
		target := itemID
		if target.IsZero() && entry.MeantFor.Kind == domain.TargetItem {
			target = entry.MeantFor.ID
		}
		if target.IsZero() {
			return shared.ErrValidation.WithDetail("jumble.destination_required").
				WithFields(shared.FieldError{Path: "/item_id", Code: "jumble.destination_required"})
		}

		body := strings.TrimSpace(domain.CutQuotedReply(entry.RawBody))
		if subject := strings.TrimSpace(entry.RawSubject); subject != "" && body == "" {
			body = subject
		}
		if entry.Sender != "" {
			body = strings.TrimSpace("From: " + entry.Sender + "\n\n" + body)
		}
		if body == "" {
			return shared.ErrValidation.WithDetail("jumble.entry_empty")
		}
		if _, err := h.Catalogue.Invoke(ctx, addCommentName, actor, usecase.Input{
			"item_id": target.String(), "body": body,
		}); err != nil {
			return err
		}
		if err := attachAll(ctx, h.Catalogue, actor, target, entry.Attachments); err != nil {
			return err
		}

		if settled, err = entry.Convert(target, now); err != nil {
			return err
		}
		decided, err := w.Entries.Settle(ctx, settled)
		if err != nil {
			return err
		}
		if !decided {
			return shared.ErrConflict.
				WithDetail("jumble.entry_settled").
				WithParams(map[string]string{"entry_id": entry.ID.String()})
		}
		return w.announceConversion(ctx, settled, "",
			event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now)
	})
	if err != nil {
		return domain.Entry{}, err
	}

	w.record(ctx, actor, EntryConvertedAction, settled, now)
	return settled, nil
}

// Descriptor is the catalogue entry.
func (h CommentJumbleEntry) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CommentJumbleEntryName,
		Summary: "Settles a jumble entry as a comment on an entry - by default the entry a mail " +
			"was meant for - with its files attached to that entry. Exactly once, like a conversion.",
		SideEffects: "Adds the comment and the files, settles the entry, publishes " +
			"jumble.entry_converted and writes audit entries.",
		TokenScope: itemsWriteScope,
		Input: []usecase.Field{
			{Name: "entry_id", Kind: usecase.KindID, Required: true, Description: "Which jumble entry."},
			{
				Name: "item_id", Kind: usecase.KindID,
				Description: "The entry to comment on. Defaults to the one the mail was meant for.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: EntryConvertedAction, TargetType: entryTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The comment inside the settlement writes the activity step.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CommentJumbleEntry) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	entryID, err := in.ID("entry_id")
	if err != nil {
		return nil, err
	}
	var itemID shared.ID
	if in.Present("item_id") {
		if itemID, err = in.ID("item_id"); err != nil {
			return nil, err
		}
	}
	entry, err := h.Execute(ctx, actor, entryID, itemID)
	if err != nil {
		return nil, err
	}
	return entryOutput(entry), nil
}
