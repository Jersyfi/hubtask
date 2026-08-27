// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

// ReadScope is the token scope that may read an event of this type (G-04).
//
// The pull half of the stream is authorised per event type, not per endpoint: a token scoped to
// read items polls item events and is refused audit events, and that falls out of the scopes the
// use case catalogue already declares rather than out of a polling permission invented beside
// them. An event says no more than the call that produced it would have, which is the only
// defensible answer - a second, looser way to the same facts is a way around the first.
//
// The scope is the *reading* one of whatever context owns the event's entity, because that is what
// a subscriber is doing. `de.hubtask.work.item.completed.v1` is answered to `items:read`, not to
// `items:write`: watching work being finished is not permission to finish it.
func (t Type) ReadScope() string { return readScopes[t] }

// itemsRead, containersRead, mediaRead and templatesRead repeat the values the application layer's
// use cases declare, and repeat them deliberately. core/domain may not import the application layer
// (ADR-0001), so the two statements cannot be one; what keeps them from drifting is
// TestEveryEventTypeIsReadableBySomeUseCasesScope in the architecture gate, which reads both sides
// and fails when a scope named here is a scope nothing serves.
const (
	itemsRead      = "items:read"
	containersRead = "containers:read"
	mediaRead      = "media:read"
	templatesRead  = "templates:read"
)

// readScopes is exhaustive over types, and TestEveryEventTypeHasAReadScope proves it. A map with a
// zero value for the missing entry would answer "" - a scope no token can hold and every poll is
// therefore refused for - which fails closed but fails silently, and a new event type nobody can
// subscribe to is a bug that only a subscriber would ever find.
var readScopes = map[Type]string{
	// A hub or a collection, and the two things that are configuration of one. Buckets and labels
	// are read with the container that holds them - CreateBucket and CreateLabel define their read
	// scope as containersRead for that reason, and an event about one says no more than the list
	// call would.
	ContainerCreated:         containersRead,
	ContainerRenamed:         containersRead,
	ContainerPoliciesUpdated: containersRead,
	ContainerMoved:           containersRead,
	ContainerArchived:        containersRead,
	ContainerUnarchived:      containersRead,
	ContainerDeleted:         containersRead,
	ContainerRestored:        containersRead,
	BucketCreated:            containersRead,
	BucketUpdated:            containersRead,
	BucketReordered:          containersRead,
	BucketDeleted:            containersRead,
	LabelCreated:             containersRead,
	LabelUpdated:             containersRead,
	LabelDeleted:             containersRead,

	// The entry itself, at every level: one aggregate, one scope (ADR-0006). What is on an entry -
	// who it is assigned to, who is on it, which labels it carries, what somebody wrote under it -
	// is read through the entry and is scoped with it.
	ItemCreated:       itemsRead,
	ItemUpdated:       itemsRead,
	ItemCompleted:     itemsRead,
	ItemReopened:      itemsRead,
	ItemMoved:         itemsRead,
	ItemAssigned:      itemsRead,
	ItemUnassigned:    itemsRead,
	ItemMemberAdded:   itemsRead,
	ItemMemberRemoved: itemsRead,
	ItemDueChanged:    itemsRead,
	ItemDueSoon:       itemsRead,
	ItemOverdue:       itemsRead,
	ItemArchived:      itemsRead,
	ItemUnarchived:    itemsRead,
	ItemTrashed:       itemsRead,
	ItemRestored:      itemsRead,
	ItemPurged:        itemsRead,
	ItemLabelAdded:    itemsRead,
	ItemLabelRemoved:  itemsRead,
	CommentCreated:    itemsRead,
	CommentUpdated:    itemsRead,
	CommentDeleted:    itemsRead,

	// An occurrence is an entry the scheduler created. The event announces the entry, so it is
	// scoped like one - `recurrence:write` is what defines a series, and defining one is not what
	// somebody watching occurrences appear is doing.
	RecurrenceOccurrenceCreated: itemsRead,

	// The file, not the entry it hangs on. An attachment event names a media object, and
	// `media:read` is what reads one.
	AttachmentAdded:   mediaRead,
	AttachmentRemoved: mediaRead,

	// What was stamped out, and from which template. `templates:read` is what may see that a
	// template exists at all.
	TemplateInstantiated: templatesRead,
}
