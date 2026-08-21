// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package lifecycle is the bounded context that decides when data stops existing: retention rules,
// the safeguards that override them, and the record of what was removed (ADR-0020,
// data-retention.md).
//
// Its own context rather than part of work, because the questions differ. Work asks what an entry
// is and who may change it; this asks whether it may still be here at all - and the answer is
// tenant configuration, legal obligation and the state of every device that ever held a copy, none
// of which a task knows anything about.
package lifecycle

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// HoldScope says what a legal hold covers.
//
// The four the schema allows. Three of them bear on a deletion of work: the tenant covers
// everything, a container covers its subtree, an item covers itself and what hangs off it. An
// account hold is about one person's own data - their profile, their trail - and is answered where
// that data is erased rather than here (data-protection.md §6); it does not preserve every entry
// they ever touched, or a single hold would freeze a whole workspace.
type HoldScope string

const (
	HoldTenant    HoldScope = "TENANT"
	HoldContainer HoldScope = "CONTAINER"
	HoldItem      HoldScope = "ITEM"
	HoldAccount   HoldScope = "ACCOUNT"
)

// LegalHold is an active instruction not to delete something (data-retention.md §4.1).
//
// It is the first thing checked and it overrides everything else - a retention rule, a person
// emptying their own trash, a tenant's configured period. Lifting one is auditable; that happens
// where holds are placed, which is not this task.
type LegalHold struct {
	ID    shared.ID
	Scope HoldScope
	// ScopeID is what the hold names. Empty for a tenant-wide hold, which names nothing because it
	// covers everything.
	ScopeID shared.ID
	// Reason is why, in the words of whoever placed it. It is operator content rather than user
	// content, and it never travels into a metric or an event - only an auditor and an operator
	// reading a blocked run ever see it.
	Reason   string
	PlacedAt time.Time
}

// Target is what a hard delete is about to remove, expressed as the levels a hold could name.
//
// The path rather than the identifier alone: a hold on a hub has to block a deletion three levels
// below it, and the only thing that knows the entry is three levels below it is the entry.
type Target struct {
	// ItemID is set when the target is an entry, empty when it is a container.
	ItemID shared.ID
	// ContainerIDs are the containers above and including the target: the hub and the collection for
	// an entry, and the container itself plus its hub for a container.
	ContainerIDs []shared.ID
	// AncestorItemIDs are the entries above the target, the target itself included. A hold on a task
	// covers the activities under it, and a purge walks the subtree from the bottom up
	// (data-retention.md §4.6).
	AncestorItemIDs []shared.ID
}

// Holds is the set of holds in force for one tenant.
//
// A type rather than a slice, because the question asked of it is always the same one and answering
// it in each caller is how one of them comes to answer it slightly differently.
type Holds []LegalHold

// Blocking returns the hold that forbids removing the target, and whether there is one.
//
// The first match wins and which one it is does not matter for the decision - but it is returned
// rather than a boolean, because a blocked run has to say why, and "a hold placed on the hub" is
// the answer an operator can act on where "blocked" is not.
func (h Holds) Blocking(target Target) (LegalHold, bool) {
	for _, hold := range h {
		if hold.Scope != HoldTenant && hold.ScopeID.IsZero() {
			// A hold that names nothing and is not tenant-wide is malformed. Skipped rather than
			// matched: the empty identifier equals the empty identifier, so a naive comparison would
			// make one bad row freeze every deletion in the installation.
			continue
		}

		switch hold.Scope {
		case HoldTenant:
			return hold, true
		case HoldContainer:
			if contains(target.ContainerIDs, hold.ScopeID) {
				return hold, true
			}
		case HoldItem:
			if target.ItemID == hold.ScopeID || contains(target.AncestorItemIDs, hold.ScopeID) {
				return hold, true
			}
		case HoldAccount:
			// Not a question about this entry. See HoldScope.
		}
	}
	return LegalHold{}, false
}

func contains(ids []shared.ID, wanted shared.ID) bool {
	for _, id := range ids {
		if id == wanted {
			return true
		}
	}
	return false
}
