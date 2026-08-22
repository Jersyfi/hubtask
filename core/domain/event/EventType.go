// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package event holds the domain events: what happened, in a form other systems can subscribe to.
//
// An event is a public contract (domain-model.md §4). Automation rules, webhook subscribers, n8n
// and Zapier read it, which is why the type name carries a version and why fields may only be
// added - removing or reinterpreting one needs a `.v2` alongside continued delivery of `.v1`.
//
// The wire format is CloudEvents 1.0 (ADR-0007), but that mapping lives in the adapter. What is
// here is the event itself: its type, who caused it, and the business payload.
package event

// Type is the event type, `de.hubtask.<context>.<entity>.<action>.v<n>` (domain-model.md §4).
//
// The context segment is `work` for work management, matching the examples in the schema and in
// automation.md §1. The backlog entry for A-07 wrote `workmanagement`; the shorter form was
// confirmed as the contract, and the backlog was corrected rather than the two documents.
type Type string

const (
	// ContainerCreated announces a new hub or collection. Consumers: automation, webhooks, search.
	ContainerCreated Type = "de.hubtask.work.container.created.v1"
	// ContainerRenamed announces that a container's own descriptive fields changed. Consumers:
	// automation, webhooks, search.
	ContainerRenamed Type = "de.hubtask.work.container.renamed.v1"
	// ContainerPoliciesUpdated announces that a collection now works differently - today, whether a
	// child's completion propagates upwards. Consumers: automation, the roll-up.
	//
	// Its own type rather than a rename carrying a different field, because the two are different
	// decisions: a rule that reacts to a collection being reconfigured must not fire when somebody
	// corrects a typo in its name. domain-model.md §4 names neither, and this is the narrower of the
	// two names it could have had.
	ContainerPoliciesUpdated Type = "de.hubtask.work.container.policies_updated.v1"
	// ContainerMoved announces that a collection sits in a different hub, or at a different rank in
	// the same one. Consumers: automation, search, SSE.
	//
	// The counterpart of ItemMoved, and one type for both reasons as that one is: a drag within a
	// level and a drag between hubs are the same gesture to a person.
	ContainerMoved Type = "de.hubtask.work.container.moved.v1"
	// ContainerArchived announces that a container is read-only, and everything under it with it
	// (I-C3). Consumers: automation, webhooks, search.
	ContainerArchived Type = "de.hubtask.work.container.archived.v1"
	// ContainerUnarchived announces that a container is writable again.
	//
	// Its own type rather than the `.restored` domain-model.md §4 pairs with `.archived`: that name
	// belongs to the trash, and a rule written to react to something coming back from a deletion must
	// not fire when somebody unarchives a hub. The same reasoning separates ItemReopened from
	// ItemCompleted.
	ContainerUnarchived Type = "de.hubtask.work.container.unarchived.v1"
	// ContainerDeleted announces that a container and everything under it are in the trash: a soft
	// delete waiting out its retention period (I-C2). Consumers: automation, webhooks, search, SSE.
	//
	// The subtree gets no event per row. The payload carries the batch every row of the deletion
	// shares, which is what a consumer needs to know that "everything under this" went with it - and
	// the alternative, an event per deleted entry, would put a hub's two hundred entries into every
	// subscriber's log for one act.
	//
	// `.deleted` rather than `.trashed`, which is the name domain-model.md §4 gives the container
	// half of this. The item half is called `.trashed` there, and the two names are kept as the
	// document writes them rather than harmonised: a public event name is a contract, and the place
	// to change it is the document.
	ContainerDeleted Type = "de.hubtask.work.container.deleted.v1"
	// ContainerRestored announces that a container's deletion has been reversed, whole. Consumers:
	// automation, webhooks, search, SSE.
	//
	// Distinct from ContainerUnarchived for the reason given there: a rule written to react to
	// something coming back from a deletion must not fire when somebody unarchives a hub.
	ContainerRestored Type = "de.hubtask.work.container.restored.v1"
	// ItemCreated announces a new task, work package or activity. One type for all three levels,
	// because they are one aggregate (ADR-0006): a subscriber filters on the payload's `type`
	// rather than on three event names, and a fifth level reaches it without a new subscription.
	// Consumers: automation, search, SSE.
	ItemCreated Type = "de.hubtask.work.item.created.v1"
	// ItemUpdated announces that an item's own fields changed: what was renamed, what was noted.
	// Consumers: automation (field change triggers are the second commonest there is), the activity
	// history, search, SSE.
	//
	// Its own type rather than a field on a general "changed" event, and separate from moved and
	// completed, because a rule that reacts to a rename must not fire when somebody drags a card
	// between lists. The three answer different questions about the same object.
	ItemUpdated Type = "de.hubtask.work.item.updated.v1"
	// ItemCompleted announces that an item is done. Consumers: automation (the commonest trigger there
	// is), the roll-up itself, ON_COMPLETION recurrence, search, SSE.
	//
	// Announced for a roll-up as much as for a person's click, and a consumer cannot tell the two apart
	// from the type - it reads the causation chain for that. The alternative, a separate event for an
	// automatic completion, would make every rule that reacts to "done" subscribe to two names and
	// forget one.
	ItemCompleted Type = "de.hubtask.work.item.completed.v1"
	// ItemReopened announces that a completed item is open again. Its own type rather than a field on
	// the completed event: a rule that reacts to work being finished must not fire when work is
	// unfinished, and a subscriber filtering on a payload field to avoid that is a subscriber that will
	// eventually not.
	ItemReopened Type = "de.hubtask.work.item.reopened.v1"
	// ItemMoved announces that an item sits somewhere else: under a different parent, in a different
	// collection, or at a different rank among the same siblings. Consumers: kanban automation, search, SSE.
	//
	// One type for all three, because domain-model.md §4 gives it one and names `orderKey` among its
	// payload: a drag within a list and a drag between lists are the same gesture to a person and the same
	// event to a rule. A consumer that cares only about reparenting compares `from_parent_id` with
	// `to_parent_id`.
	ItemMoved Type = "de.hubtask.work.item.moved.v1"
	// ItemAssigned announces that an entry is on somebody. Consumers: notification.
	//
	// The payload is the reference domain-model.md §4 names - `assigneeId` and, once C-02 lands, the
	// strategy that chose them - rather than a snapshot of the entry. What a rule and a notification
	// react to is who it is now, and `itemId` is what they read the rest from; an entry snapshot
	// would additionally have to say something about the member list, which merges separately and
	// which another device may already have merged differently (offline-sync.md §4.2).
	ItemAssigned Type = "de.hubtask.work.item.assigned.v1"
	// ItemUnassigned announces that an entry is on nobody, and names who it was.
	//
	// Its own type rather than an assignment naming nobody, on the reasoning that separates
	// ItemReopened from ItemCompleted: a rule that reacts to work being handed to somebody must not
	// fire when it is taken off them, and a subscriber filtering on a null payload field to avoid
	// that is a subscriber that will eventually not.
	ItemUnassigned Type = "de.hubtask.work.item.unassigned.v1"
	// ItemMemberAdded announces that an account is on an entry's member list. Consumers:
	// notification.
	//
	// The reference rather than a snapshot, for the reason given at ItemLabelAdded and in the same
	// words: a set is not a field. The member list merges as an OR-set, so a snapshot of the entry
	// would carry a set another device may already have merged differently; `account_id` is what a
	// rule reacts to and `item_id` is what it reads the rest from.
	ItemMemberAdded Type = "de.hubtask.work.item.member_added.v1"
	// ItemMemberRemoved announces that an account is off an entry's member list. Consumers:
	// notification.
	ItemMemberRemoved Type = "de.hubtask.work.item.member_removed.v1"
	// ItemArchived announces that an entry is kept and read-only. Consumers: automation, search
	// (an archived entry drops out of the default index), SSE.
	//
	// Separate from ItemTrashed, which is the distinction the whole lifecycle rests on: archiving is
	// a decision about how work is kept, and trashing is a deletion with a clock running against it.
	// A rule that tidies an archive must not fire on something on its way out of the system.
	ItemArchived Type = "de.hubtask.work.item.archived.v1"
	// ItemUnarchived announces that an archived entry is writable again.
	//
	// Its own type rather than the `.restored` domain-model.md §4 pairs with `.archived`, on exactly
	// the reasoning that separates ContainerUnarchived from ContainerRestored: `.restored` belongs to
	// the trash, and a rule reacting to something coming back from a deletion must not fire when
	// somebody unarchives an entry.
	ItemUnarchived Type = "de.hubtask.work.item.unarchived.v1"
	// ItemTrashed announces that an entry and everything under it are in the trash. Consumers:
	// automation, search, SSE, and anything holding a reference that is about to stop resolving.
	//
	// The subtree gets no event per row: the payload carries the batch and the path, and a consumer
	// that holds the subtree removes it under that prefix - the same contract ItemMoved uses for the
	// same reason (I-W2).
	ItemTrashed Type = "de.hubtask.work.item.trashed.v1"
	// ItemRestored announces that a deletion has been reversed, whole.
	ItemRestored Type = "de.hubtask.work.item.restored.v1"
	// ItemPurged announces that an entry is gone for good. Consumers: cleanup, media garbage
	// collection, search (the index entry has to go with it), vector stores.
	//
	// The one item event whose payload is not a snapshot, and it cannot be: the row it would
	// describe no longer exists. What it carries is what a consumer needs in order to clean up after
	// it - which entry, of what type, in which collection, and under which path - and nothing that
	// would amount to keeping a copy of the deleted entry in every subscriber's log. A purge that
	// left the title in an event stream would not be a deletion (ADR-0018).
	ItemPurged Type = "de.hubtask.work.item.purged.v1"
	// BucketCreated announces a new column on a collection's board. Consumers: kanban clients,
	// automation, search.
	//
	// domain-model.md §4 names no bucket event at all. It follows the scheme rather than a table
	// entry, because a board that could be rearranged without anything being announced would be a
	// hole in the contract exactly where a kanban client synchronises.
	BucketCreated Type = "de.hubtask.work.bucket.created.v1"
	// BucketUpdated announces that a column's own fields changed: what it is called, what it holds
	// at once, whether it means finished. Consumers: kanban clients, automation, search.
	BucketUpdated Type = "de.hubtask.work.bucket.updated.v1"
	// BucketReordered announces that a column sits elsewhere on its board.
	//
	// Its own type rather than an update carrying `order_key`, on the reasoning that separates
	// ItemMoved from ItemUpdated: a rule that reacts to a column being renamed must not fire when
	// somebody drags it one place to the left. A board has one dimension, so there is no move to
	// distinguish this from - a column cannot leave its collection.
	BucketReordered Type = "de.hubtask.work.bucket.reordered.v1"
	// BucketDeleted announces that a column is off the board, and says where its entries went.
	//
	// The destination is in the payload because a consumer cannot derive it: the entries moved to
	// the leftmost remaining column, and a kanban client that only learned the column was gone
	// would have to reload the board to find out where its cards are.
	BucketDeleted Type = "de.hubtask.work.bucket.deleted.v1"
	// LabelCreated announces a new label in a collection's vocabulary. Consumers: automation,
	// search, clients that render a chip.
	LabelCreated Type = "de.hubtask.work.label.created.v1"
	// LabelUpdated announces that a label's own fields changed: what it is called, what colour it
	// is, what it means. Consumers: automation, search, clients that render a chip.
	LabelUpdated Type = "de.hubtask.work.label.updated.v1"
	// LabelDeleted announces that a label is out of a collection's vocabulary.
	//
	// The entries that carried it are not named. A collection's vocabulary is small and its entries
	// are not, so listing them would make the payload unbounded; a consumer that renders chips
	// drops this label from all of them, which is what the deletion means.
	LabelDeleted Type = "de.hubtask.work.label.deleted.v1"
	// ItemLabelAdded announces that an entry now carries a label. Consumers: automation.
	//
	// One of the two events domain-model.md §4 names by hand, and the payload it names is `labelId`.
	// Its own type rather than an ItemUpdated carrying a set, because a set is not a field: it
	// merges as an OR-set rather than by last writer wins, and a rule written against "the labels
	// changed" could not tell an addition from a removal.
	ItemLabelAdded Type = "de.hubtask.work.item.label_added.v1"
	// ItemLabelRemoved announces that an entry no longer carries a label. Consumers: automation.
	ItemLabelRemoved Type = "de.hubtask.work.item.label_removed.v1"
)

// types is the closed set. Everything that needs to know which events exist reads it here - the
// event schemas under api/events/ are reconciled against it by the contract test, so an event
// without a schema, or a schema without an event, is a red build rather than a discovery made by
// a subscriber.
var types = [...]Type{
	ContainerCreated, ContainerRenamed, ContainerPoliciesUpdated, ContainerMoved,
	ContainerArchived, ContainerUnarchived, ContainerDeleted, ContainerRestored,
	ItemCreated, ItemUpdated, ItemCompleted, ItemReopened, ItemMoved,
	ItemAssigned, ItemUnassigned, ItemMemberAdded, ItemMemberRemoved,
	ItemArchived, ItemUnarchived, ItemTrashed, ItemRestored, ItemPurged,
	BucketCreated, BucketUpdated, BucketReordered, BucketDeleted,
	LabelCreated, LabelUpdated, LabelDeleted,
	ItemLabelAdded, ItemLabelRemoved,
}

// Types returns every defined event type.
func Types() []Type { return types[:] }

// Valid reports whether the type is one of the defined ones.
func (t Type) Valid() bool {
	for _, known := range types {
		if known == t {
			return true
		}
	}
	return false
}

func (t Type) String() string { return string(t) }
