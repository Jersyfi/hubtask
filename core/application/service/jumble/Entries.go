// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package jumble holds the use cases of the inbox (G-10, domain-model.md §5): something arrives,
// is listed, and becomes work or ages out.
//
// Jumble content is the least trusted text in the system and this layer keeps it where it
// belongs: in the row and in the API answer, never in a log, a metric, a trace, an audit entry or
// an event payload (rule 10, data-protection.md).
package jumble

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/jumble"
	outboxrepo "github.com/Jersyfi/hubtask/core/application/repository/outbox"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	SubmitJumbleEntryName = "SubmitJumbleEntry"
	ListJumbleEntriesName = "ListJumbleEntries"

	// The jumble reads and writes with the work item scopes rather than a vocabulary of its own:
	// the jumble is the work context's inbox, its entries exist to become items, and a
	// jumble-only scope waits for a real second need (api-guidelines.md §7's closed list).
	itemsWriteScope = "items:write"
	itemsReadScope  = "items:read"

	entryTarget = "jumble_entry"

	// EntrySubmittedAction is an arrival. Recorded without the content: what the trail says is
	// that something arrived over a channel, never what it said (rule 10).
	EntrySubmittedAction audit.Action = "jumble.entry_submitted"
	// EntryReadAction is what a listing performs. Info and not required, for the run log's
	// reason: an entry per read would bury the entries a review looks for.
	EntryReadAction audit.Action = "jumble.entry_read"
)

// Authorizer is the application's own decision point (ADR-0005).
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Media is the two touches an attachment costs: the object read that proves it exists and is
// sealed, and the fast half of the reference count - the recount job recomputes it from the
// actual references, jumble entries included, so the counter stays a cache (C-06).
type Media interface {
	Find(ctx context.Context, id shared.ID) (media.Object, error)
	AdjustRefCount(ctx context.Context, id shared.ID, delta int) error
}

// Writer is what the jumble use cases share.
type Writer struct {
	Entries    repository.Entries
	Media      Media
	Events     outboxrepo.Events
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// SubmitJumbleEntry catches one near-channel arrival (G-10).
type SubmitJumbleEntry struct{ Writer Writer }

// SubmitCommand is one arrival as a caller describes it.
type SubmitCommand struct {
	Channel     domain.Channel
	Sender      string
	RawSubject  string
	RawBody     string
	Attachments []shared.ID
}

// Execute stores the arrival and announces it.
func (h SubmitJumbleEntry) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd SubmitCommand,
) (domain.Entry, error) {
	w := h.Writer

	channel := cmd.Channel
	if channel == "" {
		channel = domain.ChannelAPI
	}
	if channel != domain.ChannelAPI && channel != domain.ChannelQuickCapture {
		// EMAIL and WEBHOOK name the intakes that authenticate their own way (G-10, G-11). An
		// entry claiming one through this route would be forging its provenance - the one thing
		// the channel column exists to record.
		return domain.Entry{}, shared.ErrValidation.
			WithDetail("jumble.channel_reserved").
			WithParams(map[string]string{"channel": channel.String()}).
			WithFields(shared.FieldError{Path: "/channel", Code: "jumble.channel_reserved"})
	}

	now := w.Clock.Now()
	entry, err := domain.NewEntry(domain.NewEntryInput{
		ID: w.IDs.NewID(), TenantID: actor.TenantID, Channel: channel,
		Sender: cmd.Sender, RawSubject: cmd.RawSubject, RawBody: cmd.RawBody,
		Attachments: cmd.Attachments, Now: now,
	})
	if err != nil {
		return domain.Entry{}, err
	}

	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     EntrySubmittedAction,
		TokenScope: itemsWriteScope,
		TargetType: entryTarget,
		TargetID:   entry.ID,
	}); err != nil {
		return domain.Entry{}, err
	}

	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := w.attach(ctx, entry.Attachments); err != nil {
			return err
		}
		if err := w.Entries.Insert(ctx, entry); err != nil {
			return err
		}
		return w.announceArrival(ctx, entry, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now)
	})
	if err != nil {
		return domain.Entry{}, err
	}

	w.record(ctx, actor, EntrySubmittedAction, entry, now)
	return entry, nil
}

// attach proves every named object exists and is sealed, and moves the fast half of its count.
//
// The same judgement an item attachment makes (media.Object.Attachable): a PENDING staging and a
// marked object are refused, so nothing can reference what the reconciliation is about to
// reclaim - the failure #231 recorded.
func (w Writer) attach(ctx context.Context, attachments []shared.ID) error {
	for _, id := range attachments {
		object, err := w.Media.Find(ctx, id)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				// Another tenant's object is invisible under row level security, so "not yours"
				// and "not there" are deliberately one answer (T-04).
				return shared.ErrValidation.
					WithDetail("jumble.attachment_unknown").
					WithParams(map[string]string{"media_id": id.String()}).
					WithFields(shared.FieldError{
						Path: "/attachments", Code: "jumble.attachment_unknown",
					})
			}
			return err
		}
		if err := object.Attachable(media.UsageAttachment); err != nil {
			return err
		}
		if err := w.Media.AdjustRefCount(ctx, id, 1); err != nil {
			return err
		}
	}
	return nil
}

// announceArrival publishes jumble.entry_received into the same transaction as the row, so the
// entry and its announcement cannot come apart (ADR-0007). This is what fires a JUMBLE_ENTRY rule.
func (w Writer) announceArrival(
	ctx context.Context, entry domain.Entry, actor event.Actor, now time.Time,
) error {
	if w.Events == nil {
		return nil
	}
	envelope, err := event.NewJumbleEntryReceived(w.IDs.NewID(), entry, actor, now, event.Cause{})
	if err != nil {
		return err
	}
	return w.Events.Append(ctx, envelope)
}

// ListJumbleEntries answers a page of the inbox.
type ListJumbleEntries struct{ Writer Writer }

// Execute reads a page.
func (h ListJumbleEntries) Execute(
	ctx context.Context, actor appshared.ActorContext, query repository.Query,
) (repository.Page, error) {
	w := h.Writer

	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     EntryReadAction,
		TokenScope: itemsReadScope,
		TargetType: entryTarget,
	}); err != nil {
		return repository.Page{}, err
	}

	var page repository.Page
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var listErr error
		page, listErr = w.Entries.List(ctx, query)
		return listErr
	})
	if err != nil {
		return repository.Page{}, err
	}
	return page, nil
}

// record writes the trail entry for a settlement or an arrival. The channel and the state - never
// the subject, the body or the sender (rule 10).
func (w Writer) record(
	ctx context.Context, actor appshared.ActorContext,
	action audit.Action, entry domain.Entry, at time.Time,
) {
	if w.Audit == nil {
		return
	}
	changes := []audit.Change{
		{Field: "channel", Classification: audit.Open, To: entry.Channel.String()},
		{Field: "status", Classification: audit.Open, To: string(entry.Status)},
	}
	if !entry.TargetItemID.IsZero() {
		changes = append(changes, audit.Change{
			Field: "target_item_id", Classification: audit.Open, To: entry.TargetItemID.String(),
		})
	}
	_ = w.Audit.Append(ctx, audit.Entry{
		TenantID:   entry.TenantID,
		OccurredAt: at,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: entryTarget,
		TargetID:   entry.ID,
		Changes:    audit.Changes(changes...),
	})
}

// entryOutput is the projection every channel answers from.
func entryOutput(entry domain.Entry) usecase.Output {
	attachments := make([]any, 0, len(entry.Attachments))
	for _, id := range entry.Attachments {
		attachments = append(attachments, id.String())
	}

	out := usecase.Output{
		"id":             entry.ID.String(),
		"channel":        entry.Channel.String(),
		"attachments":    attachments,
		"status":         string(entry.Status),
		"received_at":    entry.ReceivedAt,
		"target_item_id": nil,
		"settled_at":     nil,
	}
	if entry.Sender != "" {
		out["sender"] = entry.Sender
	}
	if entry.RawSubject != "" {
		out["raw_subject"] = entry.RawSubject
	}
	if entry.RawBody != "" {
		out["raw_body"] = entry.RawBody
	}
	if !entry.TargetItemID.IsZero() {
		out["target_item_id"] = entry.TargetItemID.String()
	}
	if entry.SettledAt != nil {
		out["settled_at"] = *entry.SettledAt
	}
	return out
}
