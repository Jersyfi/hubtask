// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/automation"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// AutomationRun is the queue's way into the engine (G-07).
//
// An inbound adapter like every other handler: it translates a job into a call on the application
// layer and the answer into the queue's vocabulary. What is particular to it is `on_error: RETRY` -
// the one of the three values that is not the engine's to implement, because "retry with exponential
// backoff, and the dead letter after the budget" is what the queue already is (automation.md §2). A
// second backoff inside the engine would be a second answer to a question this system has answered.
//
// It runs inside the runner's transaction, which is what makes at-least-once delivery safe to build
// on: the run row, the effects its actions had, the idempotency records and the job's completion
// commit together.
type AutomationRun struct {
	Engine automation.RunRule
	// Rules answers what to do about a failure. Read here rather than returned by the engine
	// because it is the *queue's* question - the engine records what happened, and this layer
	// decides whether the job comes back.
	Rules automation.RuleReader
}

var _ queue.Handler = AutomationRun{}

// Run performs one rule's reaction to one event.
func (a AutomationRun) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		// Every read the run makes is made for the tenant the job names. Without one there is
		// nothing to run - an automation job without a tenant is a programming error, not an empty
		// pass.
		return queue.Result{}, shared.ErrInternal.WithDetail("automation.run_without_tenant")
	}

	cmd, err := commandOf(job)
	if err != nil {
		return queue.Result{}, err
	}

	// The system acting for a tenant. The *rule's* account is the actor of every action the run
	// performs, and the engine builds that one per action - what this actor is for is the
	// transaction's tenant and the reads the engine makes on its own behalf.
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID}

	run, err := a.Engine.Execute(ctx, actor, cmd)
	if err != nil {
		return queue.Result{}, err
	}
	return a.next(ctx, actor, run, cmd)
}

// next decides whether the job comes back.
//
// Only a failed run can, and only when its rule says RETRY. The queue's backoff and its dead letter
// are what "retry with exponential backoff, and disable after the budget" means here - returning an
// error is how a handler asks for both, and the runner's attempt counter is the budget.
//
// A run that was skipped, throttled or aborted is finished. None of them is a transient condition:
// a condition that answered false will answer false again, a rule at its bound will still be at it,
// and a loop is not fixed by trying it once more.
func (a AutomationRun) next(
	ctx context.Context, actor appshared.ActorContext, run domain.Run, cmd automation.Command,
) (queue.Result, error) {
	if run.Status != domain.RunFailed {
		return queue.Result{}, nil
	}

	rule, err := a.Rules.Find(ctx, cmd.RuleID)
	if err != nil {
		// The rule went away between the run and this question. The run is recorded; there is
		// nothing left to retry for.
		return queue.Result{}, nil
	}
	if rule.OnError != domain.OnErrorRetry {
		// STOP and CONTINUE have already had their effect inside the run. The job is done - the
		// failure is in the log, and repeating it would repeat the failure rather than fix it.
		return queue.Result{}, nil
	}

	// An error, so the runner applies its backoff and eventually its dead letter. The code names
	// the rule's own choice rather than the action's failure, because what is being reported is
	// "this rule asked to be retried", and the action's own code is on the run where it belongs.
	return queue.Result{}, shared.ErrUnavailable.
		WithDetail("automation.run_retrying").
		WithParams(map[string]string{"rule_id": cmd.RuleID.String()})
}

// commandOf reads the job's payload.
//
// Data rather than a domain object, because the row outlives the process that wrote it - a type that
// changed shape in between would take the job with it.
func commandOf(job queue.Job) (automation.Command, error) {
	ruleID, err := idPayload(job, "rule_id")
	if err != nil {
		return automation.Command{}, err
	}
	eventID, err := idPayload(job, "event_id")
	if err != nil {
		return automation.Command{}, err
	}

	depth := 0
	switch value := job.Payload["causation_depth"].(type) {
	case int:
		depth = value
	case int64:
		depth = int(value)
	case float64:
		depth = int(value)
	}
	if depth < 0 {
		depth = 0
	}
	return automation.Command{RuleID: ruleID, EventID: eventID, CausationDepth: depth}, nil
}

func idPayload(job queue.Job, key string) (shared.ID, error) {
	text, _ := job.Payload[key].(string)
	if text == "" {
		if key == "event_id" {
			// A run nothing published started - which is what the other four triggers will be.
			return "", nil
		}
		return "", shared.ErrInternal.
			WithDetail("automation.run_payload_incomplete").
			WithParams(map[string]string{"field": key})
	}
	return shared.ParseID(text)
}
