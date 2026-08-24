// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
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
	UpdateWorkItemName = "UpdateWorkItem"

	// ItemUpdatedAction is the audit code. Stable: an auditor filters on it and a SIEM rule matches on
	// it (audit.md §2).
	ItemUpdatedAction audit.Action = "item.updated"
)

// UpdateWorkItem changes an item's own fields: what it is called, and what is noted on it.
//
// The capability profile is the gate. Notes on an ACTIVITY come back as `capability_not_supported`
// naming the type and the capability rather than being quietly dropped - silent ignoring is how a
// client comes to believe it stored something (domain-model.md §2, ADR-0006).
//
// What it does not change: where the item sits (MoveWorkItem), whether it is done
// (CompleteWorkItem), which labels it carries (AddLabel - a set is not a field), and the fields
// whose use cases arrive later, the due date and the assignee. A single endpoint that wrote all of
// them would need one audit entry covering everything and one event nobody could subscribe to
// narrowly.
type UpdateWorkItem struct {
	Items      repository.Items
	Buckets    repository.Buckets
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
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

// UpdateCommand is the input, typed.
type UpdateCommand struct {
	ItemID shared.ID
	// Attributes carries a pointer per field, so that "set it to nothing" and "do not touch it" stay
	// two different requests all the way down from the merge patch that expressed them.
	Attributes domain.ItemAttributes
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read none
	// and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// Execute applies the update and returns the item as it now stands.
func (h UpdateWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateCommand,
) (domain.WorkItem, error) {
	if cmd.ItemID.IsZero() {
		return domain.WorkItem{}, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}

	// The item and its collection are read before the permission question, because the answer depends
	// on the path: a membership at the hub applies downwards, and a path naming only the collection
	// would refuse somebody who does hold the right (domain-model.md §3.2). Nothing read here is
	// trusted afterwards - the state that decides the write is read again inside the transaction.
	subject, collection, err := h.collectionOf(ctx, actor, cmd.ItemID)
	if err != nil {
		return domain.WorkItem{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     ItemUpdatedAction,
		TokenScope: itemsWrite,
		TargetType: itemTarget,
		TargetID:   cmd.ItemID,
		On:         changing(subject),
	}); err != nil {
		return domain.WorkItem{}, err
	}

	var updated domain.WorkItem
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := h.Clock.Now()

		item, err := findItem(ctx, h.Items, cmd.ItemID)
		if err != nil {
			return err
		}
		collection, err := findCollection(ctx, h.Containers, item.CollectionID)
		if err != nil {
			return err
		}
		// I-C3: an archived or trashed collection is read-only, and its items inherit that.
		if err := collection.EnsureAcceptsItems(); err != nil {
			return err
		}
		profile, err := profileOf(ctx, h.Profiles, item.Type)
		if err != nil {
			return err
		}

		if cmd.Attributes.BucketID != nil && !cmd.Attributes.BucketID.IsZero() {
			if err := ensureBucketOnBoard(ctx, h.Buckets, item.CollectionID, *cmd.Attributes.BucketID); err != nil {
				return err
			}
		}

		wanted, changes, err := item.Updated(cmd.Attributes, profile, now)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			// The item already says what the caller asked it to say. Nothing is written, no version is
			// spent and nothing is announced - which is what makes a client that echoes the whole object
			// back harmless rather than merely accepted.
			//
			// The If-Match is still honoured: a caller writing against a version somebody else has moved
			// on is told so even when its own change would have been a no-op, because the state it was
			// reasoning about is not the state that is there.
			if err := ensureExpectedVersion(item, cmd.ExpectedVersion); err != nil {
				return err
			}
			updated = item
			return nil
		}

		updated, err = h.write(ctx, actor, wanted, changes, profile, cmd.ExpectedVersion, item.Version, now)
		return err
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return updated, nil
}

// write stores the item and records what the change owes: the event outwards, the change log for
// offline clients, and the audit entry - all inside the caller's transaction (test AT-5).
func (h UpdateWorkItem) write(
	ctx context.Context, actor appshared.ActorContext, after domain.WorkItem,
	changes []domain.FieldChange, profile domain.CapabilityProfile,
	expectedVersion, currentVersion int, now time.Time,
) (domain.WorkItem, error) {
	expected := expectedVersion
	if expected == 0 {
		// The caller read no version and accepts whatever is there. Not the same as skipping the check:
		// the version in hand is still the one the update matches on, so a concurrent write between the
		// read and here is still caught.
		expected = currentVersion
	}
	if err := h.Items.SetAttributes(ctx, after, expected); err != nil {
		return domain.WorkItem{}, err
	}
	after.Version = expected + 1

	// Built from the stored state rather than from the command, so that what the event says and what
	// the row holds cannot disagree.
	announcement, err := event.NewItemUpdated(
		h.IDs.NewID(), after, changes,
		event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return domain.WorkItem{}, err
	}
	if err := h.Events.Append(ctx, announcement); err != nil {
		return domain.WorkItem{}, err
	}
	if err := h.recordChanges(ctx, after, actor, changes); err != nil {
		return domain.WorkItem{}, err
	}
	if err := h.recordAudit(ctx, after, actor, changes, now); err != nil {
		return domain.WorkItem{}, err
	}
	if err := h.recordActivity(ctx, after, actor, changes, profile, now); err != nil {
		return domain.WorkItem{}, err
	}
	return after, nil
}

// recordActivity writes the step of the entry's own history.
//
// Unlike the audit trail, this one keeps the two titles: a rename with both sides hashed out would
// be an entry saying "something changed", which is what the trail is for and not what a person
// opening the history is looking for (audit.md §1). The note is the other way round - it is kept as
// "changed", because a note is a page of text and its history is that somebody edited it.
func (h UpdateWorkItem) recordActivity(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	changes []domain.FieldChange, profile domain.CapabilityProfile, now time.Time,
) error {
	changeSet := activity.ChangeSet(historyForm(profile), historyFields(changes)...)
	return h.Activity.record(ctx, actor, item, activity.ItemUpdated, changeSet, now)
}

// recordChanges writes what an offline client has to be told: one entry per field that moved.
//
// One entry per field rather than one carrying them all, because the merge rule for these fields is
// last writer wins *per field* (offline-sync.md §4.2). Each entry takes its own HLC, so a device
// that renamed an item while another wrote notes on it keeps both changes - which is precisely what
// one entry covering both would destroy, the later HLC deciding the whole payload and silently
// discarding the other device's field.
//
// The payload names only the field that moved. `version` and `updated_at` are derived and never
// merged, and a payload that repeated the untouched fields would let a stale value for one of them
// win a merge it should never have been in.
func (h UpdateWorkItem) recordChanges(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext, changes []domain.FieldChange,
) error {
	for _, change := range changes {
		err := h.Changes.Record(ctx, changelog.Change{
			TenantID:    item.TenantID,
			Entity:      itemTarget,
			EntityID:    item.ID,
			Op:          changelog.Upsert,
			ContainerID: item.CollectionID,
			ActorID:     actor.AccountID,
			HLC:         h.HLC.Next(),
			Payload:     map[string]any{change.Field: change.To},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// recordAudit writes the evidence: which fields changed, never what they now say.
//
// Both fields are user content and are classified SENSITIVE, so the trail records `changed: true`
// with a hash of each side (audit.md §4). The entry outlives the item by design, so a title kept in
// clear text here would be a copy that no deletion of that item ever reaches - and "who renamed
// this, and when" is answerable without it (rule 10, ADR-0017, ADR-0018).
func (h UpdateWorkItem) recordAudit(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	changes []domain.FieldChange, now time.Time,
) error {
	recorded := make([]audit.Change, 0, len(changes)+2)
	for _, change := range changes {
		recorded = append(recorded, audit.Change{
			Field: change.Field, Classification: audit.Sensitive,
			From: change.From, To: change.To,
		})
	}
	recorded = append(recorded,
		audit.Change{Field: "type", Classification: audit.Open, To: string(item.Type)},
		audit.Change{
			Field: "collection_id", Classification: audit.Open,
			To: item.CollectionID.String(),
		})

	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   item.TenantID,
		OccurredAt: now,
		Action:     ItemUpdatedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: itemTarget,
		TargetID:   item.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(recorded...),
	})
}

// collectionOf reads the collection an item lives in, read-only and outside the write transaction,
// because the permission check needs it first.
func (h UpdateWorkItem) collectionOf(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) (domain.WorkItem, domain.Container, error) {
	return readItemScope(ctx, h.UnitOfWork, h.Items, h.Containers, actor, itemID)
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h UpdateWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateWorkItemName,
		Summary: "Changes an item's own fields: its title, its notes where the type carries them, " +
			"and the language they are written in. " +
			"A field that is not sent is left alone; sending `notes` as null clears it. Idempotent: an " +
			"update that asks for what is already stored succeeds, writes nothing and announces nothing. " +
			"Writing notes to a type whose capability profile does not carry NOTES - an activity - is " +
			"refused rather than ignored.",
		SideEffects: "Writes the changed fields, announces " + string(event.ItemUpdated) +
			" with a change set, records one change per field for offline clients, and writes an audit entry.",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The item to change.",
			},
			{
				Name: "title", Kind: usecase.KindString,
				Description: "The new title: one line, at most 500 characters, not empty. Omitted leaves " +
					"the title as it is.",
			},
			{
				Name: "notes", Kind: usecase.KindString,
				Description: "The new notes, as Markdown; the server does not render them. Empty clears " +
					"them, omitted leaves them as they are. Only on a type whose capability profile " +
					"carries NOTES - on an activity this is refused.",
			},
			{
				Name: "bucket_id", Kind: usecase.KindID,
				Description: "The column of the collection's board to put the entry in. Empty takes " +
					"it off the board, omitted leaves it where it is. The column has to be on this " +
					"collection's board, and only a type whose capability profile carries BUCKET " +
					"has one - a board belongs to a collection, so only the entries directly in it " +
					"have a place on it.",
			},
			{
				Name: "content_language", Kind: usecase.KindString,
				Description: "The language the entry is written in, as a BCP 47 tag. Empty clears " +
					"the statement, omitted leaves it as it is. The entry is re-indexed under the " +
					"new configuration on this write; entries whose language did not change are " +
					"left alone.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read, from the If-Match header over REST. Omitted means the " +
					"caller read none and accepts whatever is there; a version that has moved on since is " +
					"refused rather than overwritten.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemUpdatedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemUpdated},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// invoke is the adapter between the catalogue's untyped input and the typed command, for all three
// channels at once.
func (h UpdateWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}

	// OptionalString rather than String, because the difference between the two is this use case's
	// whole contract: a caller that sent no `notes` wants them left alone, and one that sent an empty
	// `notes` wants them gone. A channel that could not say which would clear the notes of every
	// client that only meant to rename something.
	cmd := UpdateCommand{
		ItemID: itemID,
		Attributes: domain.ItemAttributes{
			Title: in.OptionalString("title"), Notes: in.OptionalString("notes"),
			ContentLanguage: in.OptionalString("content_language"),
		},
		ExpectedVersion: in.Int("expected_version"),
	}
	// The board is read by presence for the reason the text fields are read by pointer: an empty
	// `bucket_id` takes the entry off the board, and an absent one leaves it where it is.
	if in.Present("bucket_id") {
		bucketID, err := in.ID("bucket_id")
		if err != nil {
			return nil, err
		}
		cmd.Attributes.BucketID = &bucketID
	}
	if cmd.Attributes.IsEmpty() {
		return nil, shared.ErrValidation.
			WithDetail("items.update_empty").
			WithFields(shared.FieldError{Path: "/", Code: "items.update_empty"})
	}

	item, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return itemOutput(item), nil
}
