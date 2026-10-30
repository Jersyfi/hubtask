// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"

	"github.com/Jersyfi/hubtask/core/application/condition"
	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	TriggerRuleManuallyName = "TriggerRuleManually"

	// RuleTriggeredAction is somebody running a rule by hand. Its own code rather than sharing the
	// run log's, because "this rule acted because a person asked" is exactly the entry a review is
	// looking for - the other five triggers have nobody to record.
	RuleTriggeredAction audit.Action = "automation.rule_triggered"
)

// TriggerRuleManually starts a run because somebody asked for one (automation.md §1.1).
//
// The smallest of the triggers, and deliberately so: it produces into the engine (`RunRule`)
// rather than running anything itself. What it decides is the two things a
// producer decides - may this actor start this rule, and what makes this press its own occasion -
// and the engine then answers everything else the same way it answers an event's run.
//
// It does **not** run the rule inside the request. Actions are writes performed as the `run_as`
// account, and a caller holding an HTTP connection open while a rule restructures a hub is a
// request whose timeout decides how much of the rule happened.
type TriggerRuleManually struct {
	Rules repository.Rules
	// Entries and Containers locate the entry a macro is pressed on (UC-SVC-06 check 4); without
	// them a press naming an entry is refused.
	Entries    Entries
	Containers Containers
	Jobs       Queue
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// TriggerRuleResult is what the caller is told: the run this press will produce.
//
// The identifier is minted here rather than by the engine, so that the answer is something a caller
// can watch for. The run row does not exist yet - a worker writes it when it claims the job - which
// is why the route answers 202 rather than the run itself.
type TriggerRuleResult struct {
	RunID  shared.ID
	RuleID shared.ID
}

// Execute queues the run.
func (h TriggerRuleManually) Execute(
	ctx context.Context, actor appshared.ActorContext, ruleID shared.ID,
) (TriggerRuleResult, error) {
	return h.ExecuteOn(ctx, actor, ruleID, "")
}

// ExecuteOn queues the run about one entry - a macro (UC-SVC-06): the rule's conditions read it as
// `item` and its actions take it where they name no entry. The presser must be allowed to change
// the entry and the entry must lie in the rule's scope, so a macro is no way to reach an entry the
// presser could not change by hand.
func (h TriggerRuleManually) ExecuteOn(
	ctx context.Context, actor appshared.ActorContext, ruleID, itemID shared.ID,
) (TriggerRuleResult, error) {
	var rule domain.Rule
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var findErr error
		rule, findErr = h.Rules.Find(ctx, ruleID)
		return findErr
	})
	if err != nil {
		return TriggerRuleResult{}, err
	}

	// The plain permission at the rule's own scope, and not the composition check writing one
	// needs. Pressing the button on a rule that already exists changes nothing about what the rule
	// may do: it was checked when it was written and it is checked again, per action, when it runs
	// (automation.md §2.1). Asking the writer's question here would mean somebody who may manage
	// this rule could not run the rule they are looking at.
	if err := h.authorize(ctx, actor, rule); err != nil {
		return TriggerRuleResult{}, err
	}
	if !itemID.IsZero() {
		if err := h.authorizeSubject(ctx, actor, rule, itemID); err != nil {
			return TriggerRuleResult{}, err
		}
	}

	if rule.Trigger.Kind != domain.TriggerManual {
		// Refused by name rather than queued to do nothing. The engine would drop the job on the
		// same comparison, and a call that appears to work and silently does nothing is the
		// failure automation.md §2.2 exists to avoid.
		return TriggerRuleResult{}, shared.ErrValidation.
			WithDetail("automation.trigger_not_manual").
			WithParams(map[string]string{"kind": rule.Trigger.Kind.String()})
	}
	if !rule.Enabled {
		return TriggerRuleResult{}, shared.ErrConflict.
			WithDetail("automation.rule_not_enabled").
			WithParams(map[string]string{"rule_id": rule.ID.String()})
	}

	runID := h.IDs.NewID()
	payload := map[string]any{
		"rule_id":      rule.ID.String(),
		"trigger":      string(domain.TriggerManual),
		"run_id":       runID.String(),
		"triggered_by": actor.AccountID.String(),
		// Each press is its own occasion, so two presses are two sets of idempotency keys
		// and the second really acts. The run identifier is what makes them distinct, and
		// it is already unique per press.
		"occasion": "manual:" + runID.String(),
	}
	if !itemID.IsZero() {
		payload["subject_id"] = itemID.String()
	}
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		_, enqueueErr := h.Jobs.Enqueue(ctx, queue.Request{
			Kind:     queue.KindAutomationRun,
			TenantID: actor.TenantID,
			Payload:  payload,
			// The same key, so that a client that retries the call while the first job is still
			// pending gets the one run it asked for rather than two.
			DedupeKey: ConsumerName + ":manual:" + runID.String(),
		})
		return enqueueErr
	})
	if err != nil {
		return TriggerRuleResult{}, err
	}

	h.record(ctx, actor, rule, runID)
	return TriggerRuleResult{RunID: runID, RuleID: rule.ID}, nil
}

// authorizeSubject asks what changing the entry by hand asks, at its collection, and refuses an
// entry outside the rule's scope by name.
func (h TriggerRuleManually) authorizeSubject(
	ctx context.Context, actor appshared.ActorContext, rule domain.Rule, itemID shared.ID,
) error {
	if h.Entries == nil || h.Containers == nil {
		return shared.ErrInternal.WithDetail("automation.subject_unavailable")
	}
	var (
		item       work.WorkItem
		collection work.Container
	)
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if item, err = h.Entries.Find(ctx, itemID); err != nil {
			return err
		}
		collection, err = h.Containers.Find(ctx, item.CollectionID)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) || (err == nil && item.DeletedAt != nil) {
		return appshared.ItemNotFound(itemID)
	}
	if err != nil {
		return err
	}
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems, Path: service.ContainerScopes(collection),
		Action: RuleTriggeredAction, TokenScope: automationScope, TargetType: "work_item", TargetID: itemID,
		On: access.ItemSubject{Does: service.ItemChange, ID: itemID, Assignee: item.AssigneeID},
	}); err != nil {
		return err
	}
	at := condition.Location{CollectionID: collection.ID, HubID: collection.ParentID}
	if !covers(rule.Scope, at) {
		return shared.ErrValidation.WithDetail("automation.subject_outside_scope").
			WithFields(shared.FieldError{Path: "/item_id", Code: "automation.subject_outside_scope"})
	}
	return nil
}

func (h TriggerRuleManually) authorize(
	ctx context.Context, actor appshared.ActorContext, rule domain.Rule,
) error {
	return h.Authorizer.Authorize(ctx, actor, runRequest(rule.Scope, RuleTriggeredAction, rule.ID))
}

// record writes the entry that says a person did this. Never the rule's name (rule 10), and
// `OnBehalfOf` is the account the run will act as - which is the half a review needs and the half
// the actor field cannot carry.
func (h TriggerRuleManually) record(
	ctx context.Context, actor appshared.ActorContext, rule domain.Rule, runID shared.ID,
) {
	if h.Audit == nil {
		return
	}
	_ = h.Audit.Append(ctx, audit.Entry{
		TenantID:   rule.TenantID,
		OccurredAt: h.Clock.Now(),
		Action:     RuleTriggeredAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		OnBehalfOf: rule.RunAs,
		TargetType: ruleTarget,
		TargetID:   rule.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "run_id", Classification: audit.Open, To: runID.String()},
		),
	})
}

// Descriptor is the catalogue entry.
func (h TriggerRuleManually) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission:  service.PermissionAutomation,
		Name:        TriggerRuleManuallyName,
		Entitlement: entitlement.AutomationRules,
		Summary: "Runs a rule now, because somebody asked. Only a rule whose trigger is MANUAL " +
			"and only an enabled one; the run records who pulled the trigger, and the conditions, " +
			"the loop bound and the throttle apply exactly as they do to a run an event started.",
		SideEffects: "Queues a run and writes an audit entry. The run happens on a worker, so the " +
			"answer carries the identifier it will have rather than the run itself.",
		TokenScope: automationScope,
		Input: []usecase.Field{
			{Name: "rule_id", Kind: usecase.KindID, Required: true, Description: "Which rule."},
			{Name: "item_id", Kind: usecase.KindID, Description: "The entry the run is about, for a macro: " +
				"the conditions read it as item and the actions take it where they name no entry. The " +
				"presser must be allowed to change it, and it must lie in the rule's scope."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleTriggeredAction, TargetType: ruleTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The same reason writing a rule is exempt: a rule is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h TriggerRuleManually) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("rule_id")
	if err != nil {
		return nil, err
	}
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	result, err := h.ExecuteOn(ctx, actor, id, itemID)
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"run_id": result.RunID.String(), "rule_id": result.RuleID.String(),
	}, nil
}
