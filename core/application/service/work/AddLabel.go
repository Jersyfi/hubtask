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
	AddLabelName    = "AddLabel"
	RemoveLabelName = "RemoveLabel"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	ItemLabelAddedAction   audit.Action = "item.label_added"
	ItemLabelRemovedAction audit.Action = "item.label_removed"
)

// ItemLabelWriter is what both use cases share.
//
// The tokens are the item's rather than the container's, unlike the label itself: defining a
// collection's vocabulary is structure, and tagging an entry with a word from it is work. Somebody
// who may edit entries may label them, and somebody who may not must not be able to reach around
// through this endpoint (security.md §5, domain-model.md §3.2).
type ItemLabelWriter struct {
	Items      repository.Items
	ItemLabels repository.ItemLabels
	Labels     repository.Labels
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
	Authorizer Authorizer
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
}

// AddLabel puts a label on an entry.
type AddLabel struct {
	Writer ItemLabelWriter
}

// RemoveLabel takes a label off an entry.
type RemoveLabel struct {
	Writer ItemLabelWriter
}

// LabelCommand is the input of both directions, typed.
//
// No expected version. A label lives beside the entry rather than on its row, so there is no
// version of the entry that an addition would be racing - which is also why neither operation
// spends one. Two devices adding two different labels at once is the case the OR-set exists to
// serve, and an optimistic lock on the entry would make one of them fail for no reason
// (offline-sync.md §4.2).
type LabelCommand struct {
	ItemID  shared.ID
	LabelID shared.ID
}

// ItemLabelSet is the labels an entry carries, as every channel reports them.
type ItemLabelSet struct {
	ItemID   shared.ID
	LabelIDs []shared.ID
}

// Execute puts the label on the entry and returns the labels it now carries.
func (h AddLabel) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd LabelCommand,
) (ItemLabelSet, error) {
	return h.Writer.change(ctx, actor, cmd, adding)
}

// Execute takes the label off the entry and returns the labels it now carries.
func (h RemoveLabel) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd LabelCommand,
) (ItemLabelSet, error) {
	return h.Writer.change(ctx, actor, cmd, removing)
}

// labelDirection is which way the caller asked for. Not a boolean: `change(ctx, actor, cmd, true)`
// at a call site says nothing, and this is the parameter that decides which of two audit trails is
// written.
type labelDirection bool

const (
	adding   labelDirection = true
	removing labelDirection = false
)

func (d labelDirection) action() audit.Action {
	if d == adding {
		return ItemLabelAddedAction
	}
	return ItemLabelRemovedAction
}

// change is the whole of both use cases.
func (w ItemLabelWriter) change(
	ctx context.Context, actor appshared.ActorContext, cmd LabelCommand, want labelDirection,
) (ItemLabelSet, error) {
	if cmd.ItemID.IsZero() {
		return ItemLabelSet{}, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}
	if cmd.LabelID.IsZero() {
		return ItemLabelSet{}, labelIDRequired()
	}

	// The entry and its collection are read before the permission question, because the answer
	// depends on the path: a membership held at the hub applies downwards (domain-model.md §3.2).
	// Nothing read here is trusted afterwards - the state that decides the write is read again
	// inside the transaction.
	collection, err := w.readCollectionOf(ctx, actor, cmd.ItemID)
	if err != nil {
		return ItemLabelSet{}, err
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
	}); err != nil {
		return ItemLabelSet{}, err
	}

	var result ItemLabelSet
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		item, err := findItem(ctx, w.Items, cmd.ItemID)
		if err != nil {
			return err
		}
		if err := w.ensureLabelAllowed(ctx, item); err != nil {
			return err
		}
		// A trashed or archived entry is read-only (I-W4). Asked after the capability, exactly as
		// EnsureCompletable does and for the same reason: "an activity has no labels" is true of
		// the type whatever state one particular activity is in, and answering with the state first
		// would send a client off to unarchive an entry whose labels would still be refused.
		if err := item.EnsureEditable(); err != nil {
			return err
		}
		if err := w.ensureLabelInCollection(ctx, item, cmd.LabelID); err != nil {
			return err
		}

		now := w.Clock.Now()
		// The tag is the clock reading the OR-set merges on. It is taken here rather than derived
		// from `now`, because a merge orders changes against other devices' readings and a wall
		// clock cannot do that (offline-sync.md §4.1).
		tag := w.HLC.Next()

		changed, err := w.apply(ctx, cmd, want, tag)
		if err != nil {
			return err
		}

		carried, err := w.ItemLabels.List(ctx, cmd.ItemID)
		if err != nil {
			return err
		}
		result = ItemLabelSet{ItemID: cmd.ItemID, LabelIDs: carried}

		if !changed {
			// The entry already carries the label, or already does not. The tag has been written
			// all the same - a device that decided this has made a decision another replica has to
			// merge against - but nothing is announced, because nothing about the entry changed.
			return nil
		}
		return w.announce(ctx, actor, item, collection, cmd.LabelID, want, tag, now)
	})
	if err != nil {
		return ItemLabelSet{}, err
	}
	return result, nil
}

// apply writes the membership and the tag, and reports whether the set actually moved.
func (w ItemLabelWriter) apply(
	ctx context.Context, cmd LabelCommand, want labelDirection, tag shared.HLC,
) (bool, error) {
	if want == removing {
		return w.ItemLabels.Remove(ctx, cmd.ItemID, cmd.LabelID, tag)
	}

	carried, err := w.ItemLabels.List(ctx, cmd.ItemID)
	if err != nil {
		return false, err
	}
	for _, id := range carried {
		if id == cmd.LabelID {
			// Already carried. The addition is written anyway, so that the tag moves forward and a
			// concurrent removal on another device does not win a merge it should not - but nothing
			// is announced, because the set did not move.
			return false, w.ItemLabels.Add(ctx, cmd.ItemID, cmd.LabelID, tag)
		}
	}
	return true, w.ItemLabels.Add(ctx, cmd.ItemID, cmd.LabelID, tag)
}

// announce records what a change to an entry's labels owes: the event outwards, the change log for
// offline clients, and the audit entry - all inside the caller's transaction (test AT-5).
func (w ItemLabelWriter) announce(
	ctx context.Context, actor appshared.ActorContext, item domain.WorkItem,
	collection domain.Container, labelID shared.ID, want labelDirection,
	tag shared.HLC, now time.Time,
) error {
	by := event.Actor{Kind: actor.Kind, ID: actor.AccountID}

	announcement, err := event.NewItemLabelAdded(w.IDs.NewID(), item, labelID, by, now, event.Cause{})
	if want == removing {
		announcement, err = event.NewItemLabelRemoved(
			w.IDs.NewID(), item, labelID, by, now, event.Cause{})
	}
	if err != nil {
		return err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return err
	}
	if err := w.recordChange(ctx, item, collection, actor, labelID, want, tag); err != nil {
		return err
	}
	return w.recordAudit(ctx, item, actor, labelID, want, now)
}

// recordChange writes what an offline client has to be told.
//
// The payload is the one element that moved and the tag that decides it, not the whole set. That is
// the merge rule for a set, written down: an entry carrying `["a"]` and one carrying `["b"]` merge
// to both, and a payload that named the whole array would let the later of the two writers erase
// the other's label (offline-sync.md §4.2). The entry's own HLC is not this entry's business -
// `set` names which set the element belongs to, so that members and watchers can use the same
// shape when they arrive.
func (w ItemLabelWriter) recordChange(
	ctx context.Context, item domain.WorkItem, collection domain.Container,
	actor appshared.ActorContext, labelID shared.ID, want labelDirection, tag shared.HLC,
) error {
	operation := "add"
	if want == removing {
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
			"set":        string(domain.SetLabels),
			"element_id": labelID.String(),
			"op":         operation,
		},
	})
}

// recordAudit writes the evidence.
//
// The label is recorded by identifier rather than by name: a label's name is user content and stays
// out of the trail, and an identifier is what an auditor needs in order to ask which label it was
// (rule 10, audit.md §4).
func (w ItemLabelWriter) recordAudit(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	labelID shared.ID, want labelDirection, now time.Time,
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
			audit.Change{Field: "label_id", Classification: audit.Open, To: labelID.String()},
			audit.Change{
				Field: "collection_id", Classification: audit.Open, To: item.CollectionID.String(),
			},
		),
	})
}

// ensureLabelAllowed refuses a type whose profile does not carry LABELS.
//
// An activity has none (domain-model.md §2). Refused rather than silently ignored, which is the
// rule the capability matrix states: a client that tagged an activity and received a 200 would
// believe the tag is there.
func (w ItemLabelWriter) ensureLabelAllowed(ctx context.Context, item domain.WorkItem) error {
	profile, err := profileOf(ctx, w.Profiles, item.Type)
	if err != nil {
		return err
	}
	return profile.Require(domain.CapabilityLabels, "/label_id")
}

// ensureLabelInCollection refuses a label from another collection.
//
// Invariant I-W6: a label is a vocabulary one collection agreed on, and one from elsewhere would be
// a reference that resolves to a word nobody in this collection chose. A deleted label is refused
// for the same reason - it is no longer in the vocabulary, and a client that added one would be
// tagging an entry with something no read will ever show.
func (w ItemLabelWriter) ensureLabelInCollection(
	ctx context.Context, item domain.WorkItem, labelID shared.ID,
) error {
	label, err := findLabel(ctx, w.Labels, labelID)
	if err != nil {
		return err
	}
	if label.CollectionID != item.CollectionID {
		return shared.ErrValidation.
			WithDetail("labels.not_in_collection").
			WithParams(map[string]string{
				"label_id": labelID.String(), "collection_id": item.CollectionID.String(),
			}).
			WithFields(shared.FieldError{Path: "/label_id", Code: "labels.not_in_collection"})
	}
	return label.EnsureEditable()
}

// readCollectionOf reads the collection an entry belongs to, outside the write transaction, because
// the permission check needs its path first. Read-only, so it may be served by a replica
// (multi-tenancy.md §7).
func (w ItemLabelWriter) readCollectionOf(
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

// labelSetOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema ItemLabels).
func labelSetOutput(set ItemLabelSet) usecase.Output {
	ids := make([]string, 0, len(set.LabelIDs))
	for _, id := range set.LabelIDs {
		ids = append(ids, id.String())
	}
	return usecase.Output{"item_id": set.ItemID.String(), "label_ids": ids}
}

// Descriptor is the catalogue entry.
func (h AddLabel) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AddLabelName,
		Summary: "Puts a label on an entry. The label has to belong to the entry's own collection " +
			"- a label is a vocabulary one collection agreed on - and the entry's type has to " +
			"carry labels at all: an activity does not. Idempotent: an entry that already carries " +
			"the label succeeds and announces nothing.",
		SideEffects: "Writes the label and its merge tag, announces " +
			string(event.ItemLabelAdded) + ", records the change for offline clients, and writes " +
			"an audit entry.",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry to label.",
			},
			{
				Name: "label_id", Kind: usecase.KindID, Required: true,
				Description: "A label of the entry's own collection.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemLabelAddedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemLabelAdded},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h AddLabel) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := labelCommandOf(in)
	if err != nil {
		return nil, err
	}

	set, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return labelSetOutput(set), nil
}

// Descriptor is the catalogue entry.
func (h RemoveLabel) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RemoveLabelName,
		Summary: "Takes a label off an entry. Idempotent: an entry that does not carry it succeeds " +
			"and announces nothing. The removal is recorded either way, so that a device which " +
			"never saw the label added still merges the decision.",
		SideEffects: "Removes the label, writes its merge tag, announces " +
			string(event.ItemLabelRemoved) + ", records the change for offline clients, and writes " +
			"an audit entry.",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry to take the label off.",
			},
			{
				Name: "label_id", Kind: usecase.KindID, Required: true,
				Description: "The label to remove.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemLabelRemovedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemLabelRemoved},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h RemoveLabel) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := labelCommandOf(in)
	if err != nil {
		return nil, err
	}

	set, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return labelSetOutput(set), nil
}

// labelCommandOf is the adapter between the catalogue's untyped input and the typed command, for
// both directions and all three channels.
func labelCommandOf(in usecase.Input) (LabelCommand, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return LabelCommand{}, err
	}
	labelID, err := in.ID("label_id")
	if err != nil {
		return LabelCommand{}, err
	}
	return LabelCommand{ItemID: itemID, LabelID: labelID}, nil
}
