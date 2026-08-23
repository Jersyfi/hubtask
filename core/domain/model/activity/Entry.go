// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package activity holds an item's own history: what happened to a piece of work, in the words the
// people working on it read (arc42 §5.2, the "Audit & Activity" context).
//
// It is deliberately not the audit trail, and the distinction is the one audit.md §1 draws. The
// trail is evidence for an auditor: immutable, hash chained, kept for the audit period, and
// stripped of content. The history is part of the product: read by whoever may read the item,
// written in message codes a client renders, and deleted with the item it describes. Recording one
// in the other has been tried and rejected (ADR-0017) - a history kept as evidence outlives the
// deletion obligation of the very item it is about, and a trail kept as a product feature is one
// nobody may prune.
package activity

import (
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Verb is what happened, as a stable code. Stable because it is what a client switches on and what
// the message catalogue is keyed by - renaming one is a breaking change to every translation.
type Verb string

// The verbs of the history: the seventeen things that can happen to an entry.
//
// Item-level throughout. `ActivityEntry` is keyed on the item (domain-model.md §3.5) and
// `/items/{id}/activity` is the only reader the contract declares (api-guidelines.md §2), so a hub
// being renamed is a change the audit trail and the change log record and this history does not.
const (
	ItemCreated      Verb = "item.created"
	ItemUpdated      Verb = "item.updated"
	ItemCompleted    Verb = "item.completed"
	ItemReopened     Verb = "item.reopened"
	ItemMoved        Verb = "item.moved"
	ItemReordered    Verb = "item.reordered"
	ItemArchived     Verb = "item.archived"
	ItemUnarchived   Verb = "item.unarchived"
	ItemTrashed      Verb = "item.trashed"
	ItemRestored     Verb = "item.restored"
	ItemLabelAdded   Verb = "item.label_added"
	ItemLabelRemoved Verb = "item.label_removed"
	// The assignment four. `item.assigned` covers being handed on as well as being taken up: the
	// field is a scalar, so replacing one person with another is one step rather than a removal and
	// an addition, and the change set carries both sides of it.
	ItemAssigned      Verb = "item.assigned"
	ItemUnassigned    Verb = "item.unassigned"
	ItemMemberAdded   Verb = "item.member_added"
	ItemMemberRemoved Verb = "item.member_removed"
	// ItemCommented is the one comment verb (C-03). An edit and a deletion do not write history:
	// the comment carries its own edited_at and its tombstone, and the thread is where both are
	// read - a history entry beside them would describe the same fact in a second place.
	ItemCommented Verb = "item.commented"
)

var verbs = [...]Verb{
	ItemCreated, ItemUpdated, ItemCompleted, ItemReopened, ItemMoved, ItemReordered,
	ItemArchived, ItemUnarchived, ItemTrashed, ItemRestored, ItemLabelAdded, ItemLabelRemoved,
	ItemAssigned, ItemUnassigned, ItemMemberAdded, ItemMemberRemoved, ItemCommented,
}

// Verbs returns every verb the history knows, in a stable order.
func Verbs() []Verb { return verbs[:] }

// Valid reports whether the verb is one of them.
func (v Verb) Valid() bool {
	for _, known := range verbs {
		if known == v {
			return true
		}
	}
	return false
}

// MessageCode is the catalogue key a client renders the entry through: `activity.item_completed`
// for the verb `item.completed` (i18n-l10n.md §1, ADR-0011).
//
// Derived rather than declared beside each verb, because the two would then be two things to keep
// in step. The stored verb keeps the dot the schema's comment shows; the key is that verb in the
// `activity` namespace, and a namespace separator inside a key would make `activity.item.completed`
// look like three levels where there are two.
func (v Verb) MessageCode() string {
	return "activity." + strings.ReplaceAll(string(v), ".", "_")
}

// Actor is who did it. The label is not stored, unlike the audit trail's: this entry is deleted
// with its item rather than outliving it, so there is nothing for a denormalised copy of an
// account's name to survive (audit.md §1, §2).
type Actor struct {
	Kind shared.ActorKind
	ID   shared.ID
}

// Entry is one record of an item's history.
//
// Append-only: nothing edits one, and what removes one is the deletion of the item it belongs to -
// a tenant-scoped foreign key with ON DELETE CASCADE, which is the deletion path the data catalogue
// declares for `activity_entry` ("with the item").
type Entry struct {
	ID       shared.ID
	TenantID shared.ID
	ItemID   shared.ID
	// CollectionID is the collection the item was in when this happened. It is the visibility
	// anchor rather than a second subject: an entry is about the item, and the collection is what a
	// reader is judged against.
	CollectionID shared.ID
	Actor        Actor
	Verb         Verb
	// ChangeSet is what ChangeSet() produced: the fields that moved, with their values only where
	// the product needs them. Never a raw copy of everything that changed.
	ChangeSet  map[string]any
	OccurredAt time.Time
}

// Validate refuses an entry the history could not stand behind.
//
// Strict on purpose. The entry is written inside the transaction that made the change, so an entry
// that cannot be written aborts the change it belongs to - which is the right way round: an item
// whose history silently missed one step is an item whose history nobody can trust afterwards.
func (e Entry) Validate() error {
	switch {
	case e.ID.IsZero(), e.TenantID.IsZero(), e.ItemID.IsZero(), e.OccurredAt.IsZero():
		return shared.ErrInternal.WithDetail("activity.entry_incomplete")
	case !e.Verb.Valid():
		return shared.ErrInternal.
			WithDetail("activity.verb_unknown").
			WithParams(map[string]string{"verb": string(e.Verb)})
	case !e.Actor.Kind.Auditable():
		// The same five kinds the column's CHECK constraint allows. The anonymous actor is not one
		// of them: nothing anonymous changes an item, and an entry naming one would be refused at
		// commit time, which is the worst moment to find out.
		return shared.ErrInternal.
			WithDetail("activity.actor_kind_invalid").
			WithParams(map[string]string{"actor_type": string(e.Actor.Kind)})
	}
	return nil
}

// Form is how much of a change this item type's history keeps (domain-model.md §2, the capability
// matrix's note "compact history for activities").
type Form string

const (
	// Full keeps the fields that moved beside the verb.
	Full Form = "FULL"
	// Compact keeps the verb, the actor and the time, and nothing else. An activity is done or
	// open, dated and assigned, and every one of those is read off the entry itself - a per-field
	// diff would repeat what the client already has.
	Compact Form = "COMPACT"
)

// Detail is how much of one field's change travels with its name.
//
// The task's wording is "the field names - content only where the product needs it". So the name
// is always there and the values are a decision per field, taken where the field is known rather
// than by whatever is writing the entry.
type Detail string

const (
	// WithValues records the name and both sides. For everything that is not the user's own text -
	// identifiers, ranks, states - and for the content where the product needs it: a rename means
	// nothing without both titles.
	WithValues Detail = "VALUES"
	// NameOnly records that the field changed and not what it now says. For content whose value the
	// history does not need: a note is a page of text, and what the history is about is that
	// somebody edited it.
	NameOnly Detail = "NAME_ONLY"
)

// Field is one field that moved, before the change set is built.
type Field struct {
	Name     string
	Detail   Detail
	From, To string
}

// ChangeSet turns the fields that moved into what the history keeps.
//
// The masking happens here rather than at each call site, for the reason audit.Changes has it in
// one place: a second implementation is a second chance to record a note in full text, and that
// mistake is only discovered by reading rows nobody meant to write.
func ChangeSet(form Form, fields ...Field) map[string]any {
	set := make(map[string]any, len(fields))
	if form == Compact {
		return set
	}

	for _, field := range fields {
		if field.Name == "" {
			continue
		}
		if field.Detail == NameOnly {
			set[field.Name] = map[string]any{"changed": true}
			continue
		}
		// An absent side is a field that had no value on it, recorded as absent rather than as an
		// empty string so that a client can tell "it was empty" from "it was set". Both sides
		// absent leaves the name alone in the set, which is still the fact that it changed - names
		// always, values where they exist.
		entry := map[string]any{}
		if field.From != "" {
			entry["from"] = field.From
		}
		if field.To != "" {
			entry["to"] = field.To
		}
		set[field.Name] = entry
	}
	return set
}
