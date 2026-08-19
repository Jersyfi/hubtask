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
	// ReferenceMember and ReferenceCustomField arrive with the use cases that own them. Named here
	// because the contract's enum names them and a client switches on the value - a kind this
	// system could produce and did not declare would be one nobody handles.
	ReferenceMember      ReferenceKind = "MEMBER"
	ReferenceCustomField ReferenceKind = "CUSTOM_FIELD"
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
	// ID is the reference that could not be carried over.
	ID shared.ID
	// Code is the stable message code saying why, which is what a client renders. It is the
	// domain's rather than the adapter's, so that the same loss reads the same way through every
	// channel.
	Code string
}

// DroppedLabel is a label the destination collection does not define.
func DroppedLabel(itemID, labelID shared.ID) DroppedReference {
	return DroppedReference{
		ItemID: itemID, Kind: ReferenceLabel, ID: labelID, Code: "labels.not_in_collection",
	}
}

// DroppedBucket is a column of the board the entry left.
func DroppedBucket(itemID, bucketID shared.ID) DroppedReference {
	return DroppedReference{
		ItemID: itemID, Kind: ReferenceBucket, ID: bucketID, Code: "buckets.not_in_collection",
	}
}
