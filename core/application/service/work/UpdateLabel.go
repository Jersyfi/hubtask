// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	UpdateLabelName = "UpdateLabel"
	DeleteLabelName = "DeleteLabel"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	LabelUpdatedAction audit.Action = "label.updated"
	LabelDeletedAction audit.Action = "label.deleted"
)

// LabelWriter is what both use cases that change an existing label share.
//
// One dependency set rather than two: they read the same label, ask the same permission question of
// the same collection, and owe the same four writes. What differs is which domain method decides
// the new state and what the change log calls it.
type LabelWriter struct {
	Labels     repository.Labels
	Containers repository.Containers
	Authorizer Authorizer
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
}

// UpdateLabel changes a label's own fields.
type UpdateLabel struct {
	Writer LabelWriter
}

// DeleteLabel takes a label out of a collection's vocabulary.
//
// A soft delete, and the entries that carried it are not rewritten: the read side filters on the
// label's own stamp, which is what makes the deletion undoable without having to remember who wore
// it. That is the difference from a deleted column, whose entries have to go somewhere - a chip
// that is no longer in the vocabulary simply stops being rendered, while an entry in a column that
// is no longer on the board would be nowhere at all.
type DeleteLabel struct {
	Writer LabelWriter
}

// UpdateLabelCommand is the input, typed.
type UpdateLabelCommand struct {
	LabelID shared.ID
	// Attributes carries a pointer per field, so that "set it to nothing" and "do not touch it"
	// stay two different requests all the way down from the merge patch that expressed them.
	Attributes domain.LabelAttributes
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read
	// none and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// DeleteLabelCommand is the input, typed.
type DeleteLabelCommand struct {
	LabelID         shared.ID
	ExpectedVersion int
}

// Execute changes the label and returns it as it now stands.
func (h UpdateLabel) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateLabelCommand,
) (domain.Label, error) {
	return h.Writer.change(ctx, actor, labelChange{
		labelID:         cmd.LabelID,
		action:          LabelUpdatedAction,
		expectedVersion: cmd.ExpectedVersion,
		operation:       changelog.Upsert,
		apply: func(label domain.Label, _ time.Time) (domain.Label, []domain.FieldChange, error) {
			return label.Updated(cmd.Attributes)
		},
		store: repository.Labels.SetAttributes,
		announce: func(id shared.ID, label domain.Label, changes []domain.FieldChange,
			by event.Actor, at time.Time,
		) (event.Envelope, error) {
			return event.NewLabelUpdated(id, label, changes, by, at, event.Cause{})
		},
		// The name and the description are user content, so the trail records that they changed and
		// a hash of each side rather than the values. The colour travels with them and is not - but
		// one classification decides the whole entry, and the colour is in the event for anyone who
		// needs to read it (rule 10, audit.md §4).
		classification: audit.Sensitive,
	})
}

// Execute deletes the label and returns it as it now stands.
func (h DeleteLabel) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd DeleteLabelCommand,
) (domain.Label, error) {
	return h.Writer.change(ctx, actor, labelChange{
		labelID:         cmd.LabelID,
		action:          LabelDeletedAction,
		expectedVersion: cmd.ExpectedVersion,
		operation:       changelog.Delete,
		apply: func(label domain.Label, now time.Time) (domain.Label, []domain.FieldChange, error) {
			return label.Deleted(now)
		},
		store: repository.Labels.SetDeleted,
		announce: func(id shared.ID, label domain.Label, _ []domain.FieldChange,
			by event.Actor, at time.Time,
		) (event.Envelope, error) {
			return event.NewLabelDeleted(id, label, by, at, event.Cause{})
		},
		// A deletion stamp is a timestamp this server produced. There is no personal data in it,
		// and "when did this label go" is precisely what an auditor asks.
		classification: audit.Open,
	})
}

// labelChange is one verb's differences from the other: what it applies, what it stores, what it
// announces, what the change log calls it, and how sensitive what it changed is.
type labelChange struct {
	labelID         shared.ID
	action          audit.Action
	expectedVersion int
	operation       changelog.Operation
	apply           func(domain.Label, time.Time) (domain.Label, []domain.FieldChange, error)
	store           func(repository.Labels, context.Context, domain.Label, int) error
	announce        func(shared.ID, domain.Label, []domain.FieldChange, event.Actor, time.Time) (event.Envelope, error)
	classification  audit.Classification
}

// change is the whole of what a change to an existing label owes, once.
func (w LabelWriter) change(
	ctx context.Context, actor appshared.ActorContext, change labelChange,
) (domain.Label, error) {
	if change.labelID.IsZero() {
		return domain.Label{}, labelIDRequired()
	}

	// The label and its collection are read before the permission question, because the answer
	// depends on the path: a membership held at the hub applies downwards (domain-model.md §3.2).
	// Nothing read here is trusted afterwards.
	collection, err := w.readCollectionOf(ctx, actor, change.labelID)
	if err != nil {
		return domain.Label{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       containerPath(collection),
		Action:     change.action,
		TokenScope: labelsWrite,
		TargetType: labelTarget,
		TargetID:   change.labelID,
	}); err != nil {
		return domain.Label{}, err
	}

	var updated domain.Label
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		label, collection, err := w.readInTransaction(ctx, change.labelID)
		if err != nil {
			return err
		}
		// I-C3: an archived collection is read-only, and so is one whose hub is archived. Re-checked
		// inside the transaction rather than trusted from the read above.
		if err := collection.EnsureEditable(); err != nil {
			return err
		}

		now := w.Clock.Now()
		wanted, changes, err := change.apply(label, now)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			// The label already says what the caller asked it to say - or is already deleted.
			// Nothing is written, no version is spent and nothing is announced, which is what makes
			// a retry after a lost response harmless.
			//
			// The If-Match is still honoured: a caller writing against a version somebody else has
			// moved on is told so even when its own change would have been a no-op.
			if err := ensureLabelVersion(label, change.expectedVersion); err != nil {
				return err
			}
			updated = label
			return nil
		}

		updated, err = w.write(ctx, actor, change, wanted, collection, changes, label.Version, now)
		return err
	})
	if err != nil {
		return domain.Label{}, err
	}
	return updated, nil
}

// write stores the change and records what it owes: the event outwards, the change log for offline
// clients, and the audit entry - all inside the caller's transaction (test AT-5).
func (w LabelWriter) write(
	ctx context.Context, actor appshared.ActorContext, change labelChange,
	after domain.Label, collection domain.Container, changes []domain.FieldChange,
	currentVersion int, now time.Time,
) (domain.Label, error) {
	expected := change.expectedVersion
	if expected == 0 {
		// The caller read no version and accepts whatever is there. Not the same as skipping the
		// check: the version in hand is still what the update matches on.
		expected = currentVersion
	}
	if err := change.store(w.Labels, ctx, after, expected); err != nil {
		return domain.Label{}, err
	}
	after.Version = expected + 1

	announcement, err := change.announce(
		w.IDs.NewID(), after, changes, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now)
	if err != nil {
		return domain.Label{}, err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return domain.Label{}, err
	}
	if err := w.recordChanges(ctx, after, collection, actor, change, changes); err != nil {
		return domain.Label{}, err
	}
	if err := w.recordAudit(ctx, after, actor, change, changes, now); err != nil {
		return domain.Label{}, err
	}
	return after, nil
}

// recordChanges writes what an offline client has to be told.
//
// A deletion is one entry with no payload: there is nothing left to describe, and a tombstone with
// content would be a copy of the deleted object living on in the log (offline-sync.md §7).
//
// An update is one entry per field that moved, because the merge rule for these fields is last
// writer wins *per field* (offline-sync.md §4.2). Each entry takes its own HLC, so a device that
// renamed a label while another recoloured it keeps both changes - which is precisely what one
// entry covering both would destroy.
func (w LabelWriter) recordChanges(
	ctx context.Context, label domain.Label, collection domain.Container,
	actor appshared.ActorContext, change labelChange, changes []domain.FieldChange,
) error {
	// The visibility filter a pull applies: the hub above the collection, so that a device
	// subscribed to the hub sees the change (offline-sync.md §3.1).
	containerID := firstNonZero(collection.ParentID, label.CollectionID)

	if change.operation == changelog.Delete {
		return w.Changes.Record(ctx, changelog.Change{
			TenantID: label.TenantID, Entity: labelTarget, EntityID: label.ID,
			Op: changelog.Delete, ContainerID: containerID,
			ActorID: actor.AccountID, HLC: w.HLC.Next(),
		})
	}

	for _, moved := range changes {
		err := w.Changes.Record(ctx, changelog.Change{
			TenantID: label.TenantID, Entity: labelTarget, EntityID: label.ID,
			Op: changelog.Upsert, ContainerID: containerID,
			ActorID: actor.AccountID, HLC: w.HLC.Next(),
			Payload: map[string]any{moved.Field: clearedAsNull(moved.To)},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// recordAudit writes the evidence: which fields changed, and - where they are not user content -
// what they now say.
func (w LabelWriter) recordAudit(
	ctx context.Context, label domain.Label, actor appshared.ActorContext,
	change labelChange, changes []domain.FieldChange, now time.Time,
) error {
	recorded := make([]audit.Change, 0, len(changes)+1)
	for _, moved := range changes {
		recorded = append(recorded, audit.Change{
			Field: moved.Field, Classification: change.classification,
			From: moved.From, To: moved.To,
		})
	}
	recorded = append(recorded, audit.Change{
		Field: "collection_id", Classification: audit.Open, To: label.CollectionID.String(),
	})

	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   label.TenantID,
		OccurredAt: now,
		Action:     change.action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: labelTarget,
		TargetID:   label.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(recorded...),
	})
}

// readCollectionOf reads the collection a label belongs to, outside the write transaction, because
// the permission check needs its path first. Read-only, so it may be served by a replica.
func (w LabelWriter) readCollectionOf(
	ctx context.Context, actor appshared.ActorContext, labelID shared.ID,
) (domain.Container, error) {
	var collection domain.Container

	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		_, found, err := w.readInTransaction(ctx, labelID)
		collection = found
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	return collection, nil
}

// readInTransaction reads the label and the collection it belongs to.
func (w LabelWriter) readInTransaction(
	ctx context.Context, labelID shared.ID,
) (domain.Label, domain.Container, error) {
	label, err := findLabel(ctx, w.Labels, labelID)
	if err != nil {
		return domain.Label{}, domain.Container{}, err
	}
	collection, err := findCollection(ctx, w.Containers, label.CollectionID)
	if err != nil {
		return domain.Label{}, domain.Container{}, err
	}
	return label, collection, nil
}

// findLabel reads a label a client named, or says it does not exist in the words a client can act
// on.
func findLabel(
	ctx context.Context, labels repository.Labels, id shared.ID,
) (domain.Label, error) {
	label, err := labels.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// The same answer whether it does not exist or belongs to another tenant
			// (multi-tenancy.md §2).
			return domain.Label{}, shared.ErrNotFound.
				WithDetail("labels.not_found").
				WithParams(map[string]string{"label_id": id.String()})
		}
		return domain.Label{}, err
	}
	return label, nil
}

func labelIDRequired() error {
	return shared.ErrValidation.
		WithDetail("labels.label_id_required").
		WithFields(shared.FieldError{Path: "/label_id", Code: "labels.label_id_required"})
}

// ensureLabelVersion refuses a caller writing against a version that has moved on, even when the
// change it asked for would have been a no-op.
func ensureLabelVersion(label domain.Label, expected int) error {
	if expected == 0 || expected == label.Version {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("labels.version_conflict").
		WithParams(map[string]string{
			"label_id": label.ID.String(), "current_version": strconv.Itoa(label.Version),
		})
}

// Descriptor is the catalogue entry.
func (h UpdateLabel) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateLabelName,
		Summary: "Changes a label's own fields: what it is called, what colour it is, what it " +
			"means. A field that is not sent is left alone; sending the description as empty " +
			"clears it. The colour cannot be cleared - a label is rendered as a chip and nothing " +
			"else. Idempotent: an update that asks for what is already stored succeeds, writes " +
			"nothing and announces nothing.",
		SideEffects: "Writes the changed fields, announces " + string(event.LabelUpdated) +
			" with a change set, records one change per field for offline clients, and writes an " +
			"audit entry.",
		TokenScope: labelsWrite,
		Input: []usecase.Field{
			{
				Name: "label_id", Kind: usecase.KindID, Required: true,
				Description: "The label to change.",
			},
			{
				Name: "name", Kind: usecase.KindString,
				Description: "The new name: one line, at most 120 characters, not empty, and free " +
					"in this collection. Omitted leaves the name as it is.",
			},
			{
				Name: "color_token", Kind: usecase.KindString,
				Description: "The new theme token. It may not be empty; omitted leaves it as it is.",
			},
			{
				Name: "description", Kind: usecase.KindString,
				Description: "What the label means. Empty clears it, omitted leaves it as it is.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read, from the If-Match header over REST.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: LabelUpdatedAction, TargetType: labelTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a collection's vocabulary is its configuration. Deleting a label does not rewrite " +
				"the entries that carried it, so nothing happened to any of them.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h UpdateLabel) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	labelID, err := in.ID("label_id")
	if err != nil {
		return nil, err
	}

	// The optional readers rather than the plain ones: a caller that sent no `description` wants it
	// left alone, and one that sent an empty `description` wants it gone.
	cmd := UpdateLabelCommand{
		LabelID: labelID,
		Attributes: domain.LabelAttributes{
			Name:        in.OptionalString("name"),
			ColorToken:  in.OptionalString("color_token"),
			Description: in.OptionalString("description"),
		},
		ExpectedVersion: in.Int("expected_version"),
	}
	if cmd.Attributes.IsEmpty() {
		return nil, shared.ErrValidation.
			WithDetail("labels.update_empty").
			WithFields(shared.FieldError{Path: "/", Code: "labels.update_empty"})
	}

	label, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return labelOutput(label), nil
}

// Descriptor is the catalogue entry.
func (h DeleteLabel) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteLabelName,
		Summary: "Takes a label out of a collection's vocabulary. The entries that carried it are " +
			"not rewritten: they stop showing it because it is gone, which is what makes the " +
			"deletion undoable without having to remember who wore it. Idempotent: deleting a " +
			"label that is already gone succeeds and announces nothing.",
		SideEffects: "Writes the deletion stamp, announces " + string(event.LabelDeleted) +
			", records a deletion for offline clients, and writes an audit entry.",
		TokenScope: labelsWrite,
		Input: []usecase.Field{
			{
				Name: "label_id", Kind: usecase.KindID, Required: true,
				Description: "The label to take out of the vocabulary.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read, from the If-Match header over REST.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: LabelDeletedAction, TargetType: labelTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a collection's vocabulary is its configuration. Deleting a label does not rewrite " +
				"the entries that carried it, so nothing happened to any of them.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DeleteLabel) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	labelID, err := in.ID("label_id")
	if err != nil {
		return nil, err
	}

	label, err := h.Execute(ctx, actor, DeleteLabelCommand{
		LabelID:         labelID,
		ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return labelOutput(label), nil
}
