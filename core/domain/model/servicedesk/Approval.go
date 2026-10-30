// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// ApprovalState is where an approval stands.
type ApprovalState string

const (
	ApprovalPending  ApprovalState = "PENDING"
	ApprovalApproved ApprovalState = "APPROVED"
	ApprovalDeclined ApprovalState = "DECLINED"
	// ApprovalWithdrawn is an approval its entry left behind: moved on by another step, trashed.
	ApprovalWithdrawn ApprovalState = "WITHDRAWN"
)

// Verdict is one approver's answer.
type Verdict string

const (
	VerdictApprove Verdict = "APPROVE"
	VerdictDecline Verdict = "DECLINE"
)

// MaxApprovalReason bounds a decision's reason.
const MaxApprovalReason = 2000

// OpenLevel is one level of an approval as it opened: the step's level and the people counted
// then - named, in a group, named by a person field.
type OpenLevel struct {
	Level   work.ApprovalLevel
	Counted []shared.ID
}

// Approval is a step waiting for people's yes on one entry (UC-SVC-08): the step, the entry's
// version when it was asked, its levels and the one deciding now.
type Approval struct {
	ID          shared.ID
	TenantID    shared.ID
	ItemID      shared.ID
	Workflow    string
	Step        string
	ItemVersion int
	Levels      []OpenLevel
	DeclineTo   string
	DueHours    int
	// Current is the level deciding now, from zero.
	Current     int
	State       ApprovalState
	RequestedBy shared.ID
	RequestedAt time.Time
	// LevelSince is when the current level began, which its due time counts from.
	LevelSince time.Time
	DecidedAt  *time.Time
	Version    int
}

// Decision is one approver's answer at one level, kept as the trail (UC-SVC-09 check 3): who,
// for whom while they were away, when, and the entry's version they decided on.
type Decision struct {
	ID          shared.ID
	ApprovalID  shared.ID
	Level       int
	ApproverID  shared.ID
	OnBehalfOf  shared.ID
	Verdict     Verdict
	Reason      string
	ItemVersion int
	DecidedAt   time.Time
}

// DueAt answers when the current level is overdue; zero where the step sets no limit or the
// approval is decided.
func (a Approval) DueAt() time.Time {
	if a.DueHours == 0 || a.State != ApprovalPending {
		return time.Time{}
	}
	return a.LevelSince.Add(time.Duration(a.DueHours) * time.Hour)
}

// NewApproval opens an approval of a step on an entry, with the people each level counts.
func NewApproval(
	id shared.ID, item work.WorkItem, workflow string, step work.WorkflowStep, counted [][]shared.ID,
	requestedBy shared.ID, now time.Time,
) (Approval, error) {
	if step.Approval == nil || len(counted) != len(step.Approval.Levels) {
		return Approval{}, shared.ErrConflict.WithDetail("approvals.step_without_approval")
	}
	levels := make([]OpenLevel, 0, len(counted))
	for i, level := range step.Approval.Levels {
		people := distinctIDs(counted[i])
		if level.Counted() && len(people) == 0 || level.Rule == work.ApprovalNOf && len(level.Roles) == 0 && len(people) < level.Count {
			return Approval{}, shared.ErrConflict.WithDetail("approvals.too_few_approvers").
				WithParams(map[string]string{"level": strconv.Itoa(i + 1)})
		}
		levels = append(levels, OpenLevel{Level: level, Counted: people})
	}
	return Approval{
		ID: id, TenantID: item.TenantID, ItemID: item.ID, Workflow: workflow, Step: step.Key,
		ItemVersion: item.Version, Levels: levels, DeclineTo: step.Approval.DeclineTo,
		DueHours: step.Approval.DueHours, State: ApprovalPending, RequestedBy: requestedBy,
		RequestedAt: now, LevelSince: now, Version: 1,
	}, nil
}

// Eligibility is how the deciding person stands at the current level: named or counted there
// directly, holding one of its roles, or deciding for a counted person who is away.
type Eligibility struct {
	Direct     bool
	OnBehalfOf shared.ID
}

// Decide records an answer at the current level and moves the approval on: the level passes
// when enough said yes, the approval is approved when the last level passes, and one decline
// declines it. decided are the answers already given at this level.
func Decide(
	a Approval, decided []Decision, id, approver shared.ID, who Eligibility, verdict Verdict, reason string,
	itemVersion int, now time.Time,
) (Approval, Decision, error) {
	if a.State != ApprovalPending {
		return Approval{}, Decision{}, shared.ErrConflict.WithDetail("approvals.decided")
	}
	if !who.Direct && who.OnBehalfOf.IsZero() {
		return Approval{}, Decision{}, shared.ErrForbidden.WithDetail("approvals.not_an_approver")
	}
	for _, earlier := range decided {
		if earlier.Level == a.Current && (earlier.ApproverID == approver || earlier.OnBehalfOf == approver ||
			(!who.OnBehalfOf.IsZero() && (earlier.OnBehalfOf == who.OnBehalfOf || earlier.ApproverID == who.OnBehalfOf))) {
			return Approval{}, Decision{}, shared.ErrConflict.WithDetail("approvals.already_decided")
		}
	}
	if verdict != VerdictApprove && verdict != VerdictDecline {
		return Approval{}, Decision{}, invalid("/verdict", "approvals.verdict_unknown")
	}
	reason = strings.TrimSpace(reason)
	if verdict == VerdictDecline && reason == "" {
		return Approval{}, Decision{}, invalid("/reason", "approvals.reason_required")
	}
	if utf8.RuneCountInString(reason) > MaxApprovalReason {
		return Approval{}, Decision{}, invalid("/reason", "approvals.reason_too_long")
	}
	decision := Decision{
		ID: id, ApprovalID: a.ID, Level: a.Current, ApproverID: approver, OnBehalfOf: who.OnBehalfOf,
		Verdict: verdict, Reason: reason, ItemVersion: itemVersion, DecidedAt: now,
	}
	next := a
	next.Version++
	if verdict == VerdictDecline {
		next.State, next.DecidedAt = ApprovalDeclined, &now
		return next, decision, nil
	}
	yes := 1
	for _, earlier := range decided {
		if earlier.Level == a.Current && earlier.Verdict == VerdictApprove {
			yes++
		}
	}
	level := a.Levels[a.Current]
	if yes < level.Level.Needed(len(level.Counted)) {
		return next, decision, nil
	}
	if a.Current+1 < len(a.Levels) {
		next.Current, next.LevelSince = a.Current+1, now
		return next, decision, nil
	}
	next.State, next.DecidedAt = ApprovalApproved, &now
	return next, decision, nil
}

// Withdrawn answers the approval closed without a decision, because its entry moved on.
func Withdrawn(a Approval, now time.Time) Approval {
	a.State, a.DecidedAt = ApprovalWithdrawn, &now
	a.Version++
	return a
}

// Counts reports whether a person is among the current level's counted approvers.
func (a Approval) Counts(person shared.ID) bool {
	return a.State == ApprovalPending && slices.Contains(a.Levels[a.Current].Counted, person)
}

func distinctIDs(ids []shared.ID) []shared.ID {
	out := []shared.ID{}
	for _, id := range ids {
		if !id.IsZero() && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// MaxDelegationDays bounds one absence a delegation covers.
const MaxDelegationDays = 366

// Delegation hands a person's approvals to somebody else while they are away (UC-SVC-10): from
// one moment to another, never to themselves. One per person; a new one replaces it.
type Delegation struct {
	TenantID   shared.ID
	AccountID  shared.ID
	DelegateID shared.ID
	From       time.Time
	Until      time.Time
}

// NewDelegation checks a delegation.
func NewDelegation(tenantID, accountID, delegateID shared.ID, from, until time.Time) (Delegation, error) {
	if delegateID.IsZero() || delegateID == accountID {
		return Delegation{}, invalid("/delegate_id", "approvals.delegate_self")
	}
	if !until.After(from) {
		return Delegation{}, invalid("/until", "approvals.delegation_empty")
	}
	if until.Sub(from) > MaxDelegationDays*24*time.Hour {
		return Delegation{}, invalid("/until", "approvals.delegation_too_long")
	}
	return Delegation{TenantID: tenantID, AccountID: accountID, DelegateID: delegateID, From: from.UTC(), Until: until.UTC()}, nil
}

// Covers reports whether the delegation is in force at a moment.
func (d Delegation) Covers(at time.Time) bool { return !at.Before(d.From) && at.Before(d.Until) }
