// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

func replayer(h *engineHarness) (ReplayRuleRun, *queued, *auditSink) {
	jobs := &queued{}
	sink := &auditSink{}
	return ReplayRuleRun{
		Runs: h.runs, Rules: h.rules, Jobs: jobs,
		Authorizer: &authorizer{}, Audit: sink,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: newRuleID},
	}, jobs, sink
}

// replayCommand feeds the replay's job back into the engine, exactly as the worker would.
func replayCommand(t *testing.T, request queue.Request) Command {
	t.Helper()
	return resumeCommand(t, request)
}

// The acceptance criterion: a replay completes a half-finished run without repeating its finished
// actions. The same occasion means the same idempotency keys; a finished action finds its key
// claimed and does nothing again, a failed one finds its claim released, and a skipped one runs
// for the first time.
func TestAReplayCompletesWithoutRepeating(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{
		{Kind: "ADD_LABEL", Params: map[string]any{"label_id": "a"}},
		{Kind: "CREATE_BUCKET"},
		{Kind: "ADD_LABEL", Params: map[string]any{"label_id": "b"}},
	}
	h := newEngine(t, rule)
	h.dispatcher.refuse["CREATE_BUCKET"] = shared.ErrForbidden.WithDetail("access.not_permitted")

	failed, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("the first run: %v", err)
	}
	if failed.Status != domain.RunFailed {
		t.Fatalf("the first run says %q, want FAILED", failed.Status)
	}
	if len(h.dispatcher.calls) != 2 {
		t.Fatalf("%d dispatches in the first run", len(h.dispatcher.calls))
	}

	// Whatever made it fail has been fixed.
	delete(h.dispatcher.refuse, "CREATE_BUCKET")

	replay, jobs, sink := replayer(h)
	result, err := replay.Execute(context.Background(), engineActor(), failed.ID)
	if err != nil {
		t.Fatalf("replaying: %v", err)
	}
	if result.RunID == failed.ID {
		t.Error("the replay reuses the failed run's identity")
	}
	if len(jobs.requests) != 1 {
		t.Fatalf("%d jobs queued", len(jobs.requests))
	}
	if occasion := jobs.requests[0].Payload["occasion"]; occasion != itemEvent().ID.String() {
		t.Errorf("the replay carries occasion %v, want the original", occasion)
	}

	completed, err := h.engine.Execute(
		context.Background(), engineActor(), replayCommand(t, jobs.requests[0]))
	if err != nil {
		t.Fatalf("the completing run: %v", err)
	}
	if completed.Status != domain.RunSucceeded {
		t.Fatalf("the completing run says %q", completed.Status)
	}

	counts := map[string]int{}
	for _, call := range h.dispatcher.calls {
		name := call.kind
		if label, _ := call.params["label_id"].(string); label != "" {
			name += ":" + label
		}
		counts[name]++
	}
	// The finished action once across both runs; the failed one once per attempt; the skipped one
	// exactly once, in the replay.
	if counts["ADD_LABEL:a"] != 1 {
		t.Errorf("the finished action was dispatched %d times", counts["ADD_LABEL:a"])
	}
	if counts["CREATE_BUCKET"] != 2 {
		t.Errorf("the failed action was dispatched %d times, want its retry", counts["CREATE_BUCKET"])
	}
	if counts["ADD_LABEL:b"] != 1 {
		t.Errorf("the skipped action was dispatched %d times", counts["ADD_LABEL:b"])
	}

	// And the act is audited with the replayer.
	if len(sink.entries) != 1 || sink.entries[0].Action != RunReplayedAction {
		t.Errorf("the audit trail says %+v", sink.entries)
	}
}

// Only a FAILED run: a waiting one resumes by itself, and the rest did what their rule says.
func TestOnlyAFailedRunCanBeReplayed(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunSucceeded {
		t.Fatalf("the run says %q", run.Status)
	}

	replay, jobs, _ := replayer(h)
	_, err = replay.Execute(context.Background(), engineActor(), run.ID)
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error %v, want a conflict", err)
	}
	if code := shared.AsError(err).DetailCode; code != "automation.run_not_replayable" {
		t.Errorf("detail code %s", code)
	}
	if len(jobs.requests) != 0 {
		t.Error("a finished run was queued again")
	}
}

// A disabled rule must not act - not even on its own past failures.
func TestAReplayOfADisabledRulesRunIsRefused(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "CREATE_BUCKET"}}
	h := newEngine(t, rule)
	h.dispatcher.refuse["CREATE_BUCKET"] = shared.ErrForbidden.WithDetail("access.not_permitted")

	failed, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}

	disabled := rule
	disabled.Enabled = false
	h.rules.rows[ruleID] = disabled

	replay, _, _ := replayer(h)
	_, err = replay.Execute(context.Background(), engineActor(), failed.ID)
	if code := detailOf(t, err); code != "automation.rule_not_enabled" {
		t.Errorf("detail code %s", code)
	}
}

// A row from before the occasion was stored: reconstructed where the kind allows, refused
// honestly where it does not - fresh keys would be exactly the duplication this route is not.
func TestAnOldRunsOccasionIsReconstructedOrRefused(t *testing.T) {
	old := domain.Run{
		ID: shared.ID("01936f2a-7c1e-7000-8000-000000000d01"), TenantID: tenant, RuleID: ruleID,
		EventID: itemEvent().ID, Trigger: domain.TriggerEvent, Status: domain.RunFailed,
	}
	occasion, err := replayOccasion(old)
	if err != nil || occasion != itemEvent().ID.String() {
		t.Errorf("an event run's occasion reads %q / %v", occasion, err)
	}

	old.Trigger, old.EventID = domain.TriggerSchedule, ""
	if _, err := replayOccasion(old); err == nil {
		t.Error("a schedule run without a stored occasion was replayed under guessed keys")
	}
}
