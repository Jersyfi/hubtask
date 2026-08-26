// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package privacy declares what data subject rights need stored: the case, the consent, and the
// two marks an erasure leaves on things it does not delete.
package privacy

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Requests stores the cases (data-protection.md §4).
type Requests interface {
	// Insert records a new case.
	Insert(ctx context.Context, request domain.Request) error

	// Find answers one case, or ErrNotFound.
	Find(ctx context.Context, id shared.ID) (domain.Request, error)

	// Save writes a case back after the domain has moved it. It answers false when the row is not
	// there, which is a case somebody deleted under the caller rather than a failure to write.
	Save(ctx context.Context, request domain.Request) (bool, error)

	// List answers one page, soonest deadline first.
	List(ctx context.Context, filter Filter) (Page, error)

	// Deadlines is the reading behind alert A-19: how many open cases are past their deadline, how
	// many are open at all, and when the next one falls due.
	Deadlines(ctx context.Context, now time.Time) (Deadlines, error)
}

// Filter narrows a listing. Every field is optional; the zero filter is "what do we still owe",
// which is what somebody opening the list is asking.
type Filter struct {
	Status domain.Status
	Kind   domain.Kind
	// DueBefore keeps the cases whose deadline falls before an instant, overdue ones included.
	DueBefore time.Time
	// IncludeClosed adds the answered and refused ones, which is a different question: what did we
	// do, rather than what do we owe.
	IncludeClosed bool
	Cursor        string
	Size          int
}

// Page is one page of cases.
type Page struct {
	Requests []domain.Request
	Info     PageInfo
}

// PageInfo is the walk's own state.
type PageInfo struct {
	NextCursor string
	HasMore    bool
}

// Deadlines is what the alert reads.
type Deadlines struct {
	Overdue int
	Open    int
	// NextDueAt is when the soonest open case falls due, and the zero time when none is open.
	NextDueAt time.Time
}

// Consents stores what somebody agreed to and what they took back (Art. 21).
//
// A consent is never updated into a different consent: what an operator has to be able to show is
// not "is this allowed now" but "was it allowed then", so ending one and recording one are two
// separate acts here as well.
type Consents interface {
	// Withdraw ends every standing consent of an account for a purpose, and answers how many it
	// ended. Zero is not an error: withdrawing a consent nobody granted is a legitimate act, and
	// the record of the refusal is written either way.
	Withdraw(ctx context.Context, accountID shared.ID, purpose string, at time.Time) (int, error)

	// Record writes one consent record.
	Record(ctx context.Context, consent domain.Consent) error

	// Latest answers the most recent record for an account and a purpose, or ErrNotFound.
	Latest(ctx context.Context, accountID shared.ID, purpose string) (domain.Consent, error)
}

// Subjects is what an erasure and a restriction reach for: the account behind the case.
//
// Its own port rather than a method on the identity repository, because these two writes are not
// ordinary account maintenance - one stops processing and the other ends a person's presence in a
// workspace, and both are answerable to a legal deadline.
type Subjects interface {
	// SetStatus writes an account's status - RESTRICTED, ANONYMIZED, or back to ACTIVE - and
	// answers false when there is no such account here.
	SetStatus(ctx context.Context, id shared.ID, status string, at time.Time) (bool, error)

	// Tenants answers the workspaces of this installation in which one address is a member.
	//
	// The one cross-tenant question in the system (data-protection.md §4). It takes an address
	// rather than a tenant, and it answers identifiers rather than rows: what follows is one
	// ordinary transaction per tenant, under that tenant's own context.
	Tenants(ctx context.Context, email string) ([]shared.ID, error)
}

// Pseudonyms is the substitution the audit trail reads at the boundary (audit.md §6).
//
// The trail is exempt from erasure and cannot be edited in place - the grants, the trigger and the
// hash chain all refuse it - so what an erasure leaves is a mapping from an actor to a label with
// no meaning outside the workspace.
type Pseudonyms interface {
	// Assign maps one actor to one pseudonym. Idempotent: an erasure that is retried records the
	// same pseudonym rather than a second one.
	//
	// Not `Record`, which is what a consent is written with: one adapter serves both ports, and
	// two methods of one name on one type is a collision rather than a symmetry.
	Assign(ctx context.Context, actorID shared.ID, pseudonym, reason string, at time.Time) error

	// For answers the pseudonyms of the actors named, and nothing for the actors that have none.
	For(ctx context.Context, actorIDs []shared.ID) (map[shared.ID]string, error)
}
