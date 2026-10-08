// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// What an erasure does when legal holds are in force (data-protection.md §4.1): a hold wins over an
// erasure exactly as far as it reaches. One function decides it, and the erasure and its preview
// both call it - two readings of the same rule would show a person one thing and do another.

// RowKind is what one of the person's rows on an entry is.
type RowKind string

const (
	RowComment    RowKind = "COMMENT"
	RowAssignment RowKind = "ASSIGNMENT"
	RowEntry      RowKind = "ENTRY"
)

// Row is one of the person's rows on an entry, and where that entry is.
type Row struct {
	Kind RowKind
	// ID is the comment's identifier, or the entry's for an assignment and an entry.
	ID           shared.ID
	ItemID       shared.ID
	Path         string
	CollectionID shared.ID
	HubID        shared.ID
	// ItemCreatedBy is who created the entry the row sits on.
	ItemCreatedBy shared.ID
}

// AccountFate is what happens to the person's account.
type AccountFate string

const (
	// AccountAnonymised is the account renamed to former user, without address, password or
	// credential - what *anonymise* always does, and what a full deletion does instead of removing
	// the row while a kept contribution still names it as its author.
	AccountAnonymised AccountFate = "ANONYMISED"
	// AccountDeleted is the row removed.
	AccountDeleted AccountFate = "DELETED"
	// AccountKept is the account left as it is and restricted: a hold on it or on the workspace
	// reaches it.
	AccountKept AccountFate = "KEPT"
)

// Plan is what one erasure does, given the holds in force.
type Plan struct {
	Account AccountFate
	// DeleteComments are the comments a full deletion removes; the ones a hold keeps are not here.
	DeleteComments []shared.ID
	// ReleaseOn are the entries whose assignment to the person goes.
	ReleaseOn []shared.ID
	// KeepIntake is the intake - matched by address, on no entry - kept by a hold on the workspace.
	KeepIntake bool
	// Kept is what each hold keeps, in the order the holds were placed; empty when nothing is.
	Kept []Kept
}

// PlanInput is everything the decision needs.
type PlanInput struct {
	Subject shared.ID
	Mode    ErasureMode
	// Holds are the holds in force, oldest first: where several cover one row, the oldest is named.
	Holds lifecycle.Holds
	Rows  []Row
	// Intake is how many intake entries carry the person's address, which a workspace hold keeps.
	Intake int
}

// PlanErasure decides what an erasure keeps and what it does with the rest.
//
// A row on an entry is kept when a hold covers the entry: the workspace, the entry's hub or
// collection, the entry or one above it, or the account that created it - a field of that person's
// entry is theirs as much as the entry is. The subject's own comment is the subject's, so only a
// hold on the subject keeps it by account. A hold on the subject or on the workspace keeps the
// account itself; under any other hold the account is anonymised as it would be anyway, and the
// kept rows stay attributed to the former user.
//
// A row counts as kept only where the erasure would otherwise have changed it: a comment a full
// deletion would remove, an assignment it would release, and - where the account is kept - the
// person's name on their comments and entries.
func PlanErasure(in PlanInput) Plan {
	plan := Plan{Account: AccountAnonymised}
	if in.Mode == ModeFullDelete {
		plan.Account = AccountDeleted
	}

	parts := map[shared.ID]*Kept{}
	var order []shared.ID
	part := func(hold lifecycle.LegalHold) *Kept {
		if found, ok := parts[hold.ID]; ok {
			return found
		}
		kept := &Kept{HoldID: hold.ID, HoldScope: hold.Scope, HoldScopeID: hold.ScopeID}
		parts[hold.ID] = kept
		order = append(order, hold.ID)
		return kept
	}

	// The account first: whether it stays decides what the person's name on a kept row means.
	for _, hold := range in.Holds {
		if hold.Scope == lifecycle.HoldTenant ||
			(hold.Scope == lifecycle.HoldAccount && !in.Subject.IsZero() && hold.ScopeID == in.Subject) {
			plan.Account = AccountKept
			part(hold).Account = true
			break
		}
	}
	if hold, held := in.Holds.OnTenant(); held {
		plan.KeepIntake = true
		if in.Intake > 0 {
			part(hold).Intake += in.Intake
		}
	}

	keptLocated := false
	for _, row := range in.Rows {
		hold, covered := in.Holds.Blocking(targetOf(row, in.Subject))
		switch row.Kind {
		case RowComment:
			switch {
			case covered && (in.Mode == ModeFullDelete || plan.Account == AccountKept):
				part(hold).Comments++
				keptLocated = true
			case !covered && in.Mode == ModeFullDelete:
				plan.DeleteComments = append(plan.DeleteComments, row.ID)
			}
		case RowAssignment:
			if covered {
				part(hold).Assignments++
				keptLocated = true
			} else {
				plan.ReleaseOn = append(plan.ReleaseOn, row.ItemID)
			}
		case RowEntry:
			if covered && plan.Account == AccountKept {
				part(hold).Entries++
			}
		}
	}

	// A full deletion keeps the row of an account a kept contribution still names as its author:
	// the contribution references the account, and removing it would leave the row pointing at
	// nothing - which is anonymising held data by another route.
	if plan.Account == AccountDeleted && keptLocated {
		plan.Account = AccountAnonymised
	}

	for _, id := range order {
		plan.Kept = append(plan.Kept, *parts[id])
	}
	return plan
}

// Keeps reports whether anything is kept.
func (p Plan) Keeps() bool { return len(p.Kept) > 0 }

// targetOf is a row as a hold is judged against it. Whose row it is decides which account hold
// reaches it: an assignment and an entry belong to the entry's creator, a comment to the person.
func targetOf(row Row, subject shared.ID) lifecycle.Target {
	owner := row.ItemCreatedBy
	if row.Kind == RowComment {
		owner = subject
	}
	containers := make([]shared.ID, 0, 2)
	for _, id := range []shared.ID{row.HubID, row.CollectionID} {
		if !id.IsZero() {
			containers = append(containers, id)
		}
	}
	target := lifecycle.Target{
		ItemID:          row.ItemID,
		ContainerIDs:    containers,
		AncestorItemIDs: work.PathIDs(row.Path),
	}
	if !owner.IsZero() {
		target.Contributors = []shared.ID{owner}
	}
	return target
}
