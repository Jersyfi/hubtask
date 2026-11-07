// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Engine is the slice of RunRule a producer that runs a rule itself needs.
type Engine interface {
	Execute(ctx context.Context, actor appshared.ActorContext, cmd Command) (domain.Run, error)
}

// StepTaken runs the STEP_TAKEN rules on one step inside the step's own transaction (UC-FLW-04
// check 4): what their actions change on the entry commits with the step, and a refused step
// leaves no run behind.
//
// Synchronous rather than queued, which is the one difference from every other producer: the
// engine is the same, with the same run log, throttle, authorisation as `run_as` and depth bound.
// A rule on a step that takes another step nests here; the depth carried in the context is what
// ends such a chain inside one request, as the event's causation depth ends it between jobs.
type StepTaken struct {
	Rules      repository.Matching
	Containers Containers
	// Entitlements is what the plan leaves out; nil includes every feature.
	Entitlements adminrepo.Entitlements
	Engine       Engine
}

type stepDepthKey struct{}

// StepTaken runs every enabled rule on this workflow's step whose scope reaches the entry.
func (s StepTaken) StepTaken(
	ctx context.Context, actor appshared.ActorContext, item work.WorkItem,
	workflow, step string, announced event.Envelope,
) error {
	if s.Engine == nil || s.Rules == nil {
		return nil
	}
	rules, err := s.Rules.ByTriggerKind(ctx, domain.TriggerStepTaken)
	if err != nil {
		return err
	}
	var wanted []domain.Rule
	for _, rule := range rules {
		if rule.Enabled && rule.Trigger.Workflow == workflow && rule.Trigger.Step == step {
			wanted = append(wanted, rule)
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	if resting, err := rulesRest(ctx, s.Entitlements); err != nil || resting {
		return err
	}
	at, err := locateEvent(ctx, s.Containers, announced)
	if err != nil {
		return err
	}
	if at.CollectionID.IsZero() {
		at.CollectionID = item.CollectionID
	}

	depth, _ := ctx.Value(stepDepthKey{}).(int)
	if announced.CausationDepth > depth {
		depth = announced.CausationDepth
	}
	nested := context.WithValue(ctx, stepDepthKey{}, depth+1)
	// The system acting for the tenant, as the queue runner's actor: each action acts as the
	// rule's own account, which the engine builds per action.
	system := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: actor.TenantID}
	for _, rule := range wanted {
		// The scope is the producer's to narrow, as in MatchRules; a private hub the run_as does
		// not reach is the engine's own check.
		if !covers(rule.Scope, at) {
			continue
		}
		if _, err := s.Engine.Execute(nested, system, Command{
			RuleID: rule.ID, Trigger: domain.TriggerStepTaken,
			EventID: announced.ID, SubjectID: item.ID, Occasion: announced.ID.String(),
			CausationDepth: depth,
		}); err != nil {
			return err
		}
	}
	return nil
}
