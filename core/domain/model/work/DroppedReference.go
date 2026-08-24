// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import "github.com/Jersyfi/hubtask/core/domain/model/shared"

// ReferenceKind is what an entry lost when it moved. The values are the contract's
// (api/openapi.yaml, schema DroppedReference).
type ReferenceKind string

const (
	// ReferenceLabel is a label of the collection the entry left. A label is a vocabulary one
	// collection agreed on, so it does not travel.
	ReferenceLabel ReferenceKind = "LABEL"
	// ReferenceBucket is a column of the board the entry left.
	ReferenceBucket ReferenceKind = "BUCKET"
	// ReferenceMember is an account the destination does not reach, and ReferenceCustomField a
	// value whose definition it does not define. Both arrive with C-11, which is the first
	// operation that carries a whole entry from one collection to another.
	ReferenceMember      ReferenceKind = "MEMBER"
	ReferenceCustomField ReferenceKind = "CUSTOM_FIELD"
	// ReferenceAssignee is the one person an entry was on. Its own kind rather than a member,
	// because the two are different fields with different rules - an activity carries an assignee
	// and no member list at all (domain-model.md §2) - and a client that put the assignee back
	// would call a different operation from the one that puts a member back.
	ReferenceAssignee ReferenceKind = "ASSIGNEE"
	// ReferenceAttachment is a file an entry pointed at and the copy may not: only the capability
	// takes one away, since a media object belongs to the tenant rather than to a collection and
	// therefore resolves wherever an entry lands.
	ReferenceAttachment ReferenceKind = "ATTACHMENT"
)

// DroppedReference is one thing a move took away from an entry, and why (invariant I-W6).
//
// Reported rather than silently dropped. Moving a task to another collection takes it away from
// the vocabulary it was tagged from and the board it sat on; a client that saw the move succeed and
// the chips disappear would have no way to tell that from a rendering fault - and a person who had
// spent an afternoon labelling would have no way to know what to redo.
type DroppedReference struct {
	// ItemID is the entry that lost it. A move carries a whole subtree, so one operation can drop
	// references from several entries at once.
	ItemID shared.ID
	Kind   ReferenceKind
	// ID is the reference that could not be carried over, as the contract spells it: an identifier
	// for a label, a column, an account, and the key itself for a custom field - which has no
	// identifier in a collection that does not define it. Text rather than an identifier for
	// exactly that reason, and the contract types it as a plain string (api/openapi.yaml, schema
	// DroppedReference).
	ID string
	// Code is the stable message code saying why, which is what a client renders. It is the
	// domain's rather than the adapter's, so that the same loss reads the same way through every
	// channel.
	Code string
}

// DroppedLabel is a label the destination collection does not define.
func DroppedLabel(itemID, labelID shared.ID) DroppedReference {
	return DroppedReference{
		ItemID: itemID, Kind: ReferenceLabel, ID: labelID.String(), Code: "labels.not_in_collection",
	}
}

// DroppedBucket is a column of the board the entry left.
func DroppedBucket(itemID, bucketID shared.ID) DroppedReference {
	return DroppedReference{
		ItemID: itemID, Kind: ReferenceBucket, ID: bucketID.String(), Code: "buckets.not_in_collection",
	}
}

// DroppedMember and DroppedAssignee are accounts the destination collection does not reach: an
// entry may only be handed to somebody who can see it, and a copy into another collection is the
// moment that can stop being true (C-01, C-11).
func DroppedMember(itemID, accountID shared.ID) DroppedReference {
	return DroppedReference{
		ItemID: itemID, Kind: ReferenceMember, ID: accountID.String(), Code: "items.member_cannot_see_item",
	}
}

func DroppedAssignee(itemID, accountID shared.ID) DroppedReference {
	return DroppedReference{
		ItemID: itemID, Kind: ReferenceAssignee, ID: accountID.String(),
		Code: "items.assignee_cannot_see_item",
	}
}

// DroppedCustomField is a value whose definition the destination collection does not define, or
// whose definition there will not accept it - a key that is text in one collection and a number in
// the other is not the same field (C-07).
func DroppedCustomField(itemID shared.ID, key, code string) DroppedReference {
	return DroppedReference{ItemID: itemID, Kind: ReferenceCustomField, ID: key, Code: code}
}

// DroppedCapability is a reference the copy's own type may no longer carry: the profile in force
// has been narrowed since the entry was written, and a copy takes only what the profile allows
// (domain-model.md §2).
func DroppedCapability(itemID shared.ID, kind ReferenceKind, id string) DroppedReference {
	return DroppedReference{
		ItemID: itemID, Kind: kind, ID: id, Code: "items.capability_not_supported",
	}
}
