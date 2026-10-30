// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package agent is the governed agents module's domain (M9, F-12): an agent registered in a
// workspace as a non-human principal acting on a person's behalf, the models it may use, what it
// may do without a person and what waits for one, its budget, its kill switch, and the evaluation
// it passes before it goes live (research itsm-cmdb-ai §6.7, G1 … G9).
//
// An agent acts as its own service account, so what it reaches is that account's memberships; the
// person it acts for bounds it further - it never does what that person may not (G2). Nothing here
// holds a prompt, a tool's input or an answer: the module meters acts, it does not trace them.
package agent

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The bounds of the registry.
const (
	MaxNameLength    = 120
	MaxPurposeLength = 2000
	MaxScopes        = 64
	// MaxActionsPerHour and MaxTokensPerDay bound what a budget may allow; zero allows nothing.
	MaxActionsPerHour = 100000
	MaxTokensPerDay   = 100_000_000
	MaxConcurrentRuns = 100
)

// Kind is what an agent is: one of the four the product ships (G11), or one built elsewhere that
// reaches the workspace through MCP or the API and is governed the same way.
type Kind string

const (
	KindTicketTriage    Kind = "TICKET_TRIAGE"
	KindCISuggestion    Kind = "CI_SUGGESTION"
	KindDuplicateReview Kind = "DUPLICATE_REVIEW"
	KindKnowledgeDraft  Kind = "KNOWLEDGE_DRAFT"
	KindExternal        Kind = "EXTERNAL"
)

// Valid reports a kind this build knows.
func (k Kind) Valid() bool {
	switch k {
	case KindTicketTriage, KindCISuggestion, KindDuplicateReview, KindKnowledgeDraft, KindExternal:
		return true
	}
	return false
}

// Builtin reports one of the shipped agents, which the product itself runs.
func (k Kind) Builtin() bool { return k.Valid() && k != KindExternal }

// RiskClass is the agent's class under the AI Act as its registrant states it. HIGH is not
// registrable: no AI decides about people here (ai-first.md §2.1, Annex III point 4).
type RiskClass string

const (
	RiskMinimal RiskClass = "MINIMAL"
	// RiskLimited is an agent with a transparency duty (Art. 50): one that talks to a person.
	RiskLimited RiskClass = "LIMITED"
	RiskHigh    RiskClass = "HIGH"
)

// Status is where an agent stands in its life: drafted, approved by a person, live, stopped by
// its kill switch, or retired for good.
type Status string

const (
	StatusDraft     Status = "DRAFT"
	StatusApproved  Status = "APPROVED"
	StatusActive    Status = "ACTIVE"
	StatusSuspended Status = "SUSPENDED"
	StatusRetired   Status = "RETIRED"
)

// Class is the class of an act an agent attempts (G3): a read, a write that can be taken back, or
// one that cannot - a deletion, an act that needs a fresh sign-in, a mail to the outside.
type Class string

const (
	ClassRead         Class = "READ"
	ClassReversible   Class = "REVERSIBLE"
	ClassIrreversible Class = "IRREVERSIBLE"
)

// Rule is what happens to an act of a class: it runs, it runs and its owner is told, it waits for
// a person's approval, or it is never done.
type Rule string

const (
	RuleAllow    Rule = "ALLOW"
	RuleNotify   Rule = "NOTIFY"
	RuleApproval Rule = "APPROVAL"
	RuleNever    Rule = "NEVER"
)

// Policy is the human-in-the-loop policy per class. A read always runs; an irreversible act never
// runs without a person (G3), so its rule is APPROVAL or NEVER.
type Policy struct {
	Reversible   Rule
	Irreversible Rule
}

// DefaultPolicy is what an agent starts with: every write waits for a person (D-16).
func DefaultPolicy() Policy { return Policy{Reversible: RuleApproval, Irreversible: RuleApproval} }

// For answers the rule of a class.
func (p Policy) For(class Class) Rule {
	switch class {
	case ClassRead:
		return RuleAllow
	case ClassReversible:
		return p.Reversible
	}
	return p.Irreversible
}

// Budget bounds an agent (G6): AI tokens per day, acts per hour and runs at once. Zero is none at
// all, so a budget is stated rather than forgotten.
type Budget struct {
	TokensPerDay   int64
	ActionsPerHour int
	ConcurrentRuns int
}

// DefaultBudget is a small budget an agent is registered with unless it states one.
func DefaultBudget() Budget { return Budget{TokensPerDay: 200000, ActionsPerHour: 120, ConcurrentRuns: 2} }

// Agent is one registered agent.
type Agent struct {
	ID       shared.ID
	TenantID shared.ID
	// AccountID is the service account it acts as: its credentials are that account's tokens.
	AccountID shared.ID
	Name      string
	Purpose   string
	Kind      Kind
	// OwnerID is the person it acts for; it never does what they may not (G2).
	OwnerID   shared.ID
	RiskClass RiskClass
	// Scopes are the token scopes it may use; a token of its account holding more is narrowed.
	Scopes []string
	// ModelID is the registered model it may call, empty for an agent that calls none here.
	ModelID shared.ID
	Policy  Policy
	Budget  Budget
	// MinAccuracy is the share of its evaluation cases, in percent, a run must pass before it
	// goes live; zero asks for no evaluation.
	MinAccuracy int
	Status      Status
	// ApprovedBy and ApprovedAt name the person who approved it and when; ApprovedModelRevision
	// is the model's revision they approved, so a changed model asks for approval again (G9).
	ApprovedBy            shared.ID
	ApprovedAt            *time.Time
	ApprovedModelRevision int
	// SuspendedBy and SuspendedAt name who pulled its kill switch.
	SuspendedBy shared.ID
	SuspendedAt *time.Time
	CreatedBy   shared.ID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
}

// Check checks an agent's own fields as it is registered or changed.
func Check(a Agent, normalizer text.Normalizer) (Agent, error) {
	name := strings.TrimSpace(a.Name)
	if normalizer != nil {
		name = normalizer.NFC(name)
	}
	if name == "" {
		return Agent{}, invalid("/name", "agents.name_required")
	}
	if utf8.RuneCountInString(name) > MaxNameLength {
		return Agent{}, invalid("/name", "agents.name_too_long")
	}
	a.Name = name
	a.Purpose = strings.TrimSpace(a.Purpose)
	if normalizer != nil {
		a.Purpose = normalizer.NFC(a.Purpose)
	}
	if a.Purpose == "" {
		// G1: an agent with no stated purpose is one nobody can judge.
		return Agent{}, invalid("/purpose", "agents.purpose_required")
	}
	if utf8.RuneCountInString(a.Purpose) > MaxPurposeLength {
		return Agent{}, invalid("/purpose", "agents.purpose_too_long")
	}
	if !a.Kind.Valid() {
		return Agent{}, invalid("/kind", "agents.kind_invalid")
	}
	switch a.RiskClass {
	case RiskMinimal, RiskLimited:
	case RiskHigh:
		return Agent{}, invalid("/risk_class", "agents.high_risk_not_registrable")
	default:
		return Agent{}, invalid("/risk_class", "agents.risk_class_invalid")
	}
	if a.OwnerID.IsZero() {
		return Agent{}, invalid("/owner_id", "agents.owner_required")
	}
	if len(a.Scopes) > MaxScopes {
		return Agent{}, invalid("/scopes", "agents.too_many_scopes")
	}
	scopes := make([]string, 0, len(a.Scopes))
	for i, scope := range a.Scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || strings.ContainsAny(scope, " \t\n") {
			return Agent{}, invalid("/scopes/"+strconv.Itoa(i), "agents.scope_invalid")
		}
		if !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	slices.Sort(scopes)
	a.Scopes = scopes
	if err := checkPolicy(a.Policy); err != nil {
		return Agent{}, err
	}
	if err := checkBudget(a.Budget); err != nil {
		return Agent{}, err
	}
	if a.MinAccuracy < 0 || a.MinAccuracy > 100 {
		return Agent{}, invalid("/min_accuracy", "agents.min_accuracy_invalid")
	}
	return a, nil
}

func checkPolicy(p Policy) error {
	switch p.Reversible {
	case RuleAllow, RuleNotify, RuleApproval, RuleNever:
	default:
		return invalid("/policy/reversible", "agents.rule_invalid")
	}
	switch p.Irreversible {
	case RuleApproval, RuleNever:
	default:
		// G3: an act that cannot be taken back is never done without a person.
		return invalid("/policy/irreversible", "agents.irreversible_needs_a_person")
	}
	return nil
}

func checkBudget(b Budget) error {
	if b.TokensPerDay < 0 || b.TokensPerDay > MaxTokensPerDay {
		return invalid("/budget/tokens_per_day", "agents.budget_invalid")
	}
	if b.ActionsPerHour < 0 || b.ActionsPerHour > MaxActionsPerHour {
		return invalid("/budget/actions_per_hour", "agents.budget_invalid")
	}
	if b.ConcurrentRuns < 0 || b.ConcurrentRuns > MaxConcurrentRuns {
		return invalid("/budget/concurrent_runs", "agents.budget_invalid")
	}
	return nil
}

// Approve records a person's approval of a drafted agent, or of one whose model changed since.
// Nobody approves an agent they act through: its owner may not approve it (four eyes, G1).
func (a Agent) Approve(by shared.ID, modelRevision int, at time.Time) (Agent, error) {
	switch a.Status {
	case StatusDraft, StatusApproved, StatusActive, StatusSuspended:
	default:
		return Agent{}, transition(a.Status, StatusApproved)
	}
	if by == a.OwnerID {
		return Agent{}, shared.ErrForbidden.WithDetail("agents.owner_approves_not")
	}
	a.ApprovedBy, a.ApprovedAt, a.ApprovedModelRevision = by, &at, modelRevision
	if a.Status == StatusDraft {
		a.Status = StatusApproved
	}
	return a, nil
}

// NeedsApproval reports an agent whose approval does not cover its model as it now stands.
func (a Agent) NeedsApproval(modelRevision int) bool {
	return a.ApprovedAt == nil || (!a.ModelID.IsZero() && a.ApprovedModelRevision != modelRevision)
}

// Activate puts an approved agent live. Its approval must cover its model, and where it asks for
// an evaluation, the newest run on that model must have passed (G7).
func (a Agent) Activate(modelRevision int, latest *Run) (Agent, error) {
	if a.Status != StatusApproved && a.Status != StatusSuspended {
		return Agent{}, transition(a.Status, StatusActive)
	}
	if a.NeedsApproval(modelRevision) {
		return Agent{}, shared.ErrConflict.WithDetail("agents.approval_required")
	}
	if a.MinAccuracy > 0 {
		if latest == nil || latest.ModelRevision != modelRevision || !latest.PassedAt(a.MinAccuracy) {
			return Agent{}, shared.ErrConflict.WithDetail("agents.evaluation_required").
				WithParams(map[string]string{"min_accuracy": strconv.Itoa(a.MinAccuracy)})
		}
	}
	a.Status = StatusActive
	a.SuspendedBy, a.SuspendedAt = "", nil
	return a, nil
}

// Suspend is the kill switch: it stops the agent at once, whatever it was doing.
func (a Agent) Suspend(by shared.ID, at time.Time) (Agent, error) {
	if a.Status == StatusRetired {
		return Agent{}, transition(a.Status, StatusSuspended)
	}
	a.Status, a.SuspendedBy, a.SuspendedAt = StatusSuspended, by, &at
	return a, nil
}

// Retire ends an agent for good; a retired agent is never active again.
func (a Agent) Retire() (Agent, error) {
	if a.Status == StatusRetired {
		return Agent{}, transition(a.Status, StatusRetired)
	}
	a.Status = StatusRetired
	return a, nil
}

// Changed takes an agent's new fields: what it may do, its model and budget. A change to what it
// may do or call asks for approval again, so an approved agent goes back to draft; one that only
// spends less stays as it is.
func (a Agent) Changed(next Agent) Agent {
	widened := !slices.Equal(a.Scopes, next.Scopes) || a.ModelID != next.ModelID ||
		looser(a.Policy.Reversible, next.Policy.Reversible) || looser(a.Policy.Irreversible, next.Policy.Irreversible) ||
		next.MinAccuracy < a.MinAccuracy || a.OwnerID != next.OwnerID || a.Kind != next.Kind ||
		a.RiskClass != next.RiskClass
	next.ID, next.TenantID, next.AccountID = a.ID, a.TenantID, a.AccountID
	next.CreatedBy, next.CreatedAt, next.Version = a.CreatedBy, a.CreatedAt, a.Version
	next.Status, next.ApprovedBy, next.ApprovedAt, next.ApprovedModelRevision = a.Status, a.ApprovedBy, a.ApprovedAt, a.ApprovedModelRevision
	next.SuspendedBy, next.SuspendedAt = a.SuspendedBy, a.SuspendedAt
	if widened && a.Status != StatusRetired {
		next.Status = StatusDraft
		next.ApprovedBy, next.ApprovedAt, next.ApprovedModelRevision = "", nil, 0
	}
	return next
}

// strictness orders the rules from the loosest to the strictest.
func strictness(r Rule) int {
	switch r {
	case RuleAllow:
		return 0
	case RuleNotify:
		return 1
	case RuleApproval:
		return 2
	}
	return 3
}

func looser(was, now Rule) bool { return strictness(now) < strictness(was) }

// Narrow keeps of a credential's scopes those the agent may use.
func (a Agent) Narrow(scopes []string) []string {
	kept := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if slices.Contains(a.Scopes, scope) {
			kept = append(kept, scope)
		}
	}
	return kept
}

func transition(from, to Status) error {
	return shared.ErrConflict.WithDetail("agents.transition_invalid").
		WithParams(map[string]string{"from": string(from), "to": string(to)})
}

func invalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}
