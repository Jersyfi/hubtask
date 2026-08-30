// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// readOnlyWork is the dry run's whole promise as a fake: a writing transaction fails the test.
type readOnlyWork struct{ t *testing.T }

func (w readOnlyWork) Within(context.Context, persistence.Scope, func(context.Context) error) error {
	w.t.Fatal("the dry run opened a writing transaction")
	return nil
}

func (w readOnlyWork) WithinReadOnly(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	return fn(ctx)
}

func dryRun(t *testing.T, rule domain.Rule) TestRule {
	t.Helper()
	return TestRule{
		Rules:     newRuleStore(rule),
		Catalogue: defaultCatalogue(), Conditions: compiler{},
		Authorizer: &authorizer{},
		UnitOfWork: readOnlyWork{t: t}, Clock: clock.Fixed(now), IDs: ids{next: newRuleID},
	}
}

func sample() SampleEvent {
	return SampleEvent{Type: "de.hubtask.work.item.updated.v1", Subject: "item/x"}
}

func testActor() appshared.ActorContext {
	actor := writerActor()
	actor.Scopes = append(actor.Scopes, "automation:manage")
	return actor
}

// The acceptance criterion: the dry run reports condition results for the sample event and which
// actions would run - and changes nothing, proved here by the transaction shape and in the e2e
// session by the checksum discipline.
func TestADryRunReportsWhatWouldHappenAndChangesNothing(t *testing.T) {
	rule := enabledRule()
	rule.Conditions = []domain.Condition{{Expr: "event.type != ''"}, {Expr: "true"}}
	rule.Actions = []domain.Action{
		branchOf("true",
			[]map[string]any{{"kind": "ADD_LABEL"}},
			[]map[string]any{{"kind": "CREATE_BUCKET"}},
		),
		{Kind: "ADD_LABEL"},
	}
	h := dryRun(t, rule)

	outcome, err := h.Execute(context.Background(), testActor(), TestCommand{
		RuleID: rule.ID, Event: sample(),
	})
	if err != nil {
		t.Fatalf("testing: %v", err)
	}
	if !outcome.Matched {
		t.Error("the sample did not match")
	}
	if len(outcome.Conditions) != 2 {
		t.Fatalf("%d condition results, want every condition answered", len(outcome.Conditions))
	}

	// Both arms are reported - more than a run's log shows - and the not-taken one says so.
	byPath := map[string]PlannedAction{}
	for _, planned := range outcome.Actions {
		byPath[planned.Path] = planned
	}
	if branch := byPath["0"]; branch.Matched == nil || !*branch.Matched || !branch.WouldRun {
		t.Errorf("the branch reads %+v", branch)
	}
	if taken := byPath["0/then/0"]; !taken.WouldRun || taken.Kind != "ADD_LABEL" {
		t.Errorf("the taken arm reads %+v", taken)
	}
	if skipped := byPath["0/else/0"]; skipped.WouldRun || skipped.Kind != "CREATE_BUCKET" {
		t.Errorf("the not-taken arm reads %+v", skipped)
	}
	if last := byPath["1"]; !last.WouldRun {
		t.Errorf("the action after the branch reads %+v", last)
	}
}

// A condition that says no still answers every condition, and no action would run.
func TestADryRunWhoseConditionSaysNoRunsNothing(t *testing.T) {
	rule := enabledRule()
	rule.Conditions = []domain.Condition{{Expr: "false"}, {Expr: "true"}}
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
	h := dryRun(t, rule)

	outcome, err := h.Execute(context.Background(), testActor(), TestCommand{
		RuleID: rule.ID, Event: sample(),
	})
	if err != nil {
		t.Fatalf("testing: %v", err)
	}
	if outcome.Matched {
		t.Error("false matched")
	}
	if len(outcome.Conditions) != 2 {
		t.Errorf("%d condition results, want both", len(outcome.Conditions))
	}
	if outcome.Actions[0].WouldRun {
		t.Error("an action would run under a condition that said no")
	}
}

// A STOP ends the plan exactly as it ends a run: everything after it would not run.
func TestADryRunHonoursAStop(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{
		{Kind: "ADD_LABEL"}, {Kind: domain.ActionStop}, {Kind: "CREATE_BUCKET"},
	}
	h := dryRun(t, rule)

	outcome, err := h.Execute(context.Background(), testActor(), TestCommand{
		RuleID: rule.ID, Event: sample(),
	})
	if err != nil {
		t.Fatalf("testing: %v", err)
	}
	if !outcome.Actions[0].WouldRun || !outcome.Actions[1].WouldRun {
		t.Errorf("the actions before and at the STOP read %+v", outcome.Actions[:2])
	}
	if outcome.Actions[2].WouldRun {
		t.Error("an action after the STOP would run")
	}
}

// An inline definition goes through the same validation the create runs: a rule the dry run
// accepts is a rule the create will accept.
func TestADryRunValidatesAnInlineDefinition(t *testing.T) {
	h := dryRun(t, enabledRule())

	definition := validCommand()
	definition.Actions = []domain.Action{{Kind: "NO_SUCH_ACTION"}}
	_, err := h.Execute(context.Background(), testActor(), TestCommand{
		Rule: &definition, Event: sample(),
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want the create's own refusal", err)
	}

	definition = validCommand()
	outcome, err := h.Execute(context.Background(), testActor(), TestCommand{
		Rule: &definition, Event: sample(),
	})
	if err != nil {
		t.Fatalf("a valid definition was refused: %v", err)
	}
	if len(outcome.Actions) != 1 || !outcome.Actions[0].WouldRun {
		t.Errorf("the plan reads %+v", outcome.Actions)
	}
}

// Exactly one source, and a sample with a type: each refusal names its field.
func TestADryRunNeedsOneSourceAndATypedSample(t *testing.T) {
	h := dryRun(t, enabledRule())
	definition := validCommand()

	_, err := h.Execute(context.Background(), testActor(), TestCommand{Event: sample()})
	if code := detailOf(t, err); code != "automation.test_source_required" {
		t.Errorf("no source refused with %s", code)
	}

	_, err = h.Execute(context.Background(), testActor(), TestCommand{
		RuleID: ruleID, Rule: &definition, Event: sample(),
	})
	if code := detailOf(t, err); code != "automation.test_source_required" {
		t.Errorf("two sources refused with %s", code)
	}

	_, err = h.Execute(context.Background(), testActor(), TestCommand{RuleID: ruleID})
	if code := detailOf(t, err); code != "automation.test_event_type_required" {
		t.Errorf("a sample without a type refused with %s", code)
	}
}
