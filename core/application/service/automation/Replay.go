// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ReplayRuleRunName = "ReplayRuleRun"

	// RunReplayedAction is a failed run sent back into the engine by hand. An act rather than
	// machinery, audited with the replayer: somebody looked at a failure and decided the world is
	// ready for the rest.
	RunReplayedAction audit.Action = "automation.run_replayed"
)

// ReplayRuleRun completes a half-finished run (G-09, automation.md §2).
//
// It re-executes a failed run's remaining actions under the same idempotency keys, which is the
// whole design: the replay's run carries the original occasion, so an action the original run
// finished finds its key already claimed and does nothing again - a completion, not a
// duplication. Only what never happened happens now.
//
// It queues rather than runs, for `:trigger`'s reason, and the engine answers everything else
// exactly as it answers any run: the conditions are evaluated again against the world as it
// stands - the answer that decides an action is the answer of the day it runs (§2.1) - the loop
// bound and the throttle apply, and the run log records the replay as its own run.
type ReplayRuleRun struct {
	Runs       repository.Runs
	Rules      repository.Rules
	Jobs       Queue
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// Execute queues the completion.
func (h ReplayRuleRun) Execute(
	ctx context.Context, actor appshared.ActorContext, runID shared.ID,
) (TriggerRuleResult, error) {
	var (
		run  domain.Run
		rule domain.Rule
	)
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var findErr error
		if run, findErr = h.Runs.Find(ctx, runID); findErr != nil {
			return findErr
		}
		rule, findErr = h.Rules.Find(ctx, run.RuleID)
		return findErr
	})
	if err != nil {
		return TriggerRuleResult{}, err
	}

	if err := h.Authorizer.Authorize(
		ctx, actor, runRequest(rule.Scope, RunReplayedAction, run.ID)); err != nil {
		return TriggerRuleResult{}, err
	}

	if run.Status != domain.RunFailed {
		// A waiting run resumes by itself, a skipped or throttled one did what its rule says, and
		// a succeeded one is done. Each refusal names the status, so the caller learns which.
		return TriggerRuleResult{}, shared.ErrConflict.
			WithDetail("automation.run_not_replayable").
			WithParams(map[string]string{"status": string(run.Status)})
	}
	if !rule.Enabled {
		// Somebody switched the rule off, and that is the honest way to stop a rule acting -
		// including acting on its past failures.
		return TriggerRuleResult{}, shared.ErrConflict.
			WithDetail("automation.rule_not_enabled").
			WithParams(map[string]string{"rule_id": rule.ID.String()})
	}

	occasion, err := replayOccasion(run)
	if err != nil {
		return TriggerRuleResult{}, err
	}

	newRunID := h.IDs.NewID()
	payload := map[string]any{
		"rule_id":         rule.ID.String(),
		"trigger":         string(run.Trigger),
		"run_id":          newRunID.String(),
		"occasion":        occasion,
		"causation_depth": run.CausationDepth,
	}
	if !run.EventID.IsZero() {
		payload["event_id"] = run.EventID.String()
	}
	if !run.TriggeredBy.IsZero() {
		payload["triggered_by"] = run.TriggeredBy.String()
	}
	if !run.SubjectID.IsZero() {
		payload["subject_id"] = run.SubjectID.String()
	}

	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		_, enqueueErr := h.Jobs.Enqueue(ctx, queue.Request{
			Kind:     queue.KindAutomationRun,
			TenantID: actor.TenantID,
			Payload:  payload,
			// Unique per replay, so a client retrying the call while the job is pending gets the
			// one completion it asked for rather than two.
			DedupeKey: ConsumerName + ":replay:" + newRunID.String(),
		})
		return enqueueErr
	})
	if err != nil {
		return TriggerRuleResult{}, err
	}

	h.record(ctx, actor, rule, run, newRunID)
	return TriggerRuleResult{RunID: newRunID, RuleID: rule.ID}, nil
}

// replayOccasion answers the occasion the replay's keys must carry: the stored one, or - for rows
// written before it was stored - the reconstruction the kind allows. A kind whose occasion cannot
// be reconstructed is refused honestly rather than replayed under fresh keys, because fresh keys
// are exactly the duplication this route exists not to be.
func replayOccasion(run domain.Run) (string, error) {
	if run.Occasion != "" {
		return run.Occasion, nil
	}

	switch {
	case run.Trigger == domain.TriggerEvent && !run.EventID.IsZero():
		return run.EventID.String(), nil
	case run.Trigger == domain.TriggerManual:
		return "manual:" + run.ID.String(), nil
	case run.Trigger == domain.TriggerInboundWebhook:
		return "inbound:" + run.ID.String(), nil
	default:
		// A schedule's occurrence instant and a relative date's occurrence row are not on the run
		// row this old, and guessing one would mint fresh keys.
		return "", shared.ErrConflict.
			WithDetail("automation.replay_occasion_unknown").
			WithParams(map[string]string{"trigger": run.Trigger.String()})
	}
}

// record writes the entry that says a person did this, with both runs on it: the failure that was
// completed and the run that completes it.
func (h ReplayRuleRun) record(
	ctx context.Context, actor appshared.ActorContext,
	rule domain.Rule, run domain.Run, newRunID shared.ID,
) {
	if h.Audit == nil {
		return
	}
	_ = h.Audit.Append(ctx, audit.Entry{
		TenantID:   rule.TenantID,
		OccurredAt: h.Clock.Now(),
		Action:     RunReplayedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		OnBehalfOf: rule.RunAs,
		TargetType: runTarget,
		TargetID:   run.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "rule_id", Classification: audit.Open, To: rule.ID.String()},
			audit.Change{Field: "replay_run_id", Classification: audit.Open, To: newRunID.String()},
		),
	})
}

// Descriptor is the catalogue entry.
func (h ReplayRuleRun) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReplayRuleRunName,
		Summary: "Completes a failed run: re-executes its remaining actions under the same " +
			"idempotency keys, so an action the original run finished does nothing again and " +
			"only what never happened happens now. Only a FAILED run, and only while its rule " +
			"is enabled. The conditions are evaluated again against the world as it stands, and " +
			"the replay appears in the log as its own run.",
		SideEffects: "Queues a run and writes an audit entry naming the replayer. The run " +
			"happens on a worker, so the answer carries the identifier it will have.",
		TokenScope: automationScope,
		Input: []usecase.Field{
			{Name: "run_id", Kind: usecase.KindID, Required: true, Description: "The failed run."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RunReplayedAction, TargetType: runTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The same reason triggering a rule is exempt: a run is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReplayRuleRun) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("run_id")
	if err != nil {
		return nil, err
	}
	result, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"run_id": result.RunID.String(), "rule_id": result.RuleID.String(),
	}, nil
}
