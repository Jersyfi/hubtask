// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// NewItemCreated announces a task, a work package or an activity that now exists.
//
// One event for all three levels rather than one per level. A subscriber that wanted only
// activities filters on the payload's `type`, and the day a fifth level is configured it reaches
// that subscriber without anybody adding a subscription for an event name nobody had heard of
// (ADR-0006, domain-model.md §4).
//
// The payload is a snapshot, in the API's field names, for the reasons given at
// NewContainerCreated: a consumer that had to fetch the item would produce a request per event
// and read a state that has already moved on, and a webhook payload should describe the object in
// the same words a REST response does.
//
// `parent_id` is sent even when it is null - it is the `parentRef` domain-model.md §4 asks for,
// and a consumer reading it to place the item in a tree needs the field present rather than
// inferred from the type. `collection_id` travels beside it because the parent alone does not say
// which collection an item belongs to until the whole chain has been walked.
func NewItemCreated(id shared.ID, item work.WorkItem, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return NewEnvelope(id, ItemCreated, item.TenantID,
		ItemSubject(item.ID), actor, occurredAt, cause, itemPayload(item))
}

// NewItemUpdated announces that an item's own fields changed: what was renamed, what was noted.
//
// The one item event that is not a snapshot, and deliberately. domain-model.md §4 gives this event a
// `changeSet` - old and new per field - rather than the object, and the difference is not a saving of
// bytes: an update touches one field of an item whose other fields are somebody's notes, so a snapshot
// on every rename would copy the whole of them into every subscriber's log, every time, for a change
// that did not involve them. What did not change is not announced.
//
// Both sides of each field travel, because that is what a field change trigger is written against: a
// rule fires on "the title became X" or on "it stopped being Y", and a payload carrying only the new
// value can answer the first question but never the second.
//
// Enough of the item travels beside the change set for a consumer to place it - which collection it is
// in, and which version the change produced - and no more.
func NewItemUpdated(id shared.ID, item work.WorkItem, changes []work.FieldChange, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if len(changes) == 0 {
		// An event that announces nothing changed. The writer does not write when nothing moved, so
		// reaching this means the two disagree - a defect rather than something a client sent
		// (security.md §9).
		return Envelope{}, shared.ErrInternal.WithDetail("events.change_set_empty")
	}

	changeSet := make(map[string]any, len(changes))
	for _, change := range changes {
		changeSet[change.Field] = map[string]any{"from": change.From, "to": change.To}
	}

	return NewEnvelope(id, ItemUpdated, item.TenantID,
		ItemSubject(item.ID), actor, occurredAt, cause, map[string]any{
			"id":            item.ID.String(),
			"type":          string(item.Type),
			"collection_id": item.CollectionID.String(),
			"updated_at":    item.UpdatedAt.UTC(),
			"version":       item.Version,
			"change_set":    changeSet,
		})
}

// NewItemCompleted announces that an item is done.
//
// Announced for a roll-up exactly as for a person's click. A consumer that needs to know which it was
// reads the causation chain - the roll-up's event is caused by the child's - rather than a second event
// type, because a separate name for an automatic completion would make every rule that reacts to "done"
// subscribe to two of them and forget one.
//
// The payload is the whole item rather than just the completion, for the reason NewItemCreated gives: a
// consumer that had to fetch the item would produce a request per event and read a state that has
// already moved on. `completion.completed_at` and `completion.completed_by` are the two fields
// domain-model.md §4 names for this event, and they are inside the completion object where a REST
// response also carries them.
func NewItemCompleted(id shared.ID, item work.WorkItem, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if !item.Completion.IsCompleted {
		// The event would say the opposite of its own name. A defect in whatever built it, not something
		// a client did (security.md §9).
		return Envelope{}, shared.ErrInternal.WithDetail("events.completion_inconsistent")
	}
	return NewEnvelope(id, ItemCompleted, item.TenantID,
		ItemSubject(item.ID), actor, occurredAt, cause, itemPayload(item))
}

// NewItemReopened announces that a completed item is open again.
func NewItemReopened(id shared.ID, item work.WorkItem, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if item.Completion.IsCompleted {
		return Envelope{}, shared.ErrInternal.WithDetail("events.completion_inconsistent")
	}
	return NewEnvelope(id, ItemReopened, item.TenantID,
		ItemSubject(item.ID), actor, occurredAt, cause, itemPayload(item))
}

// Movement is where an item came from, for an event about where it now is.
//
// The "from" half cannot be read off the item afterwards - the item is the new state - so it is passed. The
// "to" half is not: it is in the snapshot, and a payload that carried both would be two places for the same
// value to be wrong.
type Movement struct {
	FromParentID shared.ID
	// FromPath is the item's materialised path before the move. It is what lets a client rewrite its own copy
	// of the subtree without being sent an event per descendant: every descendant's new path is its old one
	// with FromPath swapped for the item's current Path (I-W2).
	FromPath string
	// FromCollectionID is set when the move crossed collections, which is what decides whether a device
	// subscribed to one hub still sees the item at all (offline-sync.md §3.1).
	FromCollectionID shared.ID
}

// NewItemMoved announces that an item sits somewhere else.
//
// One event for reparenting, for a change of collection and for a reorder, because domain-model.md §4 gives
// movement one event and names `orderKey` in its payload. A drag within a list and a drag between lists are
// the same gesture to a person and the same event to a rule; a consumer that cares only about reparenting
// compares the two parent identifiers.
//
// `from_bucket_id` and `to_bucket_id` are in the documented payload and are null here: buckets arrive with
// B-09, and a field invented before the thing it names would be a promise nothing keeps.
func NewItemMoved(id shared.ID, item work.WorkItem, from Movement, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	payload := itemPayload(item)
	payload["from_parent_id"] = nil
	payload["to_parent_id"] = nil
	payload["from_path"] = from.FromPath
	payload["from_collection_id"] = nil
	payload["from_bucket_id"] = nil
	payload["to_bucket_id"] = nil

	if !from.FromParentID.IsZero() {
		payload["from_parent_id"] = from.FromParentID.String()
	}
	if !item.ParentID.IsZero() {
		payload["to_parent_id"] = item.ParentID.String()
	}
	if !from.FromCollectionID.IsZero() && from.FromCollectionID != item.CollectionID {
		payload["from_collection_id"] = from.FromCollectionID.String()
	}

	return NewEnvelope(id, ItemMoved, item.TenantID,
		ItemSubject(item.ID), actor, occurredAt, cause, payload)
}

// itemPayload is the snapshot every item event carries - created, completed, reopened and moved - in the
// API's field names, so that a webhook payload and a REST response describe the same object in the same
// words.
//
// One builder rather than one per event. Copies of this map would drift in the way that matters least to
// whoever changes one and most to a subscriber reading them all.
func itemPayload(item work.WorkItem) map[string]any {
	payload := map[string]any{
		"id":            item.ID.String(),
		"type":          string(item.Type),
		"collection_id": item.CollectionID.String(),
		"parent_id":     nil,
		"path":          item.Path,
		"depth":         item.Depth,
		"title":         item.Title,
		"completion":    completionPayload(item.Completion),
		"order_key":     item.OrderKey,
		"created_at":    item.CreatedAt.UTC(),
		"created_by":    item.CreatedBy.String(),
		// updated_at travels because the completion events are about a change, and a consumer ordering
		// two changes to the same item needs to know when each happened. It is on the created event too:
		// one builder, and a field that appeared on two of three events would be a field a subscriber
		// cannot rely on.
		"updated_at": item.UpdatedAt.UTC(),
		"version":    item.Version,
	}
	if !item.ParentID.IsZero() {
		payload["parent_id"] = item.ParentID.String()
	}
	if item.Notes != "" {
		payload["notes"] = item.Notes
	}
	return payload
}

// completionPayload is the done/open state as the API spells it (schema Completion).
//
// Always sent, even on a create where it is always open. An automation rule that reacts to items
// and reads `completion.is_completed` should not need a special case for the one event where the
// field happens to be absent.
func completionPayload(completion work.Completion) map[string]any {
	payload := map[string]any{
		"is_completed": completion.IsCompleted,
		"completed_at": nil,
		"completed_by": nil,
	}
	if completion.CompletedAt != nil {
		payload["completed_at"] = completion.CompletedAt.UTC()
	}
	if !completion.CompletedBy.IsZero() {
		payload["completed_by"] = completion.CompletedBy.String()
	}
	return payload
}

// ItemSubject is what an item event is about. Kept next to the event so that the two cannot
// drift: a consumer filtering on the subject and a producer writing it read the same line.
func ItemSubject(id shared.ID) string { return "item/" + id.String() }

// NewItemLabelAdded announces that an entry now carries a label (domain-model.md §4).
//
// The payload is the reference rather than a snapshot of the item, which is the one place this
// system does not send one. Two reasons, and they are the same reason: a set is not a field. An
// item snapshot would have to carry the whole set to be useful, and the set is exactly what merges
// separately (offline-sync.md §4.2) - so a consumer reading it out of an event would be reading a
// value that another device may already have merged differently. `label_id` is what a rule reacts
// to, and `item_id` is what it reads the rest from.
func NewItemLabelAdded(id shared.ID, item work.WorkItem, labelID shared.ID, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return newItemLabelChange(id, ItemLabelAdded, item, labelID, actor, occurredAt, cause)
}

// NewItemLabelRemoved announces that an entry no longer carries a label.
func NewItemLabelRemoved(id shared.ID, item work.WorkItem, labelID shared.ID, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return newItemLabelChange(id, ItemLabelRemoved, item, labelID, actor, occurredAt, cause)
}

func newItemLabelChange(id shared.ID, eventType Type, item work.WorkItem, labelID shared.ID,
	actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if labelID.IsZero() {
		// An event about no label at all. Nothing a client sent could have caused it, so it is a
		// defect rather than input (security.md §9).
		return Envelope{}, shared.ErrInternal.WithDetail("events.label_missing")
	}

	return NewEnvelope(id, eventType, item.TenantID,
		ItemSubject(item.ID), actor, occurredAt, cause, map[string]any{
			"item_id":       item.ID.String(),
			"collection_id": item.CollectionID.String(),
			"label_id":      labelID.String(),
		})
}

// NewAttachmentAdded announces that an entry now carries a file (domain-model.md §4).
//
// The payload is the reference, exactly as the label events are and for the same reason: an
// attachment is a set element rather than a field, and an entry snapshot carrying the whole set
// would be a value another device may already have merged differently. `media_id` is what a rule
// and the media garbage collection react to; `item_id` is what they read the rest from.
func NewAttachmentAdded(id shared.ID, item work.WorkItem, mediaID shared.ID, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return newAttachmentChange(id, AttachmentAdded, item, mediaID, actor, occurredAt, cause)
}

// NewAttachmentRemoved announces that an entry no longer carries a file.
func NewAttachmentRemoved(id shared.ID, item work.WorkItem, mediaID shared.ID, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return newAttachmentChange(id, AttachmentRemoved, item, mediaID, actor, occurredAt, cause)
}

func newAttachmentChange(id shared.ID, eventType Type, item work.WorkItem, mediaID shared.ID,
	actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if mediaID.IsZero() {
		// An event about no file at all. Nothing a client sent could have caused it, so it is a
		// defect rather than input (security.md §9).
		return Envelope{}, shared.ErrInternal.WithDetail("events.media_missing")
	}

	return NewEnvelope(id, eventType, item.TenantID,
		ItemSubject(item.ID), actor, occurredAt, cause, map[string]any{
			"item_id":       item.ID.String(),
			"collection_id": item.CollectionID.String(),
			"media_id":      mediaID.String(),
		})
}

// NewItemAssigned announces that an entry is on somebody (domain-model.md §4).
//
// The payload is the reference §4 names rather than a snapshot of the entry, which is what the four
// events of this task have in common with the label pair. For the members the reason is the label
// pair's verbatim - a set is not a field, and a snapshot would carry one another device may already
// have merged differently. For the assignee it is the second half of the same sentence: what a
// notification reacts to is who it is now, `item_id` is what it reads the rest from, and an entry
// snapshot would have to say something about the member list beside it.
func NewItemAssigned(id shared.ID, item work.WorkItem, assigneeID shared.ID, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return newItemAssignment(id, ItemAssigned, item, assigneeID, actor, occurredAt, cause)
}

// NewItemAutoAssigned is NewItemAssigned with the strategy that chose the person, which is what
// the payload's `strategy` key means (domain-model.md §4): a rule reacting to assignments can
// tell a decision somebody made from one a policy made, and a person reading a notification can
// be told why the entry landed on them.
func NewItemAutoAssigned(id shared.ID, item work.WorkItem, assigneeID shared.ID,
	strategy work.AutoAssignStrategy, actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	envelope, err := newItemAssignment(id, ItemAssigned, item, assigneeID, actor, occurredAt, cause)
	if err != nil {
		return Envelope{}, err
	}
	envelope.Payload["strategy"] = string(strategy)
	return envelope, nil
}

// NewItemUnassigned announces that an entry is on nobody, and names who it was.
//
// The former assignee travels rather than a null, because that is what the event is about: a
// notification tells the person they have been taken off, and an event carrying nobody could only
// tell everybody or nobody at all.
func NewItemUnassigned(id shared.ID, item work.WorkItem, assigneeID shared.ID, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return newItemAssignment(id, ItemUnassigned, item, assigneeID, actor, occurredAt, cause)
}

func newItemAssignment(id shared.ID, eventType Type, item work.WorkItem, assigneeID shared.ID,
	actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if assigneeID.IsZero() {
		// An event about nobody. Both directions are about a person - the one who now carries the
		// entry, or the one who no longer does - so this is the writer and the event disagreeing,
		// which is a defect rather than something a client sent (security.md §9).
		return Envelope{}, shared.ErrInternal.WithDetail("events.assignee_missing")
	}

	return NewEnvelope(id, eventType, item.TenantID,
		ItemSubject(item.ID), actor, occurredAt, cause, map[string]any{
			"item_id":       item.ID.String(),
			"collection_id": item.CollectionID.String(),
			"assignee_id":   assigneeID.String(),
		})
}

// NewItemMemberAdded announces that an account is on an entry's member list (domain-model.md §4).
func NewItemMemberAdded(id shared.ID, item work.WorkItem, accountID shared.ID, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return newItemMemberChange(id, ItemMemberAdded, item, accountID, actor, occurredAt, cause)
}

// NewItemMemberRemoved announces that an account is off an entry's member list.
func NewItemMemberRemoved(id shared.ID, item work.WorkItem, accountID shared.ID, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return newItemMemberChange(id, ItemMemberRemoved, item, accountID, actor, occurredAt, cause)
}

func newItemMemberChange(id shared.ID, eventType Type, item work.WorkItem, accountID shared.ID,
	actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if accountID.IsZero() {
		// An event about no account at all, which is the label pair's `events.label_missing` one
		// set over: a defect rather than input.
		return Envelope{}, shared.ErrInternal.WithDetail("events.account_missing")
	}

	return NewEnvelope(id, eventType, item.TenantID,
		ItemSubject(item.ID), actor, occurredAt, cause, map[string]any{
			"item_id":       item.ID.String(),
			"collection_id": item.CollectionID.String(),
			"account_id":    accountID.String(),
		})
}

// The lifecycle events: archived, trashed, restored, purged.
//
// All but the last carry the ordinary item snapshot with the stamp that moved added to it. The
// stamp is not in itemPayload because it is not part of what every item event is about - the same
// reasoning that puts `from_parent_id` on the move alone - and a field that appeared on some
// snapshots and not others would be one a subscriber cannot rely on.

// NewItemArchived announces that an entry is kept and read-only.
func NewItemArchived(id shared.ID, item work.WorkItem, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if item.ArchivedAt == nil {
		// The event would say the opposite of its own name. A defect in whatever built it, not
		// something a client did (security.md §9).
		return Envelope{}, shared.ErrInternal.WithDetail("events.lifecycle_inconsistent")
	}
	return NewEnvelope(id, ItemArchived, item.TenantID,
		ItemSubject(item.ID), actor, occurredAt, cause, lifecyclePayload(item))
}

// NewItemUnarchived announces that an archived entry is writable again.
func NewItemUnarchived(id shared.ID, item work.WorkItem, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if item.ArchivedAt != nil {
		return Envelope{}, shared.ErrInternal.WithDetail("events.lifecycle_inconsistent")
	}
	return NewEnvelope(id, ItemUnarchived, item.TenantID,
		ItemSubject(item.ID), actor, occurredAt, cause, lifecyclePayload(item))
}

// NewItemTrashed announces that an entry and everything under it are in the trash.
//
// `subtree_size` is how many rows went, the entry itself included. A consumer cannot derive it -
// the descendants get no event of their own - and it is the number that tells a client whether the
// deletion it just made was the small thing it looked like.
func NewItemTrashed(id shared.ID, item work.WorkItem, subtreeSize int, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if item.DeletedAt == nil {
		return Envelope{}, shared.ErrInternal.WithDetail("events.lifecycle_inconsistent")
	}
	payload := lifecyclePayload(item)
	payload["subtree_size"] = subtreeSize
	return NewEnvelope(id, ItemTrashed, item.TenantID,
		ItemSubject(item.ID), actor, occurredAt, cause, payload)
}

// NewItemRestored announces that a deletion has been reversed, whole.
func NewItemRestored(id shared.ID, item work.WorkItem, subtreeSize int, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if item.DeletedAt != nil {
		return Envelope{}, shared.ErrInternal.WithDetail("events.lifecycle_inconsistent")
	}
	payload := lifecyclePayload(item)
	payload["subtree_size"] = subtreeSize
	return NewEnvelope(id, ItemRestored, item.TenantID,
		ItemSubject(item.ID), actor, occurredAt, cause, payload)
}

// Purge is what is left to say about an entry that no longer exists.
//
// Passed as a value rather than read off a WorkItem, because by the time this event is built the
// row is gone and the aggregate that described it is a copy nobody should be encouraged to keep.
// The fields are the ones a consumer needs in order to clean up after the entry: which one, of what
// type, where it was, and why it went.
type Purge struct {
	TenantID     shared.ID
	ItemID       shared.ID
	Type         work.ItemType
	CollectionID shared.ID
	// Path is what a consumer holding derived data below the entry matches on: everything under it
	// went too, and the prefix is how that is expressed without an event per row (I-W2).
	Path string
	// Reason says whether a person asked for this or a retention rule did. An operator reading a
	// cleanup log needs to be able to tell those apart, and a consumer may want to react to only one.
	//
	// The lifecycle context's vocabulary rather than one of this package's own: the journal, this
	// event and the metric are asking the same question, and two enumerations would eventually
	// answer it with two different words.
	Reason lifecycle.DeletionReason
}

// NewItemPurged announces that an entry is gone for good.
//
// No title, no notes, no snapshot. A purge that left the entry's content in an event stream would
// not be a deletion, and the consumers this event exists for - media garbage collection, the search
// index, vector stores - need an identifier and a path rather than the thing that was deleted
// (ADR-0018, data-retention.md §5).
func NewItemPurged(id shared.ID, purge Purge, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return NewEnvelope(id, ItemPurged, purge.TenantID,
		ItemSubject(purge.ItemID), actor, occurredAt, cause, map[string]any{
			"id":            purge.ItemID.String(),
			"type":          string(purge.Type),
			"collection_id": purge.CollectionID.String(),
			"path":          purge.Path,
			"purged_at":     occurredAt.UTC(),
			"reason":        string(purge.Reason),
		})
}

// lifecyclePayload is the item snapshot with both lifecycle stamps on it.
//
// Both, on every lifecycle event, rather than only the one that moved: an entry can be archived and
// in the trash at once (domain-model.md §3.4 draws that edge), so a consumer reading only the stamp
// that changed would not be able to tell what state the entry is now in.
func lifecyclePayload(item work.WorkItem) map[string]any {
	payload := itemPayload(item)
	payload["archived_at"] = nil
	payload["deleted_at"] = nil
	payload["trash_batch_id"] = nil
	if item.ArchivedAt != nil {
		payload["archived_at"] = item.ArchivedAt.UTC()
	}
	if item.DeletedAt != nil {
		payload["deleted_at"] = item.DeletedAt.UTC()
	}
	if !item.TrashBatchID.IsZero() {
		payload["trash_batch_id"] = item.TrashBatchID.String()
	}
	return payload
}
