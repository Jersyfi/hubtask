// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package privacy is the bounded context of data subject rights (data-protection.md §4,
// ADR-0018): the case somebody's exercised right becomes, the deadline it carries, and the
// consent that can be taken back.
//
// Its own context rather than part of identity, because the questions differ. Identity asks who
// somebody is and what they may do; this asks what a person is entitled to demand about the data
// held on them, and the answer is a legal period, a decision the controller has to take, and a
// piece of work that touches every storage location in the data catalogue.
//
// What is deliberately *not* here: how the erasure is carried out and what an export contains.
// Those are the application layer's, because they are questions about storage.
package privacy

import (
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Kind is the right that was exercised (data-protection.md §4).
//
// Six, as the schema has allowed since `0001_init`. `RECTIFICATION` is the one that needs no
// special path - a correction is an ordinary write, and the change appears in the audit like any
// other - and it is a tracked case all the same: a deadline somebody is answerable for is worth
// recording whether or not the work is special.
type Kind string

const (
	// KindAccess is Art. 15: a copy of everything held about the person.
	KindAccess Kind = "ACCESS"
	// KindErasure is Art. 17, and the only kind that destroys anything.
	KindErasure Kind = "ERASURE"
	// KindPortability is Art. 20: the same copy, in a documented machine-readable form. It is the
	// same archive as an access request, and the difference is the obligation rather than the
	// bytes.
	KindPortability Kind = "PORTABILITY"
	// KindRestriction is Art. 18: processing stops, the data stays.
	KindRestriction Kind = "RESTRICTION"
	// KindObjection is Art. 21: consent for optional processing is withdrawn.
	KindObjection Kind = "OBJECTION"
	// KindRectification is Art. 16.
	KindRectification Kind = "RECTIFICATION"
)

var kinds = [...]Kind{
	KindAccess, KindErasure, KindPortability, KindRestriction, KindObjection, KindRectification,
}

// Kinds returns every kind a case can have.
func Kinds() []Kind { return kinds[:] }

func (k Kind) Valid() bool {
	for _, known := range kinds {
		if k == known {
			return true
		}
	}
	return false
}

// ProducesArchive reports whether starting this case writes an export. Access and portability are
// one piece of work with two legal names.
func (k Kind) ProducesArchive() bool { return k == KindAccess || k == KindPortability }

// Status is where the case has got to.
type Status string

const (
	StatusReceived   Status = "RECEIVED"
	StatusInProgress Status = "IN_PROGRESS"
	StatusCompleted  Status = "COMPLETED"
	StatusRejected   Status = "REJECTED"
)

// Closed reports a case that is over. A closed case does not move again: an answer somebody has
// already been given cannot be reopened into a different one, and a second case is the honest way
// to record a second request.
func (s Status) Closed() bool { return s == StatusCompleted || s == StatusRejected }

// transitions is the state machine data-protection.md §4 names, written out rather than derived.
//
// `RECEIVED → IN_PROGRESS → COMPLETED | REJECTED`, plus the one shortcut the law forces: a request
// can be refused before anybody starts work on it, and refusing it is exactly what the deadline is
// about.
var transitions = map[Status][]Status{
	StatusReceived:   {StatusInProgress, StatusRejected},
	StatusInProgress: {StatusCompleted, StatusRejected},
}

// CanMoveTo reports whether the case may take that step.
func (s Status) CanMoveTo(next Status) bool {
	for _, allowed := range transitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// ErasureMode is the controller's choice (ADR-0018 decision 5).
type ErasureMode string

const (
	// ModeAnonymize keeps the authorship as a former user, so that the workspace's own content -
	// which belongs to third parties as much as to the person - stays readable.
	ModeAnonymize ErasureMode = "ANONYMIZE"
	// ModeFullDelete takes the person's own contributions with them.
	ModeFullDelete ErasureMode = "FULL_DELETE"
)

func (m ErasureMode) Valid() bool { return m == ModeAnonymize || m == ModeFullDelete }

// Scope is how far the case reaches.
type Scope string

const (
	// ScopeTenant is this workspace, which is what a controller answering for their own workspace
	// means.
	ScopeTenant Scope = "TENANT"
	// ScopeInstallation is every workspace of the installation in which the person is a member -
	// the one operation in this system that legitimately crosses the tenant boundary
	// (data-protection.md §4).
	ScopeInstallation Scope = "INSTALLATION"
)

func (s Scope) Valid() bool { return s == ScopeTenant || s == ScopeInstallation }

// DefaultDeadline is the statutory period the case gets when nobody names another: one month,
// expressed as the thirty days data-protection.md §4 uses.
const DefaultDeadline = 30 * 24 * time.Hour

// maxNotes and maxReason bound the free text. A case carries what somebody wrote about it - the
// occasion, the reason for a refusal - and both are read by a person rather than searched.
const (
	maxNotes  = 4000
	maxReason = 2000
)

// Request is one exercised right, as a case somebody answers for.
type Request struct {
	ID   shared.ID
	Kind Kind
	// Status is where it has got to; the state machine is above.
	Status Status
	Scope  Scope
	// SubjectAccountID is the account the case is about, and the zero identifier once a full
	// deletion has taken it. The column is nullable for exactly that reason: the case outlives the
	// account, because the record that a request was handled is the evidence.
	SubjectAccountID shared.ID
	// SubjectEmail is who asked, for a request that has no account behind it - somebody who was
	// invited and never accepted, or a person writing about data they believe is held.
	SubjectEmail string
	ErasureMode  ErasureMode
	ReceivedAt   time.Time
	// DueAt is the statutory deadline. It is stored rather than computed, because the period is a
	// legal matter: an installation may have to answer sooner, and a case that recomputed its own
	// deadline would quietly move it.
	DueAt       time.Time
	CompletedAt time.Time
	HandledBy   shared.ID
	// RejectionReason is why the request was refused. A refusal without a reason is not an answer.
	RejectionReason string
	// TargetID is where an export is written, and ResultArchive is where it landed. The archive is
	// a Hubtask archive at a backup target (backup-restore.md §9) rather than a download this
	// system serves: an export *is* a restorable backup, and a target is a channel the operator
	// has already approved.
	TargetID      shared.ID
	ResultArchive string
	Notes         string
}

// NewRequestInput is one case as somebody recorded it.
type NewRequestInput struct {
	ID               shared.ID
	Kind             Kind
	Scope            Scope
	SubjectAccountID shared.ID
	SubjectEmail     string
	DueAt            time.Time
	TargetID         shared.ID
	Notes            string
	Now              time.Time
}

// NewRequest opens a case, and refuses one nobody could answer.
//
// The subject is the refusal that matters. A case about nobody is a deadline with no work behind
// it, and the two ways of naming a person - the account they hold here, or the address they wrote
// from - are both accepted because a request from somebody who never got as far as an account is
// still a request.
func NewRequest(in NewRequestInput) (Request, error) {
	if !in.Kind.Valid() {
		return Request{}, invalid(CodeKindInvalid, "/kind")
	}

	scope := in.Scope
	if scope == "" {
		scope = ScopeTenant
	}
	if !scope.Valid() {
		return Request{}, invalid(CodeScopeInvalid, "/scope")
	}

	email := strings.TrimSpace(in.SubjectEmail)
	if in.SubjectAccountID.IsZero() && email == "" {
		return Request{}, invalid(CodeSubjectRequired, "/subject_account_id")
	}
	notes, err := bounded(in.Notes, maxNotes, CodeNotesTooLong, "/notes")
	if err != nil {
		return Request{}, err
	}

	due := in.DueAt
	if due.IsZero() {
		due = in.Now.Add(DefaultDeadline)
	}
	if !due.After(in.Now) {
		// A deadline that has already passed is not a period to answer within; it is a mistake
		// somebody would only notice from the alert.
		return Request{}, invalid(CodeDeadlineInPast, "/due_at")
	}

	return Request{
		ID: in.ID, Kind: in.Kind, Status: StatusReceived, Scope: scope,
		SubjectAccountID: in.SubjectAccountID, SubjectEmail: email,
		ReceivedAt: in.Now, DueAt: due, TargetID: in.TargetID, Notes: notes,
	}, nil
}

// Start moves the case to IN_PROGRESS, which is what sets the work going.
//
// It is here rather than in the use case because the refusals are the domain's: an erasure with no
// mode chosen and an export with nowhere to write are both cases that cannot be carried out, and
// finding that out after a job has started would leave a case that says it is running and is not.
func (r Request) Start(mode ErasureMode, targetID shared.ID, by shared.ID) (Request, error) {
	if !r.Status.CanMoveTo(StatusInProgress) {
		return Request{}, transitionRefused(r.Status, StatusInProgress)
	}

	started := r
	started.Status = StatusInProgress
	if !by.IsZero() {
		started.HandledBy = by
	}

	if r.Kind == KindErasure {
		if mode == "" {
			mode = r.ErasureMode
		}
		if !mode.Valid() {
			return Request{}, invalid(CodeErasureModeRequired, "/erasure_mode")
		}
		started.ErasureMode = mode
	}
	if r.Kind.ProducesArchive() {
		if targetID.IsZero() {
			targetID = r.TargetID
		}
		if targetID.IsZero() {
			return Request{}, invalid(CodeExportTargetRequired, "/target_id")
		}
		started.TargetID = targetID
	}
	return started, nil
}

// Complete records that the work is done, and what it left behind.
func (r Request) Complete(at time.Time, archive string) (Request, error) {
	if !r.Status.CanMoveTo(StatusCompleted) {
		return Request{}, transitionRefused(r.Status, StatusCompleted)
	}

	done := r
	done.Status = StatusCompleted
	done.CompletedAt = at
	if archive != "" {
		done.ResultArchive = archive
	}
	return done, nil
}

// Reject refuses the request, with the reason that makes it an answer.
func (r Request) Reject(reason string, by shared.ID, at time.Time) (Request, error) {
	if !r.Status.CanMoveTo(StatusRejected) {
		return Request{}, transitionRefused(r.Status, StatusRejected)
	}
	text, err := bounded(reason, maxReason, CodeReasonTooLong, "/rejection_reason")
	if err != nil {
		return Request{}, err
	}
	if text == "" {
		return Request{}, invalid(CodeRejectionReasonRequired, "/rejection_reason")
	}

	rejected := r
	rejected.Status = StatusRejected
	rejected.RejectionReason = text
	rejected.CompletedAt = at
	if !by.IsZero() {
		rejected.HandledBy = by
	}
	return rejected, nil
}

// Overdue reports a case whose deadline has passed while it is still open. It is what the alert
// watches, and what a list of "what is late" is built from.
func (r Request) Overdue(now time.Time) bool {
	return !r.Status.Closed() && now.After(r.DueAt)
}
