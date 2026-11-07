// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// engineProbe records the commands it is handed, and nests one step of its own when asked to.
type engineProbe struct {
	commands []Command
	nest     func(ctx context.Context) error
}

func (e *engineProbe) Execute(ctx context.Context, _ appshared.ActorContext, cmd Command) (domain.Run, error) {
	e.commands = append(e.commands, cmd)
	if e.nest != nil {
		nest := e.nest
		e.nest = nil
		if err := nest(ctx); err != nil {
			return domain.Run{}, err
		}
	}
	return domain.Run{}, nil
}

// UC-FLW-04 check 4: a step runs the enabled STEP_TAKEN rules of its workflow and key whose scope
// reaches the entry, synchronously, about the entry and the step's announcement.
func TestAStepRunsTheRulesOnItInItsChange(t *testing.T) {
	const workflow = "purchase"
	onStep := func(id shared.ID, scope domain.Scope, wf, step string, enabled bool) domain.Rule {
		rule := ruleAt(scope, id)
		rule.Enabled = enabled
		rule.Trigger = domain.Trigger{Kind: domain.TriggerStepTaken, Workflow: wf, Step: step}
		return rule
	}
	wanted := onStep("01936f2a-7c1e-7000-8000-000000000201", domain.Scope{Type: domain.ScopeHub, ID: hubID}, workflow, "approve", true)
	rules := []domain.Rule{
		wanted,
		onStep("01936f2a-7c1e-7000-8000-000000000202", domain.Scope{Type: domain.ScopeTenant}, workflow, "reject", true),
		onStep("01936f2a-7c1e-7000-8000-000000000203", domain.Scope{Type: domain.ScopeTenant}, "other", "approve", true),
		onStep("01936f2a-7c1e-7000-8000-000000000204", domain.Scope{Type: domain.ScopeHub, ID: otherHubID}, workflow, "approve", true),
		onStep("01936f2a-7c1e-7000-8000-000000000205", domain.Scope{Type: domain.ScopeTenant}, workflow, "approve", false),
		ruleAt(domain.Scope{Type: domain.ScopeTenant}, "01936f2a-7c1e-7000-8000-000000000206"),
	}
	_, _, places := newMatcher(nil)
	engine := &engineProbe{}
	steps := StepTaken{Rules: matching{rules: rules}, Containers: places, Engine: engine}
	item := work.WorkItem{ID: itemID, CollectionID: collectionID}
	announced := itemEvent()

	if err := steps.StepTaken(t.Context(), appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant}, item, workflow, "approve", announced); err != nil {
		t.Fatalf("StepTaken: %v", err)
	}
	if len(engine.commands) != 1 {
		t.Fatalf("ran %d rules, want only the one on this workflow's step in scope", len(engine.commands))
	}
	cmd := engine.commands[0]
	if cmd.RuleID != wanted.ID || cmd.Trigger != domain.TriggerStepTaken || cmd.EventID != announced.ID ||
		cmd.SubjectID != itemID || cmd.CausationDepth != 0 {
		t.Fatalf("the run is about the entry and the step's announcement: %+v", cmd)
	}
}

// A rule on a step that takes another step nests, one deeper each time, so the engine's depth bound
// ends a chain inside one request.
func TestANestedStepRunsOneDeeper(t *testing.T) {
	const workflow = "purchase"
	rule := ruleAt(domain.Scope{Type: domain.ScopeTenant}, "01936f2a-7c1e-7000-8000-000000000201")
	rule.Trigger = domain.Trigger{Kind: domain.TriggerStepTaken, Workflow: workflow, Step: "approve"}
	_, _, places := newMatcher(nil)
	engine := &engineProbe{}
	steps := StepTaken{Rules: matching{rules: []domain.Rule{rule}}, Containers: places, Engine: engine}
	item := work.WorkItem{ID: itemID, CollectionID: collectionID}
	engine.nest = func(ctx context.Context) error {
		return steps.StepTaken(ctx, appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant}, item, workflow, "approve", itemEvent())
	}

	if err := steps.StepTaken(t.Context(), appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant}, item, workflow, "approve", itemEvent()); err != nil {
		t.Fatalf("StepTaken: %v", err)
	}
	if len(engine.commands) != 2 || engine.commands[1].CausationDepth != 1 {
		t.Fatalf("the nested run is one deeper: %+v", engine.commands)
	}
}
