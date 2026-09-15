// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MutationKind is what a device did offline, in the contract's words (api/openapi.yaml,
// SyncMutation.kind).
type MutationKind string

const (
	ItemCreate MutationKind = "ITEM_CREATE"
	ItemPatch  MutationKind = "ITEM_PATCH"
	ItemDelete MutationKind = "ITEM_DELETE"
	SetAdd     MutationKind = "SET_ADD"
	SetRemove  MutationKind = "SET_REMOVE"
	Move       MutationKind = "MOVE"
	CommentAdd MutationKind = "COMMENT_ADD"
)

// MutationKinds is the closed set, in the contract's order.
func MutationKinds() []MutationKind {
	return []MutationKind{ItemCreate, ItemPatch, ItemDelete, SetAdd, SetRemove, Move, CommentAdd}
}

// Valid reports whether the kind is one the contract names.
func (k MutationKind) Valid() bool {
	for _, known := range MutationKinds() {
		if known == k {
			return true
		}
	}
	return false
}

// ResultKind is what became of one mutation (offline-sync.md §3.2).
type ResultKind string

const (
	// Applied: every field the device sent won, and nothing else had moved since its base.
	Applied ResultKind = "APPLIED"
	// Merged: the server's copy had moved; some of what the device sent won and some lost, and
	// the result carries the server's state so the device adopts it.
	Merged ResultKind = "MERGED"
	// Rejected: the mutation could not be applied at all - the use case refused it, the entry is
	// gone, the device may not - and the result carries the code.
	Rejected ResultKind = "REJECTED"
	// Conflict: a free-text field lost, and the displaced version is preserved as a comment; the
	// result carries both values so a client can offer the choice (offline-sync.md §5).
	Conflict ResultKind = "CONFLICT"
)

// Bound applies §4.1's rule to a device's clock reading: a reading further than the permitted
// skew from server time is replaced by a server reading, and the caller is told by how much it
// was out so that the fact can be logged - the device and the drift, never the content.
//
// Both directions are bounded. A device ahead of the server would outvote everybody for as long
// as its lead lasted; a device behind would lose every merge it should have won, which is quieter
// and just as wrong. The replacement keeps the device's identifier: the reading is still the
// device's act, only its moment is the server's.
func Bound(reading shared.HLC, serverNow time.Time, skew time.Duration) (shared.HLC, time.Duration) {
	drift := reading.Physical.Sub(serverNow)
	if drift < 0 {
		drift = -drift
	}
	if drift <= skew {
		return reading, 0
	}
	bounded, err := shared.NewHLC(serverNow, reading.Counter, reading.Device)
	if err != nil {
		// A reading that parsed has a device and a counter within bounds; the only way to fail
		// here is a zero server time, which is a defect rather than input.
		return reading, drift
	}
	return bounded, drift
}
