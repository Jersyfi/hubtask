// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var manualRunID = shared.ID("01936f2a-7c1e-7000-8000-0000000000e9")

type triggerHarness struct {
	handler TriggerRuleManually
	store   *ruleStore
	queued  *jobs
	auth    *authorizer
	sink    *auditSink
}

func newTrigger(rule domain.Rule) *triggerHarness {
	h := &triggerHarness{
		store: newRuleStore(rule), queued: &jobs{}, auth: &authorizer{}, sink: &auditSink{},
	}
	h.handler = TriggerRuleManually{
		Rules: h.store, Jobs: h.queued, Authorizer: h.auth, Audit: h.sink,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: manualRunID},
	}
	return h
}

func manualRule() domain.Rule {
	rule := ruleAt(domain.Scope{Type: domain.ScopeTenant}, ruleID)
	rule.Trigger = domain.Trigger{Kind: domain.TriggerManual}
	rule.Enabled = true
	return rule
}

func presser() appshared.ActorContext {
	return appshared.ActorContext{
		TenantID: tenant, AccountID: writerID, AccountName: "Anna",
		Kind: shared.ActorUser, Scopes: []string{automationScope},
	}
}

// The acceptance criterion: `:trigger` runs the rule now and the run records the actor.
//
// "Now" is a job rather than an inline run: the actions are writes performed as the `run_as`
// account, and a request that held the connection open while a rule restructured a hub would let
// its own timeout decide how much of the rule happened.
func TestTriggeringARuleQueuesARunThatNamesWhoPressedIt(t *testing.T) {
	h := newTrigger(manualRule())

	result, err := h.handler.Execute(context.Background(), presser(), ruleID)
	if err != nil {
		t.Fatalf("triggering: %v", err)
	}
	if result.RunID != manualRunID || result.RuleID != ruleID {
		t.Fatalf("answered run %s of rule %s", result.RunID, result.RuleID)
	}
	if len(h.queued.queued) != 1 {
		t.Fatalf("%d jobs queued, want one", len(h.queued.queued))
	}

	job := h.queued.queued[0]
	if job.Kind != queue.KindAutomationRun {
		t.Errorf("job kind %q, want the engine's", job.Kind)
	}
	if job.TenantID != tenant {
		t.Errorf("job tenant %q, want the caller's", job.TenantID)
	}
	for field, want := range map[string]string{
		"rule_id":      ruleID.String(),
		"trigger":      string(domain.TriggerManual),
		"run_id":       manualRunID.String(),
		"triggered_by": writerID.String(),
		"occasion":     "manual:" + manualRunID.String(),
	} {
		if got, _ := job.Payload[field].(string); got != want {
			t.Errorf("payload %s = %q, want %q", field, got, want)
		}
	}
}

// The permission is the plain one at the rule's scope, not the composition check writing one needs.
// Pressing the button changes nothing about what the rule may do - that was decided when it was
// written and is decided again, per action, when it runs (automation.md §2.1).
func TestTriggeringAsksTheAutomationPermissionAtTheRulesScope(t *testing.T) {
	rule := manualRule()
	rule.Scope = domain.Scope{Type: domain.ScopeHub, ID: hubID}
	h := newTrigger(rule)

	if _, err := h.handler.Execute(context.Background(), presser(), ruleID); err != nil {
		t.Fatalf("triggering: %v", err)
	}
	if len(h.auth.requests) != 1 {
		t.Fatalf("%d permission questions, want one", len(h.auth.requests))
	}

	request := h.auth.requests[0]
	if request.Permission != service.PermissionAutomation {
		t.Errorf("permission %q, want the automation one", request.Permission)
	}
	if request.TokenScope != automationScope {
		t.Errorf("token scope %q, want %q", request.TokenScope, automationScope)
	}
	if len(request.Path) != 2 || request.Path[1].ID != hubID {
		t.Errorf("path %v, want it resolved down to the rule's hub", request.Path)
	}
	if request.Action != RuleTriggeredAction {
		t.Errorf("action %q, want its own code", request.Action)
	}
}

func TestARefusedTriggerQueuesNothing(t *testing.T) {
	h := newTrigger(manualRule())
	h.auth.refuse = true

	_, err := h.handler.Execute(context.Background(), presser(), ruleID)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want ErrForbidden", err)
	}
	if len(h.queued.queued) != 0 {
		t.Errorf("%d jobs queued after a refusal", len(h.queued.queued))
	}
	if len(h.sink.entries) != 0 {
		t.Errorf("%d audit entries for a run that never happened", len(h.sink.entries))
	}
}

// Refused by name rather than queued to do nothing. The engine would drop the job on the same
// comparison, and a call that appears to work and silently does nothing is the failure
// automation.md §2.2 exists to avoid.
func TestOnlyAManualRuleAndOnlyAnEnabledOneCanBePressed(t *testing.T) {
	t.Run("another kind", func(t *testing.T) {
		rule := manualRule()
		rule.Trigger = domain.Trigger{Kind: domain.TriggerSchedule, RRule: "FREQ=DAILY", Timezone: "UTC"}
		h := newTrigger(rule)

		_, err := h.handler.Execute(context.Background(), presser(), ruleID)
		if !errors.Is(err, shared.ErrValidation) {
			t.Fatalf("error %v, want a validation refusal", err)
		}
		if !strings.Contains(shared.AsError(err).DetailCode, "trigger_not_manual") {
			t.Errorf("code %q does not say which half is wrong", shared.AsError(err).DetailCode)
		}
		if len(h.queued.queued) != 0 {
			t.Error("a job was queued for a rule nothing can press")
		}
	})

	t.Run("switched off", func(t *testing.T) {
		rule := manualRule()
		rule.Enabled = false
		h := newTrigger(rule)

		_, err := h.handler.Execute(context.Background(), presser(), ruleID)
		if !errors.Is(err, shared.ErrConflict) {
			t.Fatalf("error %v, want a conflict", err)
		}
		if len(h.queued.queued) != 0 {
			t.Error("a job was queued for a rule that is switched off")
		}
	})
}

// The entry a review looks for: a person made this rule act, and it acted with somebody else's
// rights. Never the rule's name (rule 10).
func TestPressingTheButtonIsAudited(t *testing.T) {
	h := newTrigger(manualRule())

	if _, err := h.handler.Execute(context.Background(), presser(), ruleID); err != nil {
		t.Fatalf("triggering: %v", err)
	}
	if len(h.sink.entries) != 1 {
		t.Fatalf("%d audit entries, want one", len(h.sink.entries))
	}

	entry := h.sink.entries[0]
	if entry.Action != RuleTriggeredAction {
		t.Errorf("action %q, want its own code", entry.Action)
	}
	if entry.ActorID != writerID {
		t.Errorf("actor %q, want the person who pressed it", entry.ActorID)
	}
	if entry.OnBehalfOf != serviceID {
		t.Errorf("on behalf of %q, want the account the run will act as", entry.OnBehalfOf)
	}
	if entry.TargetID != ruleID {
		t.Errorf("target %q, want the rule", entry.TargetID)
	}
}
