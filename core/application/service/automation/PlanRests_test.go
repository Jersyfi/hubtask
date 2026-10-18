// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The workspace's plan as the engine reads it, changeable between two calls: a plan changes with no
// job, so "included again" is the next read (UC-INS-13 checks 2 and 4).
type planLeftOut struct{ leftOut entitlement.LeftOut }

func (p *planLeftOut) LeftOut(context.Context) (entitlement.LeftOut, error) { return p.leftOut, nil }

func withoutRules() *planLeftOut {
	return &planLeftOut{leftOut: entitlement.LeftOut{entitlement.AutomationRules}}
}

// A plan without the rules: a run queued before the move runs into nothing - no run row, no action
// - and the rule is untouched; included again, the next event runs it (multi-tenancy.md §4.2).
func TestARuleRestsWhileThePlanLeavesTheRulesOut(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)
	plan := withoutRules()
	h.engine.Entitlements = plan

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.ID != "" || len(h.runs.order) != 0 || len(h.dispatcher.calls) != 0 {
		t.Fatalf("a resting rule ran: %+v, %d rows, %d actions", run, len(h.runs.order), len(h.dispatcher.calls))
	}
	if stored := h.rules.rows[ruleID]; !stored.Enabled {
		t.Error("resting switched the rule off")
	}

	plan.leftOut = nil
	run, err = h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil || run.Status != domain.RunSucceeded || len(h.dispatcher.calls) != 1 {
		t.Fatalf("included again: %+v, %v, %d actions", run, err, len(h.dispatcher.calls))
	}
}

// A run parked on a WAIT ends when it would resume into a plan without the rules, outside the
// streak: the rule did nothing wrong.
func TestAWaitingRunEndsWithoutTheStreakWhenThePlanLeavesTheRulesOut(t *testing.T) {
	h := newEngine(t, waitingRule())
	if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
		t.Fatalf("parking: %v", err)
	}
	h.engine.Entitlements = withoutRules()

	run, err := h.engine.Execute(context.Background(), engineActor(), resumeCommand(t, h.jobs.requests[0]))
	if err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if run.Status != domain.RunFailed || run.ErrorCode != entitlement.NotIncludedCode {
		t.Errorf("the run says %q / %q", run.Status, run.ErrorCode)
	}
	if h.failures.count != 0 || len(h.dispatcher.calls) != 1 {
		t.Errorf("streak %d, %d dispatches", h.failures.count, len(h.dispatcher.calls))
	}
}

// An action whose feature the plan leaves out - a webhook sent on a plan without webhooks - is
// skipped, not failed: the run succeeds and the streak clears, so the plan never switches a rule
// off (P-03).
func TestAnActionThePlanLeavesOutIsSkippedNotFailed(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "SEND_WEBHOOK"}, {Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)
	h.dispatcher.scopes["SEND_WEBHOOK"] = "webhooks:write"
	h.dispatcher.refuse["SEND_WEBHOOK"] = entitlement.NotIncluded(entitlement.Webhooks)

	for range MaxConsecutiveFailures + 1 {
		run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
		if err != nil {
			t.Fatalf("running: %v", err)
		}
		if run.Status != domain.RunSucceeded {
			t.Fatalf("status %q %q %+v", run.Status, run.ErrorCode, run.ActionResults)
		}
		if got := run.ActionResults[0]; got.Status != domain.ActionSkipped || got.ErrorCode != entitlement.NotIncludedCode {
			t.Fatalf("the left-out action is %q / %q", got.Status, got.ErrorCode)
		}
		if got := run.ActionResults[1]; got.Status != domain.ActionSucceeded {
			t.Fatalf("the next action is %q", got.Status)
		}
	}
	if h.failures.count != 0 || h.failures.disabled {
		t.Errorf("the streak counted %d, disabled %v", h.failures.count, h.failures.disabled)
	}
}

// No event queues a run where the plan leaves the rules out; included again, it does.
func TestNoEventQueuesARunWhileThePlanLeavesTheRulesOut(t *testing.T) {
	matcher, queued, _ := newMatcher([]domain.Rule{ruleAt(domain.Scope{Type: domain.ScopeTenant}, ruleID)})
	plan := withoutRules()
	matcher.Entitlements = plan

	if err := matcher.Deliver(context.Background(), itemEvent()); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(queued.queued) != 0 {
		t.Fatalf("%d jobs queued for a plan without the rules", len(queued.queued))
	}

	plan.leftOut = entitlement.LeftOut{entitlement.Webhooks}
	if err := matcher.Deliver(context.Background(), itemEvent()); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(queued.queued) != 1 {
		t.Fatalf("included again, %d jobs queued", len(queued.queued))
	}
}

// A schedule due while the plan leaves the rules out moves on from now and fires nothing; included
// again, it fires its next moment and never one it missed. A relative-date occurrence due while
// resting is claimed and dropped (multi-tenancy.md §4.2: "without the occurrences it missed").
func TestASchedulePassesItsMomentsWhileThePlanLeavesTheRulesOut(t *testing.T) {
	missed := now.Add(-time.Hour)
	store := newSchedules(scheduledRule(missed))
	pass, q := newPass(store, 24*time.Hour)
	owed := newOccurrences()
	owed.rows["due"] = domain.Occurrence{ID: shared.ID("01936f2a-7c1e-7000-8000-0000000009a1"), RuleID: ruleID, FireAt: missed}
	pass.Occurrences = owed
	plan := withoutRules()
	pass.Entitlements = plan

	result, err := pass.Run(context.Background(), tenantScope())
	if err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if result.Started != 0 || len(q.queued) != 0 {
		t.Fatalf("resting, %d runs started, %d jobs", result.Started, len(q.queued))
	}
	next := now.Add(24 * time.Hour)
	if moved := store.moved[ruleID]; !moved.Equal(next) {
		t.Errorf("moved to %v, want the next moment after now", moved)
	}
	if len(owed.rows) != 0 {
		t.Errorf("the missed occurrence is still owed: %v", owed.rows)
	}
	if !result.NextDue.Equal(next) {
		t.Errorf("next due %v, want %v - the poller keeps the rule's moments", result.NextDue, next)
	}

	// Included again at the next moment: one run, for that moment.
	plan.leftOut = nil
	pass.Clock = clock.Fixed(next)
	result, err = pass.Run(context.Background(), tenantScope())
	if err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if result.Started != 1 || len(q.queued) != 1 {
		t.Fatalf("included again, %d runs, %d jobs", result.Started, len(q.queued))
	}
	if got, want := q.queued[0].Payload["occasion"], "schedule:"+next.UTC().Format(time.RFC3339); got != want {
		t.Errorf("fired %v, want %v - never the missed one", got, want)
	}
}

// The inbound address of a rule whose workspace's plan leaves the rules out answers as a switched-
// off rule's does - the one uniform not found - and queues nothing; included again, it starts the
// rule.
func TestAnInboundAddressIsGoneWhileThePlanLeavesTheRulesOut(t *testing.T) {
	handler, open, _, _ := rotator(inboundRule())
	minted, err := handler.Execute(context.Background(), presser(), ruleID)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	start, queued := starter(open)
	plan := withoutRules()
	start.Entitlements = plan
	delivery := InboundDelivery{Token: minted.Token, Payload: map[string]any{}}

	if _, err := start.Execute(context.Background(), delivery); shared.AsError(err).DetailCode != "automation.inbound_not_found" {
		t.Fatalf("a resting rule's address answered %v", err)
	}
	if len(queued.queued) != 0 {
		t.Fatalf("%d jobs queued for a plan without the rules", len(queued.queued))
	}

	plan.leftOut = nil
	if _, err := start.Execute(context.Background(), delivery); err != nil || len(queued.queued) != 1 {
		t.Fatalf("included again: %v, %d jobs", err, len(queued.queued))
	}
}
