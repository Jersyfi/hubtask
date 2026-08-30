// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package automation holds the rule: what starts a run, what has to be true for it to go on, and
// what it then does (automation.md §1, ADR-0009).
//
// A rule is data that will later be executed with somebody else's rights, which is what makes the
// validation here the substance of the aggregate rather than a formality around it. Two rules
// govern what this package accepts, and both are about the same thing - that a rule which cannot
// be run must not be storable:
//
//   - **What is accepted is what is executable.** A condition is a CEL expression and there is no
//     evaluator yet, so a non-empty condition is refused rather than stored. An action naming a
//     kind automation.md §1.3 documents and no release serves is refused for the same reason. The
//     alternative - accept it, ignore it - is a rule whose owner believes it is filtering and whose
//     behaviour says otherwise, which is the failure E-08 was built to avoid.
//   - **Refusals name what is wrong.** Every one carries a field path and a code, so that an editor
//     can point at the line and an agent can act on the answer rather than parse a sentence
//     (ai-first.md §1.2).
//
// What this package does *not* decide: who may write a rule, and whether the `run_as` account may
// do what the rule asks. Both are authorisation and both live in the application layer (ADR-0005,
// rule 2) - a domain model that consulted a role matrix would be the second place that answers a
// permission question.
package automation

import (
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxNameLength matches the contract. A rule's name is a title somebody wrote, so it is bounded
// like every other title rather than by the column's absence of a bound.
const MaxNameLength = 200

// MaxActions is how many steps one rule may take.
//
// Fifty is far past any rule a person writes and far short of a rule that would hold a worker for
// minutes. The bound exists because a rule is executed at least once per matching event, so its
// length multiplies by the workspace's traffic - and an unbounded list is a denial of service
// somebody can write through the ordinary API.
const MaxActions = 50

// MaxConditions bounds the other list for the same reason. Each is evaluated with its own timeout
// (automation.md §1.2), so the count is what bounds a match's total cost.
const MaxConditions = 20

// ScopeType is where a rule applies: the whole workspace, one hub, or one collection.
//
// The three levels `automation_rule.scope_type` has carried since 0001_init, and the same three
// identity.ScopeType names for containers - deliberately the same words, because the rule's scope
// is resolved against memberships and two vocabularies for one path would be two chances to get
// the resolution wrong.
type ScopeType string

const (
	ScopeTenant     ScopeType = "TENANT"
	ScopeHub        ScopeType = "HUB"
	ScopeCollection ScopeType = "COLLECTION"
)

// Scope is where the rule applies.
//
// Descendants are included, and there is no flag saying so. A membership held at a hub applies
// downwards already (domain-model.md §3.2), so a rule scoped to a hub sees what happens in its
// collections by the ordinary rule; a flag beside it could only ever contradict that.
type Scope struct {
	Type ScopeType
	// ID is the hub or the collection. Zero for TENANT, and required for the other two.
	ID shared.ID
}

// Path is the scope as the authoriser reads it: from the tenant downwards.
//
// It is here rather than in the application layer because it is the translation between this
// aggregate's two fields and the permission path, and a translation written at each call site is a
// translation one call site gets wrong.
func (s Scope) Path() []identity.Scope {
	switch s.Type {
	case ScopeHub:
		return []identity.Scope{identity.TenantScope(), identity.HubScope(s.ID)}
	case ScopeCollection:
		return []identity.Scope{identity.TenantScope(), identity.CollectionScope(s.ID)}
	default:
		return []identity.Scope{identity.TenantScope()}
	}
}

// TriggerKind is what starts a run (automation.md §1.1).
type TriggerKind string

const (
	// TriggerEvent is a domain event. The only kind an engine serves in this release; the other
	// four that are not the jumble's arrive with G-08, and the rule is stored switched off until
	// then like every newly written rule.
	TriggerEvent TriggerKind = "EVENT"
	// TriggerSchedule is a recurrence rule in a named zone.
	TriggerSchedule TriggerKind = "SCHEDULE"
	// TriggerRelativeDate is an offset from an instant on the entry: "24 hours before it is due".
	TriggerRelativeDate TriggerKind = "RELATIVE_DATE"
	// TriggerInboundWebhook is a token-protected address per rule.
	TriggerInboundWebhook TriggerKind = "INBOUND_WEBHOOK"
	// TriggerManual is a button, an API call, or an MCP tool.
	TriggerManual TriggerKind = "MANUAL"
	// TriggerJumbleEntry is a new arrival in the jumble.
	TriggerJumbleEntry TriggerKind = "JUMBLE_ENTRY"
)

// Valid reports whether the kind is one of the six. Asked where a kind arrives from outside the
// aggregate - a stored run, a job payload - rather than in ValidTrigger, which needs the switch
// anyway to check the fields each kind carries.
func (k TriggerKind) Valid() bool {
	switch k {
	case TriggerEvent, TriggerSchedule, TriggerRelativeDate,
		TriggerInboundWebhook, TriggerManual, TriggerJumbleEntry:
		return true
	default:
		return false
	}
}

// String is the stored value, which is also the contract's.
func (k TriggerKind) String() string { return string(k) }

// DateAnchor is the instant a RELATIVE_DATE trigger measures from.
type DateAnchor string

const (
	AnchorDueDate   DateAnchor = "DUE_DATE"
	AnchorCreatedAt DateAnchor = "CREATED_AT"
)

// OnError is what a failing action does to the rest of the run (automation.md §2).
type OnError string

const (
	// OnErrorStop ends the run at the failure. The default, because a rule whose third step failed
	// has not done what it says it does, and carrying on would leave the workspace in a state
	// nobody described.
	OnErrorStop OnError = "STOP"
	// OnErrorContinue runs the remaining actions anyway.
	OnErrorContinue OnError = "CONTINUE"
	// OnErrorRetry hands the run back to the queue with a backoff.
	OnErrorRetry OnError = "RETRY"
)

// Trigger is what starts a run, carrying the fields its own kind needs and no others.
//
// One struct with a kind rather than six types, because that is the shape the `trigger` column has
// always held and the shape the contract declares. What keeps it honest is that validation is per
// kind and refuses a field belonging to another one: a trigger carrying a cron expression and an
// event type is a rule nobody can read, and the day somebody edits one from a schedule into an
// event the leftover field would decide something.
type Trigger struct {
	Kind TriggerKind
	// EventType is EVENT's, and required there.
	EventType event.Type
	// ChangedFields narrows an `item.updated` trigger to the fields that moved, so a rule about
	// deadlines does not fire on a rename. EVENT's, and optional.
	ChangedFields []string
	// RRule and Timezone are SCHEDULE's, and both required there: a schedule without a zone is a
	// schedule that means something different in summer.
	RRule    string
	Timezone string
	// Anchor and Offset are RELATIVE_DATE's, and both required there.
	Anchor DateAnchor
	Offset string
}

// Action is one step of a run: a use case name in SCREAMING_SNAKE_CASE, and its parameters.
type Action struct {
	Kind   string
	Params map[string]any
}

// Condition is one expression that has to hold for the run to go on.
type Condition struct {
	Expr string
}

// Throttle bounds a storm. Both halves are stored here and observed by the engine.
type Throttle struct {
	MaxRunsPerHour int
	DedupeKeyExpr  string
}

// Rule is one automation.
type Rule struct {
	ID       shared.ID
	TenantID shared.ID
	Name     string
	Scope    Scope
	// Enabled is false on a rule that has just been written. Writing what a rule would do and
	// letting it loose on the workspace are two decisions, and a rule that acted the moment it was
	// saved would give nobody the chance to read it back first.
	Enabled bool
	// RunAs is the account the rule acts as. It can never do more than that account may
	// (automation.md §2), which is enforced where authorisation is - not here.
	RunAs      shared.ID
	Trigger    Trigger
	Conditions []Condition
	Actions    []Action
	Throttle   Throttle
	OnError    OnError
	// FailureCount is the run of failed runs. A run of them disables the rule by itself, and
	// enabling it by hand clears the count.
	FailureCount int
	CreatedBy    shared.ID
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
	Version      int
}

// NewRuleInput is what writing a rule needs.
type NewRuleInput struct {
	ID       shared.ID
	TenantID shared.ID
	Name     string
	Scope    Scope
	RunAs    shared.ID
	Trigger  Trigger
	// Conditions are checked for their shape here and compiled by the application layer, which is
	// the only side that may hold an evaluator.
	Conditions []Condition
	Actions    []Action
	Throttle   Throttle
	OnError    OnError
	CreatedBy  shared.ID
	Now        time.Time
}

// NewRule validates and builds a rule, switched off.
func NewRule(in NewRuleInput) (Rule, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() || in.CreatedBy.IsZero() {
		return Rule{}, shared.ErrInternal.WithDetail("automation.rule_incomplete")
	}

	name, err := RuleName(in.Name)
	if err != nil {
		return Rule{}, err
	}
	scope, err := ValidScope(in.Scope)
	if err != nil {
		return Rule{}, err
	}
	if in.RunAs.IsZero() {
		return Rule{}, fieldError("/run_as", "automation.run_as_required")
	}
	trigger, err := ValidTrigger(in.Trigger)
	if err != nil {
		return Rule{}, err
	}
	conditions, err := ValidConditionShape(in.Conditions)
	if err != nil {
		return Rule{}, err
	}
	actions, err := ValidActionShape(in.Actions)
	if err != nil {
		return Rule{}, err
	}
	throttle, err := ValidThrottle(in.Throttle)
	if err != nil {
		return Rule{}, err
	}
	onError, err := ValidOnError(in.OnError)
	if err != nil {
		return Rule{}, err
	}

	now := in.Now.UTC()
	return Rule{
		ID: in.ID, TenantID: in.TenantID, Name: name, Scope: scope,
		Enabled: false,
		RunAs:   in.RunAs, Trigger: trigger, Conditions: conditions, Actions: actions,
		Throttle: throttle, OnError: onError,
		CreatedBy: in.CreatedBy, CreatedAt: now, UpdatedAt: now, Version: 1,
	}, nil
}

// RuleName checks a title somebody wrote.
func RuleName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	switch {
	case name == "":
		return "", fieldError("/name", "automation.name_required")
	case len([]rune(name)) > MaxNameLength:
		return "", fieldError("/name", "automation.name_too_long")
	}
	return name, nil
}

// ValidScope checks that the scope names what its level needs.
func ValidScope(scope Scope) (Scope, error) {
	switch scope.Type {
	case ScopeTenant:
		if !scope.ID.IsZero() {
			// Refused rather than ignored: a caller that named a hub and a tenant scope means one
			// of the two, and guessing which would be guessing at what the rule watches.
			return Scope{}, fieldError("/scope/id", "automation.scope_id_not_allowed")
		}
		return Scope{Type: ScopeTenant}, nil
	case ScopeHub, ScopeCollection:
		if scope.ID.IsZero() {
			return Scope{}, fieldError("/scope/id", "automation.scope_id_required")
		}
		return scope, nil
	default:
		return Scope{}, fieldError("/scope/type", "automation.scope_type_unknown")
	}
}

// ValidTrigger checks the kind and the fields that kind needs, and refuses the fields it does not.
func ValidTrigger(trigger Trigger) (Trigger, error) {
	switch trigger.Kind {
	case TriggerEvent:
		if trigger.EventType == "" {
			return Trigger{}, fieldError("/trigger/event_type", "automation.event_type_required")
		}
		if !trigger.EventType.Valid() {
			return Trigger{}, shared.ErrValidation.
				WithDetail("automation.event_type_unknown").
				WithParams(map[string]string{"event_type": trigger.EventType.String()}).
				WithFields(shared.FieldError{
					Path: "/trigger/event_type", Code: "automation.event_type_unknown",
				})
		}
		kept := Trigger{
			Kind: TriggerEvent, EventType: trigger.EventType,
			ChangedFields: trimmedFields(trigger.ChangedFields),
		}
		return kept, foreign(trigger, kept)
	case TriggerSchedule:
		if strings.TrimSpace(trigger.RRule) == "" {
			return Trigger{}, fieldError("/trigger/rrule", "automation.rrule_required")
		}
		if strings.TrimSpace(trigger.Timezone) == "" {
			return Trigger{}, fieldError("/trigger/timezone", "automation.timezone_required")
		}
		if _, err := time.LoadLocation(trigger.Timezone); err != nil {
			// By name, because a zone the platform does not know is a schedule that would fire at
			// a time nobody chose - and the caller can only fix what they are told.
			return Trigger{}, shared.ErrValidation.
				WithDetail("automation.timezone_unknown").
				WithParams(map[string]string{"timezone": trigger.Timezone}).
				WithFields(shared.FieldError{
					Path: "/trigger/timezone", Code: "automation.timezone_unknown",
				})
		}
		kept := Trigger{
			Kind:  TriggerSchedule,
			RRule: strings.TrimSpace(trigger.RRule), Timezone: trigger.Timezone,
		}
		return kept, foreign(trigger, kept)
	case TriggerRelativeDate:
		switch trigger.Anchor {
		case AnchorDueDate, AnchorCreatedAt:
		case "":
			return Trigger{}, fieldError("/trigger/anchor", "automation.anchor_required")
		default:
			return Trigger{}, fieldError("/trigger/anchor", "automation.anchor_unknown")
		}
		offset, err := ValidOffset(trigger.Offset)
		if err != nil {
			return Trigger{}, err
		}
		kept := Trigger{Kind: TriggerRelativeDate, Anchor: trigger.Anchor, Offset: offset}
		return kept, foreign(trigger, kept)
	case TriggerInboundWebhook, TriggerManual, TriggerJumbleEntry:
		// Three kinds that configure nothing on the rule. The address an INBOUND_WEBHOOK answers
		// is a credential minted beside the rule rather than a field on it, and the other two are
		// started by somebody rather than by a setting.
		kept := Trigger{Kind: trigger.Kind}
		return kept, foreign(trigger, kept)
	case "":
		return Trigger{}, fieldError("/trigger/kind", "automation.trigger_kind_required")
	default:
		return Trigger{}, shared.ErrValidation.
			WithDetail("automation.trigger_kind_unknown").
			WithParams(map[string]string{"kind": string(trigger.Kind)}).
			WithFields(shared.FieldError{
				Path: "/trigger/kind", Code: "automation.trigger_kind_unknown",
			})
	}
}

// ValidOffset checks the signed ISO 8601 duration a RELATIVE_DATE trigger measures by.
//
// Parsed here rather than left to the scheduler, because a trigger this system cannot read is a
// rule that would fail at a moment nobody is watching - and the writer is the one person who can
// still fix it.
func ValidOffset(raw string) (string, error) {
	offset := strings.TrimSpace(raw)
	if offset == "" {
		return "", fieldError("/trigger/offset", "automation.offset_required")
	}
	if _, err := parseOffset(offset); err != nil {
		return "", shared.ErrValidation.
			WithDetail("automation.offset_invalid").
			WithParams(map[string]string{"offset": offset}).
			WithFields(shared.FieldError{Path: "/trigger/offset", Code: "automation.offset_invalid"})
	}
	return offset, nil
}

// MaxOffset is how far from its anchor a RELATIVE_DATE trigger may reach. A year is past any
// "remind me before this is due" a person writes, and short enough that a typo of one digit cannot
// schedule something after everyone who wrote it has left.
const MaxOffset = 365 * 24 * time.Hour

// parseOffset reads a signed ISO 8601 duration of weeks down to seconds.
//
// Written here rather than taken from a library, for the reason core/domain has no libraries at all
// (ADR-0001, rule 1) - and here rather than borrowed from the reminders, because that one is bounded
// by the reminder's own maximum and an aggregate reaching into another aggregate for arithmetic is
// a coupling neither of them asked for. Reminder.go says the same thing about the same twenty lines.
//
// Years and months are refused rather than converted. "One month before it is due" is a different
// instant depending on which month, and a trigger that resolved it here would be deciding a calendar
// question in a parser.
func parseOffset(text string) (time.Duration, error) {
	sign := time.Duration(1)
	switch {
	case strings.HasPrefix(text, "-"):
		sign, text = -1, text[1:]
	case strings.HasPrefix(text, "+"):
		text = text[1:]
	}
	if !strings.HasPrefix(text, "P") {
		return 0, errOffsetMalformed
	}

	date, clock, timed := strings.Cut(text[1:], "T")
	if timed && clock == "" {
		return 0, errOffsetMalformed
	}

	var total time.Duration
	components := 0
	for _, part := range []struct {
		text  string
		units map[byte]time.Duration
	}{
		{date, map[byte]time.Duration{'W': 7 * 24 * time.Hour, 'D': 24 * time.Hour}},
		{clock, map[byte]time.Duration{'H': time.Hour, 'M': time.Minute, 'S': time.Second}},
	} {
		rest := part.text
		for rest != "" {
			count, unit, remainder, err := cutComponent(rest)
			if err != nil {
				return 0, err
			}
			scale, known := part.units[unit]
			if !known {
				// 'M' before the T is months and after it is minutes, which is why the two halves
				// are read against two tables rather than one.
				return 0, errOffsetMalformed
			}
			total += time.Duration(count) * scale
			if total > MaxOffset {
				return 0, errOffsetMalformed
			}
			components++
			rest = remainder
		}
	}
	if components == 0 {
		return 0, errOffsetMalformed
	}
	return sign * total, nil
}

// cutComponent takes one `<digits><unit>` off the front.
func cutComponent(text string) (count int, unit byte, rest string, err error) {
	digits := 0
	for digits < len(text) && text[digits] >= '0' && text[digits] <= '9' {
		count = count*10 + int(text[digits]-'0')
		digits++
		if count > int(MaxOffset/time.Second) {
			return 0, 0, "", errOffsetMalformed
		}
	}
	if digits == 0 || digits == len(text) {
		return 0, 0, "", errOffsetMalformed
	}
	return count, text[digits], text[digits+1:], nil
}

// errOffsetMalformed never leaves this package: ValidOffset turns it into the coded refusal, so the
// caller is told which field and which code rather than a sentence about a grammar.
var errOffsetMalformed = shared.ErrValidation.WithDetail("automation.offset_invalid")

// foreign refuses a field belonging to another kind of trigger.
//
// The comparison is against what validation kept rather than against a list of names, so a field
// added to Trigger tomorrow is covered by this the moment it is not copied into `kept` - which is
// the failure mode a list would have: it would be the one thing nobody updates.
func foreign(sent, kept Trigger) error {
	var findings []shared.FieldError
	if sent.EventType != kept.EventType {
		findings = append(findings, foreignField("event_type"))
	}
	if len(sent.ChangedFields) > 0 && len(kept.ChangedFields) == 0 {
		findings = append(findings, foreignField("changed_fields"))
	}
	if strings.TrimSpace(sent.RRule) != kept.RRule {
		findings = append(findings, foreignField("rrule"))
	}
	if strings.TrimSpace(sent.Timezone) != strings.TrimSpace(kept.Timezone) {
		findings = append(findings, foreignField("timezone"))
	}
	if sent.Anchor != kept.Anchor {
		findings = append(findings, foreignField("anchor"))
	}
	if strings.TrimSpace(sent.Offset) != kept.Offset {
		findings = append(findings, foreignField("offset"))
	}
	if len(findings) == 0 {
		return nil
	}

	slices.SortFunc(findings, func(a, b shared.FieldError) int { return strings.Compare(a.Path, b.Path) })
	return shared.ErrValidation.
		WithDetail("automation.trigger_field_not_for_kind").
		WithParams(map[string]string{"kind": string(kept.Kind)}).
		WithFields(findings...)
}

func foreignField(name string) shared.FieldError {
	return shared.FieldError{
		Path: "/trigger/" + name, Code: "automation.trigger_field_not_for_kind",
	}
}

// ValidConditionShape checks what a condition looks like, and nothing about what it means.
//
// An empty list is a rule with no conditions, which is a rule that runs on every match - that is
// what the model has always meant. An empty *expression* is not a condition at all and is refused
// as the empty field it is.
//
// Whether the text is an expression, and whether it names things that exist, is the compiler's
// question - and the compiler is a port the application layer holds, because core/domain may not
// reach for one (ADR-0001). Until G-06 there was no compiler and this function refused every
// non-empty condition; what replaced that refusal is a real check rather than the absence of one.
func ValidConditionShape(conditions []Condition) ([]Condition, error) {
	if len(conditions) > MaxConditions {
		return nil, shared.ErrValidation.
			WithDetail("automation.too_many_conditions").
			WithParams(map[string]string{"maximum": itoa(MaxConditions)}).
			WithFields(shared.FieldError{
				Path: "/conditions", Code: "automation.too_many_conditions",
			})
	}

	kept := make([]Condition, 0, len(conditions))
	var findings []shared.FieldError
	for i, condition := range conditions {
		expr := strings.TrimSpace(condition.Expr)
		if expr == "" {
			findings = append(findings, shared.FieldError{
				Path: "/conditions/" + itoa(i) + "/expr", Code: "automation.condition_empty",
			})
			continue
		}
		kept = append(kept, Condition{Expr: expr})
	}
	if len(findings) > 0 {
		return nil, shared.ErrValidation.
			WithDetail("automation.condition_empty").
			WithFields(findings...)
	}
	return kept, nil
}

// ValidActionShape checks what a rule's actions look like, and nothing about what they mean.
//
// Which kinds exist, and whether the parameters are ones their use case declares, is the
// application layer's question: the answer is the use case catalogue, and core/domain may not read
// it (ADR-0001). What is here is the shape - at least one action, not too many, every kind named.
func ValidActionShape(actions []Action) ([]Action, error) {
	switch {
	case len(actions) == 0:
		return nil, fieldError("/actions", "automation.actions_required")
	case len(actions) > MaxActions:
		return nil, shared.ErrValidation.
			WithDetail("automation.too_many_actions").
			WithParams(map[string]string{"maximum": itoa(MaxActions)}).
			WithFields(shared.FieldError{Path: "/actions", Code: "automation.too_many_actions"})
	}

	kept := make([]Action, 0, len(actions))
	var findings []shared.FieldError
	for i, action := range actions {
		kind := strings.TrimSpace(action.Kind)
		if kind == "" {
			findings = append(findings, shared.FieldError{
				Path: "/actions/" + itoa(i) + "/kind", Code: "automation.action_kind_required",
			})
			continue
		}
		params := action.Params
		if params == nil {
			// An action with no parameters is an action with an empty map, never a null. What is
			// stored is what is read back, and a null would come back as one.
			params = map[string]any{}
		}
		kept = append(kept, Action{Kind: kind, Params: params})
	}
	if len(findings) > 0 {
		return nil, shared.ErrValidation.
			WithDetail("automation.action_kind_required").
			WithFields(findings...)
	}
	return kept, nil
}

// ValidThrottle checks the bounds. The dedupe key is an expression like any other and is compiled
// where the conditions are - in the application layer, against the same environment and the same
// limits, because a key that could not be evaluated would collapse every run into one.
func ValidThrottle(throttle Throttle) (Throttle, error) {
	if throttle.MaxRunsPerHour < 0 {
		return Throttle{}, fieldError("/throttle/max_runs_per_hour", "automation.throttle_invalid")
	}
	return Throttle{
		MaxRunsPerHour: throttle.MaxRunsPerHour,
		DedupeKeyExpr:  strings.TrimSpace(throttle.DedupeKeyExpr),
	}, nil
}

// ValidOnError checks the three values, and reads an absent one as the default.
func ValidOnError(onError OnError) (OnError, error) {
	switch onError {
	case "":
		return OnErrorStop, nil
	case OnErrorStop, OnErrorContinue, OnErrorRetry:
		return onError, nil
	default:
		return "", shared.ErrValidation.
			WithDetail("automation.on_error_unknown").
			WithParams(map[string]string{"value": string(onError)}).
			WithFields(shared.FieldError{Path: "/on_error", Code: "automation.on_error_unknown"})
	}
}

// Enable switches the rule on and clears its failure count.
//
// The count goes because a rule somebody has looked at and turned back on is a rule whose run of
// failures has been dealt with. Leaving it would mean the next single failure disables the rule
// again, which is a rule that can never recover from one bad afternoon.
func (r Rule) Enable(now time.Time) Rule {
	r.Enabled, r.FailureCount = true, 0
	r.UpdatedAt = now.UTC()
	r.Version++
	return r
}

// Disable switches the rule off, and leaves the count alone: what a run of failures concluded is
// still true, and somebody turning the rule off by hand has not fixed it.
func (r Rule) Disable(now time.Time) Rule {
	r.Enabled = false
	r.UpdatedAt = now.UTC()
	r.Version++
	return r
}

// IsDeleted reports whether the rule is in the soft-deleted state its runs outlive.
func (r Rule) IsDeleted() bool { return r.DeletedAt != nil }

// EnsureVersion is the optimistic lock. Zero means the caller expressed no expectation, which is
// how a client that read nothing writes without pretending to have read something.
func (r Rule) EnsureVersion(expected int) error {
	if expected == 0 || expected == r.Version {
		return nil
	}
	return shared.ErrConflict.
		WithDetail("automation.version_conflict").
		WithParams(map[string]string{
			"expected": itoa(expected), "current": itoa(r.Version),
		})
}

func trimmedFields(fields []string) []string {
	kept := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

func fieldError(path, code string) error {
	return shared.ErrValidation.
		WithDetail(code).
		WithFields(shared.FieldError{Path: path, Code: code})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}
