// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package automation runs what a rule decided to do.
//
// The rule engine itself - triggers, CEL conditions, throttling, the run log - arrives with its
// own task. What is here is the half that matters for parity: every action of a rule is an
// adapter over a use case, so the list of available actions grows with the catalogue rather than
// with a table somebody maintains (automation.md §1.3).
package automation

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Catalogue is the slice of the use case registry the dispatcher needs.
type Catalogue interface {
	All() []usecase.Descriptor
	ByAutomationAction(kind string) (usecase.Descriptor, bool)
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// Action is one step of a rule: what to do, and with which parameters (automation.md §1).
type Action struct {
	Kind   string
	Params map[string]any
}

// ActionDispatcher executes actions.
//
// It grants nothing. The rule runs as its `run_as` account and can never do more than that
// account may (automation.md §2) - which is true here because the dispatcher passes the actor to
// the same use case a person would reach, and the permission check happens there, once
// (ADR-0005). A dispatcher that checked permissions itself would be a second answer to the same
// question, and the two would eventually disagree.
type ActionDispatcher struct {
	Catalogue Catalogue
}

func NewActionDispatcher(catalogue Catalogue) ActionDispatcher {
	return ActionDispatcher{Catalogue: catalogue}
}

// Actions lists the kinds a rule may use, in the catalogue's order. It is what the rule editor
// and /meta/capabilities answer from, so a new use case appears as an action without anybody
// adding it to a list.
func (d ActionDispatcher) Actions() []string {
	descriptors := d.Catalogue.All()

	kinds := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		kinds = append(kinds, descriptor.AutomationAction())
	}
	return kinds
}

// Dispatch runs one action as the given actor.
//
// What it deliberately does not do yet: derive the idempotency key from
// (rule_id, event_id, action_index), count the causal depth, and record the result in the run
// log. Those belong to the engine that owns a rule run, and inventing half of them here would
// mean two places deciding what "the same action twice" means (automation.md §2).
func (d ActionDispatcher) Dispatch(
	ctx context.Context, runAs appshared.ActorContext, action Action,
) (usecase.Output, error) {
	if !runAs.IsAuthenticated() {
		// Fail closed. A rule without a usable `run_as` account is a misconfigured rule, and
		// running it as nobody would run it without a tenant.
		return nil, shared.ErrForbidden.
			WithDetail("automation.run_as_missing").
			WithParams(map[string]string{"action": action.Kind})
	}

	descriptor, found := d.Catalogue.ByAutomationAction(action.Kind)
	if !found {
		// A rule naming an action this installation does not have is a rule that has to be
		// corrected; it is not a defect of the server.
		return nil, shared.ErrValidation.
			WithDetail("automation.action_unknown").
			WithParams(map[string]string{"action": action.Kind})
	}

	return d.Catalogue.Invoke(ctx, descriptor.Name, runAs, usecase.Input(action.Params))
}
