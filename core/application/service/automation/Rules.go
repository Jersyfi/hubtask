// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	expression "github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/recurrence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	CreateRuleName  = "CreateRule"
	GetRuleName     = "GetRule"
	ListRulesName   = "ListRules"
	UpdateRuleName  = "UpdateRule"
	EnableRuleName  = "EnableRule"
	DisableRuleName = "DisableRule"
	DeleteRuleName  = "DeleteRule"

	// automationScope is the token scope every rule operation needs. The same one a webhook
	// subscription needs, and deliberately: subscribing an external system to the event stream and
	// writing a rule that reacts to it are the same power over the same stream.
	automationScope = "automation:manage"

	ruleTarget = "automation_rule"

	// The audit codes. A rule is a standing instruction to act on this workspace with somebody's
	// rights, so every change to one is an act a review looks for (audit.md §2) - and switching one
	// on is its own code, because letting a rule loose is the decision worth finding.
	RuleCreatedAction  audit.Action = "automation.rule_created"
	RuleUpdatedAction  audit.Action = "automation.rule_updated"
	RuleEnabledAction  audit.Action = "automation.rule_enabled"
	RuleDisabledAction audit.Action = "automation.rule_disabled"
	RuleDeletedAction  audit.Action = "automation.rule_deleted"
	// RuleReadAction is what a listing or a lookup performs.
	RuleReadAction audit.Action = "automation.rule_read"
)

// Authorizer is the decision point, as these use cases see it.
//
// Both halves, because the listing needs the quiet one. Authorize records a refusal in the trail;
// Permits answers the same question without recording anything, which is what a page filtering out
// rules the caller may not see has to do - ninety withheld rows is nobody trying anything, and an
// entry each would bury the refusals that matter (audit.md §4).
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
	Permits(ctx context.Context, actor appshared.ActorContext, request access.Request) (bool, error)
}

// Accounts is the slice of the account repository this package reads: one lookup, to find out what
// the rule would run as.
type Accounts interface {
	Find(ctx context.Context, accountID shared.ID) (identity.Account, error)
}

// Writer is what the rule use cases share.
//
// One struct rather than seven sets of the same six fields, for the reason the webhook writer is
// one: the six use cases are one aggregate's writers, and a field added to the set is added once.
type Writer struct {
	Rules repository.Rules
	// Schedules moves a rule's next moment when it is switched on. Separate from Rules for the
	// port's reason: advancing a moment is not editing a definition, and it deliberately does not
	// bump the version.
	Schedules   repository.Schedules
	Accounts    Accounts
	Memberships identityrepo.Memberships
	Catalogue   Catalogue
	// Conditions compiles a rule's expressions when it is written. A port, so that the use case
	// never learns which engine evaluates one (ADR-0009, rule 1).
	Conditions expression.Compiler
	// Expander works out when a SCHEDULE rule next fires, at the moment it is written (G-08).
	//
	// Here rather than only in the pass, for two reasons. A rule this installation cannot expand is
	// refused to its author while they are looking at it rather than failing at three in the
	// morning; and the moment has to be stored before the poller can find the rule at all.
	Expander recurrence.Expander
	// Jobs seeds this tenant's schedule poller. The write that makes something owed is what starts
	// it, because nothing may enumerate tenants (multi-tenancy.md §2.1).
	Jobs Queue
	// Encryptor seals an HTTP_REQUEST's header secret when the rule is written (E-02, T-21). The
	// application never stores a plaintext and the repository never holds a key: the sealing
	// happens here, between the two.
	Encryptor  crypto.Encryptor
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// schedule works out a rule's next moment and puts it on the rule.
//
// Called wherever a definition is written, so that the stored moment always belongs to the stored
// rule. It answers the zero time for the five triggers that are not a schedule, which is what makes
// it safe to call unconditionally.
func (w Writer) schedule(rule domain.Rule, after time.Time) (domain.Rule, error) {
	next, err := NextOccurrence(w.Expander, rule, after)
	if err != nil {
		return domain.Rule{}, err
	}
	rule.NextRunAt = next
	return rule, nil
}

// wake seeds or pulls forward this tenant's schedule poller.
//
// One job per tenant, rescheduling itself, seeded by the write that made something owed - the shape
// every per-tenant job in this system has. `Enqueue`'s conflict clause pulls an existing wake-up
// forward rather than adding a row, so a tenant with forty scheduled rules still has one job.
//
// Only for a rule that is *on*. A rule is written switched off, so nothing is owed until somebody
// enables it - and a poller seeded for a rule that does not act would wake up to find nothing due.
func (w Writer) wake(ctx context.Context, tenantID shared.ID, rule domain.Rule) error {
	if w.Jobs == nil || !rule.Enabled || rule.NextRunAt.IsZero() {
		return nil
	}
	_, err := w.Jobs.Enqueue(ctx, queue.Request{
		Kind:      queue.KindAutomationSchedule,
		TenantID:  tenantID,
		DedupeKey: tenantID.String(),
		RunAt:     rule.NextRunAt.UTC(),
	})
	return err
}

// CreateRule writes a rule, switched off.
type CreateRule struct{ Writer Writer }

// GetRule reads one.
type GetRule struct{ Writer Writer }

// ListRules reads a page.
type ListRules struct{ Writer Writer }

// UpdateRule changes what a rule does.
type UpdateRule struct{ Writer Writer }

// EnableRule switches one on.
type EnableRule struct{ Writer Writer }

// DisableRule switches one off.
type DisableRule struct{ Writer Writer }

// DeleteRule removes one, softly.
type DeleteRule struct{ Writer Writer }

// CreateRuleCommand is a new rule.
type CreateRuleCommand struct {
	Name       string
	Scope      domain.Scope
	RunAs      shared.ID
	Trigger    domain.Trigger
	Conditions []domain.Condition
	Actions    []domain.Action
	Throttle   domain.Throttle
	OnError    domain.OnError
}

// UpdateRuleCommand changes one. A nil pointer is a field the caller did not send.
type UpdateRuleCommand struct {
	ID              shared.ID
	Name            *string
	Scope           *domain.Scope
	RunAs           *shared.ID
	Trigger         *domain.Trigger
	Conditions      *[]domain.Condition
	Actions         *[]domain.Action
	Throttle        *domain.Throttle
	OnError         *domain.OnError
	ExpectedVersion int
}

// Execute writes the rule.
func (h CreateRule) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateRuleCommand,
) (domain.Rule, error) {
	w := h.Writer

	// Built before anything is authorised, because the scope the permission is asked about is the
	// rule's own and the aggregate is what validates it. A refusal here is about the rule's shape
	// and says nothing about who is asking, which is the right order: telling somebody they may not
	// write a malformed rule tells them the wrong thing.
	rule, err := domain.NewRule(domain.NewRuleInput{
		ID: w.IDs.NewID(), TenantID: actor.TenantID, Name: cmd.Name, Scope: cmd.Scope,
		RunAs: cmd.RunAs, Trigger: cmd.Trigger, Conditions: cmd.Conditions,
		Actions: cmd.Actions, Throttle: cmd.Throttle, OnError: cmd.OnError,
		CreatedBy: actor.AccountID, Now: w.Clock.Now(),
	})
	if err != nil {
		return domain.Rule{}, err
	}

	if err := w.authorizeWrite(ctx, actor, rule, RuleCreatedAction); err != nil {
		return domain.Rule{}, err
	}

	// A rule is written switched off, so nothing is owed yet - but the moment is worked out now
	// all the same, because this is where a recurrence this installation cannot read is refused to
	// the person who wrote it.
	rule, err = w.schedule(rule, w.Clock.Now())
	if err != nil {
		return domain.Rule{}, err
	}

	// After every check and before the write: from here on the rule stores ciphertext or nothing,
	// and the plaintext a caller sent exists nowhere (E-02, T-21).
	if err := sealOutboundSecrets(ctx, w.Encryptor, &rule, nil); err != nil {
		return domain.Rule{}, err
	}

	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return w.Rules.Insert(ctx, rule)
	})
	if err != nil {
		return domain.Rule{}, err
	}

	w.record(ctx, actor, RuleCreatedAction, rule, audit.SeverityNotice)
	return rule, nil
}

// Execute reads one rule.
func (h GetRule) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (domain.Rule, error) {
	w := h.Writer

	var rule domain.Rule
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var findErr error
		rule, findErr = w.Rules.Find(ctx, id)
		return findErr
	})
	if err != nil {
		return domain.Rule{}, err
	}

	// The permission is asked at the rule's own scope, after it is read: which scope decides is a
	// property of the rule, and a caller naming an identifier has not told us where it lives.
	if err := w.authorize(ctx, actor, rule.Scope, RuleReadAction, rule.ID); err != nil {
		return domain.Rule{}, err
	}
	return rule, nil
}

// Execute reads a page.
//
// The listing is bounded by the tenant and by nothing finer, and then filtered: a rule the caller
// may not see at its own scope is left out rather than refused, because a page is not a request for
// one object and a 403 for the whole page would hide the rules they may read.
func (h ListRules) Execute(
	ctx context.Context, actor appshared.ActorContext, query repository.Query,
) (repository.Page, error) {
	w := h.Writer

	if err := actor.RequireScope(automationScope); err != nil {
		return repository.Page{}, err
	}

	var page repository.Page
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var listErr error
		page, listErr = w.Rules.List(ctx, query)
		return listErr
	})
	if err != nil {
		return repository.Page{}, err
	}

	visible := make([]domain.Rule, 0, len(page.Rules))
	for _, rule := range page.Rules {
		allowed, err := w.permits(ctx, actor, rule.Scope)
		if err != nil {
			// Not "may not see": nobody was refused anything, the question could not be answered.
			// Reporting it as a refusal would silently shorten the page on a database blip.
			return repository.Page{}, err
		}
		if allowed {
			visible = append(visible, rule)
		}
	}
	page.Rules = visible
	return page, nil
}

// Execute changes a rule.
func (h UpdateRule) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateRuleCommand,
) (domain.Rule, error) {
	w := h.Writer

	var current domain.Rule
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var findErr error
		current, findErr = w.Rules.Find(ctx, cmd.ID)
		return findErr
	})
	if err != nil {
		return domain.Rule{}, err
	}

	// The permission at the rule as it stands, so that somebody who may not touch it at all is
	// refused before the new shape is even considered.
	if err := w.authorizeWrite(ctx, actor, current, RuleUpdatedAction); err != nil {
		return domain.Rule{}, err
	}

	wanted, err := merged(current, cmd, w.Clock.Now())
	if err != nil {
		return domain.Rule{}, err
	}

	// And again at the rule as it would be. A rule may not be edited into doing something its
	// writer may not do, and a rule may not be moved to a scope its writer does not hold - either
	// would be the same laundering the composition rule exists to stop, performed in two steps.
	if err := w.authorizeWrite(ctx, actor, wanted, RuleUpdatedAction); err != nil {
		return domain.Rule{}, err
	}

	// Recomputed with the definition rather than carried over: an edit may change the recurrence
	// rule or its zone, and a moment left pointing at an occurrence of a rule that no longer exists
	// is a rule that fires at a time nobody chose.
	wanted, err = w.schedule(wanted, w.Clock.Now())
	if err != nil {
		return domain.Rule{}, err
	}

	// A fresh secret is sealed; the mask copies the stored one forward from the same position
	// (E-02, T-21). After this line the new definition carries ciphertext or nothing.
	if err := sealOutboundSecrets(ctx, w.Encryptor, &wanted, &current); err != nil {
		return domain.Rule{}, err
	}

	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := w.Rules.Update(ctx, wanted, expectedOr(cmd.ExpectedVersion, current.Version)); err != nil {
			return err
		}
		return w.wake(ctx, actor.TenantID, wanted)
	})
	if err != nil {
		return domain.Rule{}, err
	}

	wanted.Version = current.Version + 1
	w.record(ctx, actor, RuleUpdatedAction, wanted, audit.SeverityNotice)
	return wanted, nil
}

// Execute switches a rule on.
func (h EnableRule) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (domain.Rule, error) {
	return h.Writer.setEnabled(ctx, actor, id, true, RuleEnabledAction)
}

// Execute switches a rule off.
func (h DisableRule) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (domain.Rule, error) {
	return h.Writer.setEnabled(ctx, actor, id, false, RuleDisabledAction)
}

// Execute removes a rule.
func (h DeleteRule) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) error {
	w := h.Writer

	var rule domain.Rule
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var findErr error
		rule, findErr = w.Rules.Find(ctx, id)
		return findErr
	})
	if err != nil {
		return err
	}

	// The rule as it stands, and not the composition rule: removing a rule takes a power away
	// rather than granting one, so asking the remover to hold what the rule could do would mean a
	// member who lost a right could no longer clean up after themselves.
	if err := w.authorize(ctx, actor, rule.Scope, RuleDeletedAction, rule.ID); err != nil {
		return err
	}

	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		_, deleteErr := w.Rules.Delete(ctx, id, w.Clock.Now())
		return deleteErr
	})
	if err != nil {
		return err
	}

	w.record(ctx, actor, RuleDeletedAction, rule, audit.SeverityNotice)
	return nil
}

// setEnabled is what the two switches share.
//
// The permission is re-checked here rather than trusted from whenever the rule was written: the
// writer's rights may have narrowed since, and letting a rule act on the workspace is exactly the
// moment to ask again. It is the composition check, not the plain one - switching a rule on is
// what makes its actions happen, so it needs the same rights writing them did.
func (w Writer) setEnabled(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, enabled bool,
	action audit.Action,
) (domain.Rule, error) {
	var rule domain.Rule
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var findErr error
		rule, findErr = w.Rules.Find(ctx, id)
		return findErr
	})
	if err != nil {
		return domain.Rule{}, err
	}

	if enabled {
		if err := w.authorizeWrite(ctx, actor, rule, action); err != nil {
			return domain.Rule{}, err
		}
	} else if err := w.authorize(ctx, actor, rule.Scope, action, rule.ID); err != nil {
		// Switching off needs the plain permission and no more. Stopping a rule is taking a power
		// away, and somebody who may manage rules here should never be unable to stop one.
		return domain.Rule{}, err
	}

	now := w.Clock.Now()
	if enabled {
		// Recomputed from *now* rather than fired from wherever the rule was left. A schedule that
		// has been switched off for a week owes nothing for that week: "from now on, at three in
		// the morning" is what somebody switching a rule on means, and firing the occurrences it
		// missed while it was off would be a burst nobody asked for.
		if rule, err = w.schedule(rule, now); err != nil {
			return domain.Rule{}, err
		}
	}

	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := w.Rules.SetEnabled(ctx, id, enabled, rule.Version, now); err != nil {
			return err
		}
		if !enabled {
			return nil
		}
		if w.Schedules == nil {
			return nil
		}
		if err := w.Schedules.SetNextRun(ctx, id, rule.NextRunAt); err != nil {
			return err
		}
		// The write that makes something owed seeds its own tenant's poller. Enabling is that
		// write: until now the rule was stored and doing nothing.
		return w.wake(ctx, actor.TenantID, rule.Enable(now))
	})
	if err != nil {
		return domain.Rule{}, err
	}

	if enabled {
		rule = rule.Enable(now)
	} else {
		rule = rule.Disable(now)
	}
	w.record(ctx, actor, action, rule, audit.SeverityNotice)
	return rule, nil
}

// authorize is the ordinary question: may this actor manage rules at this scope?
func (w Writer) authorize(
	ctx context.Context, actor appshared.ActorContext, scope domain.Scope,
	action audit.Action, target shared.ID,
) error {
	return w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionAutomation,
		Path:       scope.Path(),
		Action:     action,
		TokenScope: automationScope,
		TargetType: ruleTarget,
		TargetID:   target,
	})
}

// permits is the same question without an audit entry, for the listing.
func (w Writer) permits(
	ctx context.Context, actor appshared.ActorContext, scope domain.Scope,
) (bool, error) {
	return w.Authorizer.Permits(ctx, actor, access.Request{
		Permission: service.PermissionAutomation,
		Path:       scope.Path(),
		TokenScope: automationScope,
	})
}

// authorizeWrite is the ordinary question plus the composition rule, and the composition rule is
// this task's sharpest decision (automation.md §2).
//
// Writing a rule is not doing what the rule does - it is arranging for it to be done later, by
// somebody else's account, without anybody looking. So the automation permission alone is not
// enough: a member holds it, holds nothing else, and would otherwise write a rule that runs as a
// generously-scoped service account and restructures a hub they may not touch. The rights would
// have been laundered through the `run_as`.
//
// Two halves close it, and neither alone does:
//
//   - **You cannot delegate more than you hold.** The `run_as` account's effective role at the
//     rule's scope may not exceed the writer's own there. That is the general form of the leak and
//     it needs no list: whatever a service account can do, the writer could already have done.
//   - **You must hold what the actions ask for.** Every action's use case declares the scope a
//     credential needs, and the writer's own credential has to carry it. This is the credential
//     half of the same sentence, and it is read off the catalogue rather than restated - a use case
//     says what it needs in exactly one place.
//
// A person's account is refused as a `run_as` outright unless it is the writer's own. Acting as a
// colleague is impersonation, and no amount of automation permission is a grant of it - the role
// comparison would let an owner do it, and an owner writing rules that act as a named colleague is
// precisely what the audit trail exists to make impossible to fake.
//
// The run time re-checks all of it as the boundary (automation.md §2, rule 2). This is the courtesy
// that tells the writer now rather than at three in the morning - and a role change between the two
// narrows the rule rather than widening a stale check, because the engine asks the authoriser
// again and gets the answer of the day.
func (w Writer) authorizeWrite(
	ctx context.Context, actor appshared.ActorContext, rule domain.Rule, action audit.Action,
) error {
	if err := w.authorize(ctx, actor, rule.Scope, action, rule.ID); err != nil {
		return err
	}

	// The conditions before the actions, because a caller who wrote both wants to hear about both -
	// and a rule refused for its actions with an unparseable condition still in it would be refused
	// twice, once per round trip.
	if err := checkConditions(w.Conditions, rule); err != nil {
		return err
	}
	checked, err := checkActions(w.Catalogue, rule.Actions)
	if err != nil {
		return err
	}
	for _, scope := range requiredScopes(checked) {
		if !actor.HasScope(scope) {
			return shared.ErrForbidden.
				WithDetail("automation.writer_lacks_action_right").
				WithParams(map[string]string{"scope": scope})
		}
	}

	return w.canDelegateTo(ctx, actor, rule)
}

// canDelegateTo answers whether this writer may make a rule act as this account.
func (w Writer) canDelegateTo(
	ctx context.Context, actor appshared.ActorContext, rule domain.Rule,
) error {
	if rule.RunAs == actor.AccountID {
		// A rule that acts as its writer delegates nothing.
		return nil
	}

	var (
		runAs  identity.Account
		mine   []identity.Membership
		theirs []identity.Membership
	)
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var findErr error
		if runAs, findErr = w.Accounts.Find(ctx, rule.RunAs); findErr != nil {
			return findErr
		}
		path := rule.Scope.Path()
		if mine, findErr = w.Memberships.Along(ctx, actor.AccountID, path); findErr != nil {
			return findErr
		}
		theirs, findErr = w.Memberships.Along(ctx, rule.RunAs, path)
		return findErr
	})
	if err != nil {
		return err
	}

	if runAs.Kind != identity.AccountServiceAccount {
		return shared.ErrForbidden.
			WithDetail("automation.run_as_not_delegable").
			WithParams(map[string]string{"run_as": rule.RunAs.String()})
	}
	if runAs.Status != identity.AccountActive {
		return shared.ErrValidation.
			WithDetail("automation.run_as_inactive").
			WithFields(shared.FieldError{Path: "/run_as", Code: "automation.run_as_inactive"})
	}

	path := rule.Scope.Path()
	theirRole, theyHold := service.EffectiveRole(theirs, path)
	if !theyHold {
		// An account with no role at the scope can do nothing there, so delegating to it grants
		// nothing. A rule that does nothing is a rule somebody has misconfigured rather than a
		// privilege problem, and the run will say so.
		return nil
	}
	myRole, iHold := service.EffectiveRole(mine, path)
	if !iHold || !myRole.AtLeast(theirRole) {
		return shared.ErrForbidden.
			WithDetail("automation.run_as_exceeds_writer").
			WithParams(map[string]string{"run_as": rule.RunAs.String()})
	}
	return nil
}

// record writes the audit entry. Never the rule's name: a title is something somebody wrote, and no
// user content goes into the trail (rule 10, ADR-0017).
func (w Writer) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action,
	rule domain.Rule, severity audit.Severity,
) {
	if w.Audit == nil {
		return
	}
	_ = w.Audit.Append(ctx, audit.Entry{
		TenantID:   rule.TenantID,
		OccurredAt: w.Clock.Now(),
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   severity,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		// The account the rule would act as, which is what OnBehalfOf is for (audit.md §2): a
		// review reading "this rule was switched on" needs to know whose rights it was switched on
		// with, and that is not the person who pressed the button.
		OnBehalfOf: rule.RunAs,
		TargetType: ruleTarget,
		TargetID:   rule.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
	})
}

// merged applies the change set to the rule as it stands, and validates the result whole.
//
// Whole rather than field by field, because the aggregate's rules are about combinations - a
// trigger's fields belong to its kind, and an edit that changes the kind and leaves a field behind
// is exactly the case a per-field check would miss.
func merged(current domain.Rule, cmd UpdateRuleCommand, now time.Time) (domain.Rule, error) {
	in := domain.NewRuleInput{
		ID: current.ID, TenantID: current.TenantID,
		Name: current.Name, Scope: current.Scope, RunAs: current.RunAs,
		Trigger: current.Trigger, Conditions: current.Conditions, Actions: current.Actions,
		Throttle: current.Throttle, OnError: current.OnError,
		CreatedBy: current.CreatedBy, Now: current.CreatedAt,
	}
	if cmd.Name != nil {
		in.Name = *cmd.Name
	}
	if cmd.Scope != nil {
		in.Scope = *cmd.Scope
	}
	if cmd.RunAs != nil {
		in.RunAs = *cmd.RunAs
	}
	if cmd.Trigger != nil {
		in.Trigger = *cmd.Trigger
	}
	if cmd.Conditions != nil {
		in.Conditions = *cmd.Conditions
	}
	if cmd.Actions != nil {
		in.Actions = *cmd.Actions
	}
	if cmd.Throttle != nil {
		in.Throttle = *cmd.Throttle
	}
	if cmd.OnError != nil {
		in.OnError = *cmd.OnError
	}

	wanted, err := domain.NewRule(in)
	if err != nil {
		return domain.Rule{}, err
	}

	// What an edit may not touch: the identity, who wrote it, when, whether it is running, and how
	// many times it has failed. NewRule mints a rule rather than editing one, so the fields it
	// resets are carried over here explicitly - a field the constructor decides is a field an edit
	// would otherwise silently decide too.
	wanted.Enabled = current.Enabled
	wanted.FailureCount = current.FailureCount
	wanted.CreatedAt = current.CreatedAt
	wanted.UpdatedAt = now.UTC()
	wanted.Version = current.Version
	return wanted, nil
}

// expectedOr reads an absent expectation as the version just read. Zero is how a client that read
// nothing writes without pretending to have read something.
func expectedOr(expected, current int) int {
	if expected == 0 {
		return current
	}
	return expected
}
