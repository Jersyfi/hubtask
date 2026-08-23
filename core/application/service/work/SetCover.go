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
	SetCoverName   = "SetCover"
	ClearCoverName = "ClearCover"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	ItemCoverSetAction     audit.Action = "item.cover_set"
	ItemCoverClearedAction audit.Action = "item.cover_cleared"
)

// MediaReferences is the slice of the media record store this package needs: read one object, and
// move its reference counter.
//
// Narrow rather than the whole port, for the reason Visibility is narrow: a use case declaring a
// dependency it does not use is a use case whose test has to satisfy it anyway. The counter is the
// only thing this package writes about a media object - what the object is, and whether it may be
// used at all, is the media domain's answer (media.Object.Attachable).
type MediaReferences interface {
	Find(ctx context.Context, id shared.ID) (media.Object, error)
	AdjustRefCount(ctx context.Context, id shared.ID, delta int) error
}

// CoverWriter is what both directions of the cover share.
//
// One dependency set held by both use cases rather than two copies, for the reason AssignmentWriter
// is one: putting a cover on and taking it off are the same walk in opposite directions, and the
// only thing that differs is whether an image arrives or leaves.
type CoverWriter struct {
	Items      repository.Items
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
	Media      MediaReferences
	Authorizer Authorizer
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	Activity   ActivityJournal
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
}

// SetCover puts a cover on an entry: a colour token, or an image.
//
// Only a type whose profile carries COVER has one, which by default is a task and neither a work
// package nor an activity (domain-model.md §2, §3.4). Refused rather than quietly ignored: a client
// that covered a work package and received a 200 would believe the picture is there.
//
// Replacing a cover is this call again. The displaced image loses a reference and is left to the
// reconciliation job, which is the whole of the deletion path here - a request has no business
// waiting on a bucket for a picture somebody just swapped out.
type SetCover struct {
	Cover CoverWriter
}

// ClearCover takes the cover off an entry.
type ClearCover struct {
	Cover CoverWriter
}

// CoverCommand is the input of both directions, typed. The cover itself is empty for a clearing,
// which is the one part the two do not share.
type CoverCommand struct {
	ItemID     shared.ID
	Kind       domain.CoverKind
	ColorToken string
	MediaID    shared.ID
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read
	// none and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// Execute puts the cover on the entry and returns it.
func (h SetCover) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CoverCommand,
) (domain.WorkItem, error) {
	return h.Cover.change(ctx, actor, cmd, covering)
}

// Execute takes the cover off the entry and returns it.
func (h ClearCover) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CoverCommand,
) (domain.WorkItem, error) {
	return h.Cover.change(ctx, actor, cmd, uncovering)
}

// coverDirection is which way the caller asked for. Not a boolean, for the reason labelDirection is
// not: `change(ctx, actor, cmd, true)` at a call site says nothing, and this is the parameter that
// decides which of two audit trails is written.
type coverDirection bool

const (
	covering   coverDirection = true
	uncovering coverDirection = false
)

func (d coverDirection) action() audit.Action {
	if d == covering {
		return ItemCoverSetAction
	}
	return ItemCoverClearedAction
}

func (d coverDirection) verb() activity.Verb {
	if d == covering {
		return activity.ItemCoverSet
	}
	return activity.ItemCoverCleared
}

// change is the whole of both use cases.
func (w CoverWriter) change(
	ctx context.Context, actor appshared.ActorContext, cmd CoverCommand, want coverDirection,
) (domain.WorkItem, error) {
	if cmd.ItemID.IsZero() {
		return domain.WorkItem{}, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}

	// The entry and its collection are read before the permission question, because the answer
	// depends on the path: a membership held at the hub applies downwards (domain-model.md §3.2).
	// Nothing read here is trusted afterwards - the state that decides the write is read again
	// inside the transaction.
	subject, collection, err := readItemScope(
		ctx, w.UnitOfWork, w.Items, w.Containers, actor, cmd.ItemID)
	if err != nil {
		return domain.WorkItem{}, err
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
		return domain.WorkItem{}, err
	}

	var changed domain.WorkItem
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
		if err := item.EnsureCoverable(profile); err != nil {
			return err
		}

		wanted, err := w.wanted(ctx, item, cmd, want, now)
		if err != nil {
			return err
		}
		if sameCover(item.Cover, wanted.Cover) {
			// Already in the state asked for. Nothing is written, no version is spent and nothing
			// is announced - which is what makes a repeat harmless rather than merely accepted, and
			// what makes two devices setting the same cover converge on one version.
			//
			// The If-Match is still honoured: a caller that wrote against a version somebody else
			// has moved on is told so, even when its own change would have been a no-op, because
			// the state it was reasoning about is not the state that is there.
			if err := ensureExpectedVersion(item, cmd.ExpectedVersion); err != nil {
				return err
			}
			changed = item
			return nil
		}

		written, err := w.write(ctx, actor, item, wanted, cmd.ExpectedVersion, profile, want, now)
		changed = written
		return err
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return changed, nil
}

// wanted builds the state the caller asked for, and refuses an image the media context will not
// stand behind.
func (w CoverWriter) wanted(
	ctx context.Context, item domain.WorkItem, cmd CoverCommand, want coverDirection, now time.Time,
) (domain.WorkItem, error) {
	if want == uncovering {
		return item.Uncovered(now), nil
	}

	cover, err := domain.NewCover(cmd.Kind, cmd.ColorToken, cmd.MediaID)
	if err != nil {
		return domain.WorkItem{}, err
	}
	if cover.Kind == domain.CoverImage {
		if err := w.ensureImageUsable(ctx, cover.MediaID); err != nil {
			return domain.WorkItem{}, err
		}
	}
	return item.Covered(cover, now), nil
}

// ensureImageUsable refuses a media object that may not stand behind a cover.
//
// The judgement is the media domain's rather than this one's: READY, staged as a cover, and not on
// its way out (media.Object.Attachable). What this adds is the tenant, which is not a question at
// all - row level security answers it by not returning another tenant's row (ADR-0010).
func (w CoverWriter) ensureImageUsable(ctx context.Context, mediaID shared.ID) error {
	object, err := w.Media.Find(ctx, mediaID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return shared.ErrNotFound.
				WithDetail("media.not_found").
				WithParams(map[string]string{"media_id": mediaID.String()}).
				WithFields(shared.FieldError{Path: "/media_id", Code: "media.not_found"})
		}
		return err
	}
	return object.Attachable(media.UsageCover)
}

// write stores the cover and records what the change owes: the reference counters, the event
// outwards, the change log for offline clients, the audit entry, and the step of the entry's own
// history - all inside the caller's transaction (test AT-5).
func (w CoverWriter) write(
	ctx context.Context, actor appshared.ActorContext, before, after domain.WorkItem,
	expectedVersion int, profile domain.CapabilityProfile, want coverDirection, now time.Time,
) (domain.WorkItem, error) {
	expected := expectedVersion
	if expected == 0 {
		// The caller read no version and accepts whatever is there. Not the same as skipping the
		// check: the version in hand is still the one the update matches on, so a concurrent write
		// between the read and here is still caught.
		expected = before.Version
	}
	if err := w.Items.SetCover(ctx, after, expected); err != nil {
		return domain.WorkItem{}, err
	}
	after.Version = expected + 1

	if err := w.adjustReferences(ctx, before, after); err != nil {
		return domain.WorkItem{}, err
	}

	change := domain.FieldChange{
		Field: domain.FieldCover,
		From:  coverReference(before.Cover),
		To:    coverReference(after.Cover),
	}

	// An ItemUpdated carrying the field rather than a cover event of its own. The cover is a
	// scalar on the row - it merges by last writer wins per field, not as a set - so a rule
	// written against "this field changed" is exactly the subscription the catalogue offers for
	// it (domain-model.md §4, offline-sync.md §4.2).
	announcement, err := event.NewItemUpdated(
		w.IDs.NewID(), after, []domain.FieldChange{change},
		event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.recordChange(ctx, after, actor, change); err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.recordAudit(ctx, before, after, actor, change, want, now); err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.recordActivity(ctx, after, actor, change, profile, want, now); err != nil {
		return domain.WorkItem{}, err
	}
	return after, nil
}

// adjustReferences moves the counters the cover change owes: the arriving image gains one, the
// displaced one loses one.
//
// Both inside the write transaction, which is what makes the counter and the reference one fact
// rather than two that can disagree. What is *not* here is the deletion: an object that just lost
// its last reference is left to the reconciliation job, which is the ordering the schema's
// ON DELETE RESTRICT makes impossible to get wrong (data-protection.md §5).
func (w CoverWriter) adjustReferences(ctx context.Context, before, after domain.WorkItem) error {
	arrived, displaced := coverImage(after.Cover), coverImage(before.Cover)
	if arrived == displaced {
		return nil
	}

	if !arrived.IsZero() {
		if err := w.Media.AdjustRefCount(ctx, arrived, 1); err != nil {
			return err
		}
	}
	if !displaced.IsZero() {
		return w.Media.AdjustRefCount(ctx, displaced, -1)
	}
	return nil
}

// recordActivity writes the step of the entry's own history.
//
// The form is the type's, as every entry's is: only a type carrying COVER reaches this point, and
// whether its history keeps the values is the profile's answer rather than this call's
// (domain-model.md §2).
func (w CoverWriter) recordActivity(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	change domain.FieldChange, profile domain.CapabilityProfile, want coverDirection,
	now time.Time,
) error {
	changeSet := activity.ChangeSet(historyForm(profile), historyFields([]domain.FieldChange{change})...)
	return w.Activity.record(ctx, actor, item, want.verb(), changeSet, now)
}

// recordChange writes what an offline client has to be told.
//
// One entry naming one field, which is the merge rule for a scalar written down: the cover merges
// as last writer wins per field, so it takes an HLC of its own and carries nothing else
// (offline-sync.md §4.2). A clearing names the field as empty rather than leaving it out - an
// absent field means "not touched", and a device that read it that way would keep a cover somebody
// removed.
func (w CoverWriter) recordChange(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	change domain.FieldChange,
) error {
	return w.Changes.Record(ctx, changelog.Change{
		TenantID:    item.TenantID,
		Entity:      itemTarget,
		EntityID:    item.ID,
		Op:          changelog.Upsert,
		ContainerID: item.CollectionID,
		ActorID:     actor.AccountID,
		HLC:         w.HLC.Next(),
		Payload:     map[string]any{domain.FieldCover: change.To},
	})
}

// recordAudit writes the evidence.
//
// The cover is recorded in clear text on both sides, and it is not user content: a colour token is
// a name from the design system's vocabulary and a media identifier is an identifier. What is not
// here is the file name behind an image cover - that one is content, and rule 10 keeps it out.
func (w CoverWriter) recordAudit(
	ctx context.Context, before, after domain.WorkItem, actor appshared.ActorContext,
	change domain.FieldChange, want coverDirection, now time.Time,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   after.TenantID,
		OccurredAt: now,
		Action:     want.action(),
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: itemTarget,
		TargetID:   after.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{
				Field: domain.FieldCover, Classification: audit.Open,
				From: change.From, To: change.To,
			},
			audit.Change{Field: "type", Classification: audit.Open, To: string(after.Type)},
			audit.Change{
				Field: "collection_id", Classification: audit.Open,
				To: before.CollectionID.String(),
			},
		),
	})
}

// sameCover reports whether two covers say the same thing, absence included.
func sameCover(a, b *domain.Cover) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

// coverImage is the media object behind a cover, zero for a colour or for none.
func coverImage(cover *domain.Cover) shared.ID {
	if cover == nil || cover.Kind != domain.CoverImage {
		return ""
	}
	return cover.MediaID
}

// coverReference is how a cover travels through a change set: the token for a colour, the
// identifier for an image, and the empty string for none.
//
// One string rather than a nested object, because FieldChange carries strings and every other
// field in this system reaches an event, a change log entry and an audit entry through it. The kind
// is not lost - a colour token and a UUID are not confusable - and a consumer that needs the shape
// reads the entry rather than the change set.
func coverReference(cover *domain.Cover) string {
	switch {
	case cover == nil:
		return ""
	case cover.Kind == domain.CoverImage:
		return cover.MediaID.String()
	default:
		return cover.ColorToken
	}
}

// coverOutput is how a cover reaches a projection: the contract's Cover object, or an explicit
// null. Leaving the key out would say "this server does not know about covers", which is a
// different statement from "this entry has none".
func coverOutput(cover *domain.Cover) any {
	if cover == nil {
		return nil
	}
	return map[string]any{
		"kind":        string(cover.Kind),
		"color_token": textOrNull(cover.ColorToken),
		"media_id":    idOrNil(cover.MediaID),
	}
}

func textOrNull(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h SetCover) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetCoverName,
		Summary: "Puts a cover on an entry: a colour token from the design system's vocabulary, or " +
			"an image by its media identifier - exactly one of the two, matching the kind. Only a " +
			"type whose profile carries COVER has one, which by default is a task and neither a " +
			"work package nor an activity. An image has to be a READY media object staged as a " +
			"cover in this workspace. Replacing a cover is this call again; the displaced image " +
			"loses a reference and is reclaimed once nothing points at it. Idempotent: the same " +
			"cover again succeeds and announces nothing.",
		SideEffects: "Writes the cover, moves the media reference counters, announces " +
			string(event.ItemUpdated) + ", records the change for offline clients, writes an audit " +
			"entry and a step of the entry's history.",
		TokenScope: itemsWrite,
		Input: append(
			[]usecase.Field{
				{
					Name: "kind", Kind: usecase.KindString, Required: true,
					Description: "COLOR or IMAGE.",
				},
				{
					Name: "color_token", Kind: usecase.KindString,
					Description: "The colour, as a design system token rather than a value - " +
						"theming belongs to the client (ADR-0029). Exactly for a COLOR cover.",
				},
				{
					Name: "media_id", Kind: usecase.KindID,
					Description: "The image, as a READY media object of usage COVER. Exactly for " +
						"an IMAGE cover.",
				},
			},
			coverInput("The entry to cover.")...,
		),
		Audit: usecase.AuditDeclaration{
			Action: ItemCoverSetAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemCoverSet},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h ClearCover) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ClearCoverName,
		Summary: "Takes the cover off an entry. Idempotent: an entry with no cover succeeds and " +
			"announces nothing. An image that loses its last reference is reclaimed by the " +
			"reconciliation job rather than by this call.",
		SideEffects: "Clears the cover, lowers the media reference counter, announces " +
			string(event.ItemUpdated) + ", records the change for offline clients, writes an audit " +
			"entry and a step of the entry's history.",
		TokenScope: itemsWrite,
		Input:      coverInput("The entry to take the cover off."),
		Audit: usecase.AuditDeclaration{
			Action: ItemCoverClearedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemCoverCleared},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// coverInput is what both directions take beyond the cover itself. One list, so that a client which
// learned one of them from /meta/capabilities does not find the other spelled differently.
func coverInput(itemDescription string) []usecase.Field {
	return []usecase.Field{
		{Name: "item_id", Kind: usecase.KindID, Required: true, Description: itemDescription},
		{
			Name: "expected_version", Kind: usecase.KindInt,
			Description: "The version last read, from the If-Match header over REST. Omitted means " +
				"the caller read none and accepts whatever is there; a version that has moved on " +
				"since is refused rather than overwritten.",
		},
	}
}

func (h SetCover) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := coverCommand(in)
	if err != nil {
		return nil, err
	}
	cmd.Kind = domain.CoverKind(in.String("kind"))
	cmd.ColorToken = in.String("color_token")
	if named := in.String("media_id"); named != "" {
		mediaID, err := in.ID("media_id")
		if err != nil {
			return nil, err
		}
		cmd.MediaID = mediaID
	}

	item, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return itemOutput(item), nil
}

func (h ClearCover) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := coverCommand(in)
	if err != nil {
		return nil, err
	}

	item, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return itemOutput(item), nil
}

// coverCommand is the adapter between the catalogue's untyped input and the typed command, for both
// directions and all three channels.
func coverCommand(in usecase.Input) (CoverCommand, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return CoverCommand{}, err
	}
	return CoverCommand{ItemID: itemID, ExpectedVersion: in.Int("expected_version")}, nil
}
