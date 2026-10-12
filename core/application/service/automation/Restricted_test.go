// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/jumble"
	privacy "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// No rule runs as or about a person whose processing is restricted (UC-PRV-04 check 4, UC-PRV-03
// check 10, data-protection.md §4): the run is SKIPPED with the code that says why, nothing of it
// is evaluated or done, and it never counts towards switching the rule off.

var restrictedPerson = shared.ID("01936f2a-7c1e-7000-8000-0000000000f1")

type restrictedPeople []privacy.RestrictedPerson

func (r restrictedPeople) RestrictedPeople(context.Context) ([]privacy.RestrictedPerson, error) {
	return r, nil
}

func restrictedEngine(t *testing.T, rule domain.Rule, envelope event.Envelope, item work.WorkItem) *engineHarness {
	t.Helper()
	h := newEngine(t, rule)
	h.engine.Source = source{envelope: envelope}
	h.engine.Entries = entries{rows: map[shared.ID]work.WorkItem{itemID: item}}
	h.engine.Restrictions = restrictedPeople{{AccountID: restrictedPerson, Email: "rae@example.org"}}
	return h
}

func TestARunAsOrAboutARestrictedPersonIsWithheld(t *testing.T) {
	colleague := shared.ID("01936f2a-7c1e-7000-8000-0000000000f2")
	theirs := work.WorkItem{ID: itemID, CreatedBy: colleague}
	for _, testCase := range []struct {
		name     string
		runAs    shared.ID
		actor    shared.ID
		item     work.WorkItem
		wantCode string
	}{
		{"the rule acts as the restricted person", restrictedPerson, colleague, theirs, CodeRunAsRestricted},
		{"the restricted person caused the event", colleague, restrictedPerson, theirs, CodeSubjectRestricted},
		{"the entry was created by the restricted person", colleague, colleague,
			work.WorkItem{ID: itemID, CreatedBy: restrictedPerson}, CodeSubjectRestricted},
		{"the entry is assigned to the restricted person", colleague, colleague,
			work.WorkItem{ID: itemID, CreatedBy: colleague, AssigneeID: restrictedPerson}, CodeSubjectRestricted},
		{"nobody restricted is involved", colleague, colleague, theirs, ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			rule := enabledRule()
			rule.RunAs = testCase.runAs
			rule.Conditions = []domain.Condition{{Expr: "true"}}
			rule.Actions = []domain.Action{{Kind: "ADD_LABEL", Params: map[string]any{"label_id": "x"}}}
			envelope := itemEvent()
			envelope.Actor = event.Actor{Kind: shared.ActorUser, ID: testCase.actor}
			h := restrictedEngine(t, rule, envelope, testCase.item)

			run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
			if err != nil {
				t.Fatalf("running: %v", err)
			}
			if testCase.wantCode == "" {
				if run.Status != domain.RunSucceeded || len(h.dispatcher.calls) != 1 {
					t.Errorf("an ordinary run read %s with %d actions", run.Status, len(h.dispatcher.calls))
				}
				return
			}
			if run.Status != domain.RunSkipped || run.ErrorCode != testCase.wantCode {
				t.Errorf("the run read %s / %q, want SKIPPED / %s", run.Status, run.ErrorCode, testCase.wantCode)
			}
			if len(h.dispatcher.calls) != 0 || len(run.ConditionResults) != 0 {
				t.Errorf("a withheld run evaluated %d conditions and did %d actions",
					len(run.ConditionResults), len(h.dispatcher.calls))
			}
			if stored := h.runs.last(); stored.Status != domain.RunSkipped || stored.ErrorCode != testCase.wantCode {
				t.Errorf("the stored run says %s / %q", stored.Status, stored.ErrorCode)
			}
		})
	}
}

// More withheld runs than the streak that switches a rule off: the rule stays on, its streak at
// zero - a restriction is a person's right, not the rule's failure.
func TestAWithheldRunNeverCountsTowardsSwitchingTheRuleOff(t *testing.T) {
	rule := enabledRule()
	rule.RunAs = restrictedPerson
	h := restrictedEngine(t, rule, itemEvent(), work.WorkItem{ID: itemID})
	for i := 0; i <= MaxConsecutiveFailures; i++ {
		if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
			t.Fatalf("running: %v", err)
		}
	}
	if h.failures.count != 0 || h.failures.disabled || h.failures.threshold != 0 {
		t.Errorf("the streak is %d, disabled %v", h.failures.count, h.failures.disabled)
	}
	if h.failures.cleared != MaxConsecutiveFailures+1 {
		t.Errorf("the streak was cleared %d times", h.failures.cleared)
	}
}

// What a restricted person mailed into the intake is theirs: a rule on it does not run.
func TestARunAboutARestrictedPersonsIntakeIsWithheld(t *testing.T) {
	entryID := shared.ID("01936f2a-7c1e-7000-8000-0000000000f3")
	rule := enabledRule()
	rule.Trigger = domain.Trigger{Kind: domain.TriggerJumbleEntry}
	h := restrictedEngine(t, rule, event.Envelope{}, work.WorkItem{})
	h.engine.Jumble = jumbleEntries{entry: jumble.Entry{ID: entryID, Sender: "RAE@example.org"}}

	run, err := h.engine.Execute(context.Background(), engineActor(), Command{
		RuleID: ruleID, Trigger: domain.TriggerJumbleEntry, SubjectID: entryID, Occasion: "o",
	})
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunSkipped || run.ErrorCode != CodeSubjectRestricted {
		t.Errorf("the run read %s / %q", run.Status, run.ErrorCode)
	}
}

// A restriction placed while a run waits holds the rest of it back: what was done stays recorded,
// nothing more is done.
func TestARestrictionPlacedDuringAWaitWithholdsTheRest(t *testing.T) {
	rule := waitingRule()
	rule.RunAs = restrictedPerson
	h := newEngine(t, rule)
	parked, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil || parked.Status != domain.RunWaiting {
		t.Fatalf("parking: %v, %s", err, parked.Status)
	}

	h.engine.Restrictions = restrictedPeople{{AccountID: restrictedPerson}}
	resumed, err := h.engine.Execute(context.Background(), engineActor(), resumeCommand(t, h.jobs.requests[0]))
	if err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if resumed.Status != domain.RunSkipped || resumed.ErrorCode != CodeRunAsRestricted {
		t.Errorf("the resumed run read %s / %q", resumed.Status, resumed.ErrorCode)
	}
	if len(h.dispatcher.calls) != 1 || len(resumed.ActionResults) != 1 {
		t.Errorf("after the restriction %d actions ran, %d recorded", len(h.dispatcher.calls), len(resumed.ActionResults))
	}
	if h.failures.count != 0 {
		t.Errorf("the withheld resume counted as a failure")
	}
}
