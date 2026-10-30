// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Decision is what became of one act an agent attempted, as the ledger keeps it.
type Decision string

const (
	DecisionAllowed  Decision = "ALLOWED"
	DecisionNotified Decision = "NOTIFIED"
	DecisionHeld     Decision = "HELD"
	DecisionRefused  Decision = "REFUSED"
)

// Usage is what an agent spent, as the ledger counts it at the moment of an act.
type Usage struct {
	ActionsLastHour int
	TokensToday     int64
	RunsNow         int
}

// Situation is everything an act is judged on besides the act itself.
type Situation struct {
	// Halted is the workspace's kill switch: every agent stops.
	Halted bool
	// ModelRevision is the agent's model's revision now; ModelRetired whether it was retired.
	ModelRevision int
	ModelRetired  bool
	Usage         Usage
	// HourEnds is when the hour the acts are counted in renews.
	HourEnds time.Time
}

// Admit judges one act of an agent: whether it runs, runs with its owner told, waits for a
// person, or is refused - and with which code. The order is the kill switches first, then the
// approval, then the budget, then the policy, so a stopped agent is told it is stopped and not
// that it is over budget.
func (a Agent) Admit(class Class, s Situation) (Decision, error) {
	switch {
	case s.Halted:
		return DecisionRefused, shared.ErrForbidden.WithDetail("agents.halted")
	case a.Status == StatusSuspended:
		return DecisionRefused, shared.ErrForbidden.WithDetail("agents.suspended")
	case a.Status != StatusActive:
		return DecisionRefused, shared.ErrForbidden.WithDetail("agents.not_active").
			WithParams(map[string]string{"status": string(a.Status)})
	case !a.ModelID.IsZero() && (s.ModelRetired || a.NeedsApproval(s.ModelRevision)):
		return DecisionRefused, shared.ErrForbidden.WithDetail("agents.model_changed")
	}
	if class != ClassRead && s.Usage.ActionsLastHour >= a.Budget.ActionsPerHour {
		return DecisionRefused, shared.ErrRateLimited.WithDetail("agents.budget_spent").
			WithParams(map[string]string{"budget": "actions_per_hour", "renews_at": s.HourEnds.UTC().Format(time.RFC3339)})
	}
	switch a.Policy.For(class) {
	case RuleAllow:
		return DecisionAllowed, nil
	case RuleNotify:
		return DecisionNotified, nil
	case RuleApproval:
		return DecisionHeld, nil
	}
	return DecisionRefused, shared.ErrForbidden.WithDetail("agents.act_never").
		WithParams(map[string]string{"class": string(class)})
}

// MaySpend reports whether a run of an agent may spend AI tokens and start now (G6).
func (a Agent) MaySpend(u Usage) error {
	if a.Budget.TokensPerDay <= u.TokensToday {
		return shared.ErrRateLimited.WithDetail("agents.budget_spent").
			WithParams(map[string]string{"budget": "tokens_per_day"})
	}
	if a.Budget.ConcurrentRuns <= u.RunsNow {
		return shared.ErrRateLimited.WithDetail("agents.budget_spent").
			WithParams(map[string]string{"budget": "concurrent_runs", "runs": strconv.Itoa(u.RunsNow)})
	}
	return nil
}

// Action is one row of the ledger: an act an agent attempted, its class and what became of it,
// and the tokens a run spent. Never its input or answer (rule 10): the ledger meters, it does not
// trace.
type Action struct {
	ID       shared.ID
	AgentID  shared.ID
	UseCase  string
	Class    Class
	Decision Decision
	// Code is the refusal's message code, empty otherwise.
	Code      string
	Tokens    int64
	RequestID shared.ID
	At        time.Time
}

// RequestStatus is where a held act stands.
type RequestStatus string

const (
	RequestPending  RequestStatus = "PENDING"
	RequestApproved RequestStatus = "APPROVED"
	RequestDeclined RequestStatus = "DECLINED"
	RequestFailed   RequestStatus = "FAILED"
)

// Request is an act an agent attempted that waits for a person (G3): the use case and its input,
// held as the workspace's data until somebody with the right approves or declines it. Approving
// performs it as the approving person, so nobody approves what they could not do by hand.
type Request struct {
	ID        shared.ID
	AgentID   shared.ID
	UseCase   string
	Class     Class
	Input     map[string]any
	Status    RequestStatus
	DecidedBy shared.ID
	DecidedAt *time.Time
	// Code is the refusal's message code when performing it failed.
	Code      string
	CreatedAt time.Time
	Version   int
}

// Decide answers a pending request.
func (r Request) Decide(status RequestStatus, by shared.ID, at time.Time) (Request, error) {
	if r.Status != RequestPending {
		return Request{}, shared.ErrConflict.WithDetail("agents.request_decided")
	}
	r.Status, r.DecidedBy, r.DecidedAt = status, by, &at
	return r, nil
}
