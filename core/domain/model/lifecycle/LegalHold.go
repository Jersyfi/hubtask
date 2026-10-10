// SPDX-License-Identifier: Apache-2.0
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
	"errors"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// HoldScope says what a legal hold covers.
//
// The four the schema allows: the tenant covers everything, a container covers its subtree, an item
// covers itself and what hangs off it, and an account covers that person's account and what they
// contributed - the entries they created, their comments, the files they attached
// (data-protection.md §4.1). A removal takes comments and attachments with their entry, so it is
// judged against the contributors of everything it takes (Target.Contributors).
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
// where holds are placed and lifted, not here.
type LegalHold struct {
	ID    shared.ID
	Scope HoldScope
	// ScopeID is what the hold names. Empty for a tenant-wide hold, which names nothing because it
	// covers everything.
	ScopeID shared.ID
	// Reason is why, in the words of whoever placed it. It is operator content rather than user
	// content, and it never travels into a metric or an event - only an auditor and an operator
	// reading a blocked run ever see it.
	Reason string
	// PlacedBy and PlacedAt are who and when. The audit obligation of §4.1 is about who as much as
	// about what - "lifting it is auditable" is a statement about a person.
	PlacedBy shared.ID
	PlacedAt time.Time
	// ReleasedBy, ReleasedAt and ReleasedReason are the lifting. A hold is never deleted: the row
	// stays and gains an end, because a hold that vanished would leave an auditor unable to tell
	// "there was never one" from "somebody lifted it".
	ReleasedBy     shared.ID
	ReleasedAt     time.Time
	ReleasedReason string
}

// Released reports a hold that is no longer in force.
func (h LegalHold) Released() bool { return !h.ReleasedAt.IsZero() }

// maxHoldReason bounds what a reason may say. Two thousand characters is a paragraph and a
// citation; beyond that somebody is storing a document in a column, and the column is read by an
// auditor rather than searched.
const maxHoldReason = 2000

// NewHoldInput is one hold as somebody asked for it.
type NewHoldInput struct {
	ID       shared.ID
	Scope    HoldScope
	ScopeID  shared.ID
	Reason   string
	PlacedBy shared.ID
	Now      time.Time

	// Text brings the reason to normal form C before it is bounded and stored (i18n-l10n.md §5);
	// work.NewWorkItemInput says why it is handed in.
	Text text.Normalizer
}

// NewLegalHold builds a hold and refuses what cannot be honoured.
//
// A hold that is stored and not honoured is worse than a hold that was refused, because somebody
// believes it is in force. Every scope here is honoured: by every deletion path through
// Holds.Blocking, and by an erasure (data-protection.md §4.1). Whether what it names exists in the
// workspace is the application's question, which reads the workspace.
func NewLegalHold(in NewHoldInput) (LegalHold, error) {
	reason, err := shared.NFC(strings.TrimSpace(in.Reason), in.Text)
	if err != nil {
		return LegalHold{}, err
	}
	switch {
	case in.ID.IsZero() || in.PlacedBy.IsZero():
		return LegalHold{}, invalidHold(CodeHoldIncomplete, "/reason")
	case !in.Scope.Valid():
		return LegalHold{}, invalidHold(CodeHoldScopeInvalid, "/scope")
	case reason == "":
		return LegalHold{}, invalidHold(CodeHoldReasonRequired, "/reason")
	case len(reason) > maxHoldReason:
		return LegalHold{}, invalidHold(CodeHoldReasonTooLong, "/reason")
	// A tenant-wide hold names nothing because it covers everything; every other scope names what
	// it covers, and one that did not would be a hold nothing could be judged against.
	case (in.Scope == HoldTenant) != in.ScopeID.IsZero():
		return LegalHold{}, invalidHold(CodeHoldScopeIDMismatch, "/scope")
	}

	return LegalHold{
		ID: in.ID, Scope: in.Scope, ScopeID: in.ScopeID, Reason: reason,
		PlacedBy: in.PlacedBy, PlacedAt: in.Now,
	}, nil
}

// Release lifts a hold, and refuses to lift one twice.
//
// A second lifting would overwrite who lifted it and when, which is the one pair of values the
// record exists to keep - and the caller asking for it is working from a stale reading rather than
// asking for something new.
//
// The normaliser is handed in for the reason NewLegalHold takes one (i18n-l10n.md §5).
func (h LegalHold) Release(by shared.ID, reason string, form text.Normalizer, at time.Time) (LegalHold, error) {
	trimmed, err := shared.NFC(strings.TrimSpace(reason), form)
	if err != nil {
		return LegalHold{}, err
	}
	switch {
	case h.Released():
		return LegalHold{}, shared.ErrConflict.WithDetail(CodeHoldAlreadyReleased).
			WithParams(map[string]string{"hold_id": h.ID.String()})
	case by.IsZero():
		return LegalHold{}, invalidHold(CodeHoldIncomplete, "/reason")
	case trimmed == "":
		return LegalHold{}, invalidHold(CodeHoldReasonRequired, "/reason")
	case len(trimmed) > maxHoldReason:
		return LegalHold{}, invalidHold(CodeHoldReasonTooLong, "/reason")
	}

	h.ReleasedBy, h.ReleasedAt, h.ReleasedReason = by, at, trimmed
	return h, nil
}

// Valid reports whether a scope is one the schema allows.
func (s HoldScope) Valid() bool {
	switch s {
	case HoldTenant, HoldContainer, HoldItem, HoldAccount:
		return true
	}
	return false
}

func invalidHold(code, field string) error {
	return shared.ErrValidation.WithDetail(code).
		WithFields(shared.FieldError{Path: field, Code: code}).
		WithCause(errors.New(code))
}

// The refusals of a legal hold, as codes rather than as prose.
const (
	CodeHoldIncomplete      = "lifecycle.hold_incomplete"
	CodeHoldScopeInvalid    = "lifecycle.hold_scope_invalid"
	CodeHoldScopeIDMismatch = "lifecycle.hold_scope_id_mismatch"
	CodeHoldReasonRequired  = "lifecycle.hold_reason_required"
	CodeHoldReasonTooLong   = "lifecycle.hold_reason_too_long"
	CodeHoldNotFound        = "lifecycle.hold_not_found"
	// CodeHoldTargetNotFound is a hold naming a hub, collection, entry or account this workspace
	// does not have.
	CodeHoldTargetNotFound  = "lifecycle.hold_target_not_found"
	CodeHoldAlreadyReleased = "lifecycle.hold_already_released"
	// CodeLegalHold refuses removing for good what a hold covers.
	CodeLegalHold = "lifecycle.legal_hold"
	// RefusalScopeWorkspace is the scope a refusal names when the workspace as a whole is to go -
	// deleted, or reset to a backup. Never the hold's own scope: whoever asks learns that a hold
	// stands, not that a person or one entry is under it (P-01).
	RefusalScopeWorkspace = "WORKSPACE"
)

// WorkspaceUnderHold is the refusal of an act that would remove the whole workspace's data while
// any hold is in force (data-protection.md §5): its deletion, or a reset to a backup. A conflict,
// because the state is what stands in the way, and waiting for the hold's release is what helps.
func WorkspaceUnderHold() error {
	return shared.ErrConflict.WithDetail(CodeLegalHold).
		WithParams(map[string]string{"scope": RefusalScopeWorkspace})
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
	// (data-retention.md §4 item 6).
	AncestorItemIDs []shared.ID
	// Contributors are the accounts whose own data goes with the target: who created an entry,
	// wrote a comment on it, or uploaded a file attached to it, across everything the removal takes.
	// An account hold covers that data (data-protection.md §4.1), and a comment falls with its entry.
	// Empty where no account hold is in force - the caller does not have to read them then.
	Contributors []shared.ID
}

// TemplateTarget is what a hold is judged against for a template (data-retention.md §4): the hub or
// collection it is defined on and, for a collection, the hub above it - empty for a workspace-wide
// one, which only a workspace hold reaches. No entry: a template sits on none, so an entry hold
// never reaches it. No contributors: a template names no author, so an account hold finds nothing
// of the person's in it - a node's assignee is a name in a plan, not something they contributed.
func TemplateTarget(scopeID, hubID shared.ID) Target {
	containers := make([]shared.ID, 0, 2)
	for _, id := range []shared.ID{scopeID, hubID} {
		if !id.IsZero() {
			containers = append(containers, id)
		}
	}
	return Target{ContainerIDs: containers}
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
			if contains(target.Contributors, hold.ScopeID) {
				return hold, true
			}
		}
	}
	return LegalHold{}, false
}

// AnyOnAccounts reports whether an account hold is in force, which is when a removal has to read
// the contributors of what it removes at all.
func (h Holds) AnyOnAccounts() bool {
	for _, hold := range h {
		if hold.Scope == HoldAccount && !hold.ScopeID.IsZero() {
			return true
		}
	}
	return false
}

// OnAccount returns the hold in force on one account, and whether there is one.
func (h Holds) OnAccount(accountID shared.ID) (LegalHold, bool) {
	for _, hold := range h {
		if hold.Scope == HoldAccount && !accountID.IsZero() && hold.ScopeID == accountID {
			return hold, true
		}
	}
	return LegalHold{}, false
}

// OnTenant returns the hold in force on the whole workspace, and whether there is one.
func (h Holds) OnTenant() (LegalHold, bool) {
	for _, hold := range h {
		if hold.Scope == HoldTenant {
			return hold, true
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
