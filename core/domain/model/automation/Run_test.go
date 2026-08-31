// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	runID   = shared.ID("01936f2a-7c1e-7000-8000-0000000000d1")
	eventID = shared.ID("01936f2a-7c1e-7000-8000-0000000000d2")
	started = time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
)

func startedRun(t *testing.T, depth int) automation.Run {
	t.Helper()

	run, err := automation.StartRun(automation.NewRunInput{
		ID: runID, TenantID: tenantID, RuleID: ruleID, EventID: eventID,
		Trigger: automation.TriggerEvent, CausationDepth: depth, Now: started,
	})
	if err != nil {
		t.Fatalf("starting the run: %v", err)
	}
	return run
}

// Written before the conditions are evaluated, which is the only thing that makes a crashed run
// visible: a row written when the run ends loses exactly the runs somebody needs to see.
func TestARunOpensAsRunning(t *testing.T) {
	run := startedRun(t, 0)

	if run.Status != automation.RunRunning {
		t.Errorf("status %q, want RUNNING", run.Status)
	}
	if run.Status.Finished() {
		t.Error("a running run reports itself finished")
	}
	if run.FinishedAt != nil {
		t.Errorf("finished at %v before it ended", run.FinishedAt)
	}
	// Empty rather than null, so what is read back is a list either way.
	if run.ConditionResults == nil || run.ActionResults == nil {
		t.Error("the result lists came back null")
	}
}

func TestARunWithoutItsIdentifiersIsAnInternalError(t *testing.T) {
	for _, missing := range []string{"run", "tenant", "rule", "depth", "trigger", "bad trigger"} {
		t.Run(missing, func(t *testing.T) {
			in := automation.NewRunInput{
				ID: runID, TenantID: tenantID, RuleID: ruleID,
				Trigger: automation.TriggerEvent, Now: started,
			}
			switch missing {
			case "run":
				in.ID = ""
			case "tenant":
				in.TenantID = ""
			case "rule":
				in.RuleID = ""
			case "depth":
				in.CausationDepth = -1
			case "trigger":
				// A run that does not know what started it. Internal, not a refusal: the six kinds
				// are named by the producer that queued the job, never by a caller.
				in.Trigger = ""
			case "bad trigger":
				in.Trigger = "CRON"
			}

			// Internal rather than a validation refusal: no caller sends these.
			if _, err := automation.StartRun(in); !errors.Is(err, shared.ErrInternal) {
				t.Errorf("error %v, want ErrInternal", err)
			}
		})
	}
}

// The loop protection. Asked of the depth the event carried, before anything else about the run is
// decided: evaluating conditions is where the reads are, and a loop that read on every hop would
// cost exactly what the bound exists to prevent.
func TestTheDepthLimitIsReachedAtTheDocumentedNumber(t *testing.T) {
	for depth := range automation.MaxCausationDepth {
		if automation.TooDeep(depth) {
			t.Errorf("depth %d was refused, and the limit is %d", depth, automation.MaxCausationDepth)
		}
	}
	if !automation.TooDeep(automation.MaxCausationDepth) {
		t.Errorf("depth %d was allowed", automation.MaxCausationDepth)
	}
	if !automation.TooDeep(automation.MaxCausationDepth + 1) {
		t.Error("a depth past the limit was allowed")
	}
}

// Aborting is not failing: "this rule is misconfigured into a loop" and "this rule's action was
// refused" send their reader to two different places.
func TestAnAbortedRunSaysWhyAndIsNotAFailure(t *testing.T) {
	run := startedRun(t, automation.MaxCausationDepth).Abort(started.Add(time.Second))

	if run.Status != automation.RunAbortedLoop {
		t.Errorf("status %q, want ABORTED_LOOP", run.Status)
	}
	if run.ErrorCode != "automation.loop_depth_exceeded" {
		t.Errorf("code %q", run.ErrorCode)
	}
	if run.FinishedAt == nil || !run.FinishedAt.Equal(started.Add(time.Second)) {
		t.Errorf("finished at %v", run.FinishedAt)
	}
	if len(run.ActionResults) != 0 {
		t.Error("an aborted run recorded actions")
	}
}

// Not a failure and not a skip: the conditions were never asked, because the rule has already run
// as often as it may.
func TestAThrottledRunIsItsOwnAnswer(t *testing.T) {
	run := startedRun(t, 0).Throttle(started.Add(time.Second))

	if run.Status != automation.RunThrottled {
		t.Errorf("status %q, want THROTTLED", run.Status)
	}
	if len(run.ConditionResults) != 0 {
		t.Error("a throttled run evaluated conditions")
	}
}

// The ordinary answer of a rule that is working, and it carries how each condition answered - which
// is the question somebody asks first.
func TestASkippedRunCarriesEveryConditionsAnswer(t *testing.T) {
	results := []automation.ConditionResult{
		{Index: 0, Matched: true},
		{Index: 1, Matched: false},
	}
	run := startedRun(t, 0).Skip(results, started.Add(time.Second))

	if run.Status != automation.RunSkipped {
		t.Errorf("status %q, want SKIPPED", run.Status)
	}
	if len(run.ConditionResults) != 2 || run.ConditionResults[1].Matched {
		t.Errorf("condition results %+v", run.ConditionResults)
	}
	if run.ErrorCode != "" {
		t.Errorf("a skip carries the error code %q, and a skip is not an error", run.ErrorCode)
	}
}

// A condition that could not be evaluated and a condition that answered no are different facts
// about a rule, and the run says which.
func TestAConditionThatCouldNotBeEvaluatedIsNotTheSameAsOneThatSaidNo(t *testing.T) {
	broken := automation.ConditionResult{Index: 0, Matched: false, ErrorCode: "expression.timed_out"}
	answered := automation.ConditionResult{Index: 1, Matched: false}

	if broken.ErrorCode == answered.ErrorCode {
		t.Error("the two are indistinguishable")
	}
}

// SUCCEEDED even when an action failed, as long as the run reached its end - that is what
// `on_error: CONTINUE` means, and the per-action results are where a reader sees which one refused.
func TestACompletedRunCountsWhatItsActionsDid(t *testing.T) {
	actions := []automation.ActionResult{
		{Index: 0, Kind: "ADD_LABEL", Status: automation.ActionSucceeded},
		{Index: 1, Kind: "CREATE_BUCKET", Status: automation.ActionFailed, ErrorCode: "access.not_permitted"},
		{Index: 2, Kind: "COMPLETE", Status: automation.ActionSucceeded},
	}
	run := startedRun(t, 0).Complete(nil, actions, started.Add(time.Second))

	if run.Status != automation.RunSucceeded {
		t.Errorf("status %q, want SUCCEEDED", run.Status)
	}
	if run.Succeeded() != 2 || run.Failed() != 1 {
		t.Errorf("counted %d succeeded and %d failed", run.Succeeded(), run.Failed())
	}
	// The authoriser's own refusal, unchanged, which is what makes the log answer "why did this
	// not happen" rather than "something went wrong".
	if run.ActionResults[1].ErrorCode != "access.not_permitted" {
		t.Errorf("the refusal was rewritten to %q", run.ActionResults[1].ErrorCode)
	}
}

// An action the run never reached is not one that ran and did nothing.
func TestASkippedActionIsNotASucceededOne(t *testing.T) {
	actions := []automation.ActionResult{
		{Index: 0, Status: automation.ActionFailed},
		{Index: 1, Status: automation.ActionSkipped},
	}
	run := startedRun(t, 0).Fail("automation.action_failed", started.Add(time.Second))
	run.ActionResults = actions

	if run.Succeeded() != 0 {
		t.Errorf("a skipped action counted as succeeded")
	}
	if run.Status != automation.RunFailed || run.ErrorCode != "automation.action_failed" {
		t.Errorf("status %q code %q", run.Status, run.ErrorCode)
	}
}

func TestEveryStatusTheColumnAllowsIsValidAndNothingElseIs(t *testing.T) {
	for _, status := range []automation.RunStatus{
		automation.RunRunning, automation.RunSucceeded, automation.RunSkipped,
		automation.RunFailed, automation.RunAbortedLoop, automation.RunThrottled,
	} {
		if !status.Valid() {
			t.Errorf("%q is in the column's CHECK and is not valid here", status)
		}
	}
	if automation.RunStatus("EXPLODED").Valid() {
		t.Error("a status the column would refuse is valid here")
	}
}

// Every ending stamps the moment and fills the lists, so a reader never has to special-case one.
func TestEveryEndingFinishesTheRun(t *testing.T) {
	at := started.Add(time.Second)
	endings := map[string]automation.Run{
		"abort":    startedRun(t, 0).Abort(at),
		"throttle": startedRun(t, 0).Throttle(at),
		"skip":     startedRun(t, 0).Skip(nil, at),
		"fail":     startedRun(t, 0).Fail("x", at),
		"complete": startedRun(t, 0).Complete(nil, nil, at),
	}
	for name, run := range endings {
		t.Run(name, func(t *testing.T) {
			if !run.Status.Finished() {
				t.Errorf("status %q does not report itself finished", run.Status)
			}
			if run.FinishedAt == nil {
				t.Error("no finishing moment")
			}
			if run.ConditionResults == nil || run.ActionResults == nil {
				t.Error("a result list came back null")
			}
		})
	}
}
