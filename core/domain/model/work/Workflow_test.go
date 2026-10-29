// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var (
	workflowNow        = time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
	workflowCollection = shared.MustParseID("0192f000-0000-7000-8000-0000000000f1")
	workflowHub        = shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
)

func serviceShape() work.WorkflowShape {
	return work.WorkflowShape{
		Statuses: []work.WorkflowStatus{
			{Key: "todo", Name: "To do", Category: work.CategoryTodo},
			{Key: "doing", Name: "In progress", Category: work.CategoryInProgress, ColorToken: "blue"},
			{Key: "waiting", Name: "Waiting for customer", Category: work.CategoryInProgress},
			{Key: "done", Name: "Done", Category: work.CategoryDone, ColorToken: "green"},
			{Key: "rejected", Name: "Rejected", Category: work.CategoryDone, ColorToken: "red"},
		},
		Steps: []work.WorkflowStep{
			{Key: "start", Name: "Start", From: []string{"todo"}, To: "doing"},
			{Key: "finish", Name: "Done", From: []string{"doing", "waiting"}, To: "done",
				Requires: []string{"custom_fields.resolution"}, Asks: []string{"custom_fields.resolution"}},
			{Key: "reject", Name: "Reject", To: "rejected", Who: work.StepWho{Roles: []string{"ADMIN"}}},
		},
	}
}

// UC-FLW-02 check 1: a workflow has statuses in one category each, with a name and a colour from
// the token set, and steps from one or more statuses - or from any - to one status.
func TestNewWorkflowShapeRefusesWhatIsNotAWorkflow(t *testing.T) {
	cases := []struct {
		name   string
		change func(*work.WorkflowShape)
		code   string
	}{
		{"no statuses", func(s *work.WorkflowShape) { s.Statuses = nil }, "workflows.statuses_empty"},
		{"a malformed key", func(s *work.WorkflowShape) { s.Statuses[0].Key = "To Do" }, "workflows.key_malformed"},
		{"a key twice", func(s *work.WorkflowShape) { s.Statuses[1].Key = "todo" }, "workflows.key_taken"},
		{"an unknown category", func(s *work.WorkflowShape) { s.Statuses[0].Category = "LATER" }, "workflows.category_unknown"},
		{"a colour value", func(s *work.WorkflowShape) { s.Statuses[0].ColorToken = "ff0000" }, "workflows.color_token_unknown"},
		{"an unnamed status", func(s *work.WorkflowShape) { s.Statuses[2].Name = " " }, "workflows.name_empty"},
		{"no done status", func(s *work.WorkflowShape) {
			s.Statuses = s.Statuses[:3]
			s.Steps = nil
		}, "workflows.no_done_status"},
		{"a done initial status", func(s *work.WorkflowShape) { s.Initial = "done" }, "workflows.initial_invalid"},
		{"a step to nowhere", func(s *work.WorkflowShape) { s.Steps[0].To = "later" }, "workflows.status_unknown"},
		{"a step from nowhere", func(s *work.WorkflowShape) { s.Steps[0].From = []string{"later"} }, "workflows.status_unknown"},
		{"a step only from its target", func(s *work.WorkflowShape) { s.Steps[0].From = []string{"doing"} }, "workflows.step_goes_nowhere"},
		{"a step key twice", func(s *work.WorkflowShape) { s.Steps[1].Key = "start" }, "workflows.key_taken"},
		{"an unknown role", func(s *work.WorkflowShape) { s.Steps[2].Who.Roles = []string{"BOSS"} }, "workflows.role_unknown"},
		{"an unknown person field", func(s *work.WorkflowShape) { s.Steps[2].Who.PersonFields = []string{"owner"} }, "workflows.field_unknown"},
		{"a required field that is none", func(s *work.WorkflowShape) { s.Steps[1].Requires = []string{"colour"} }, "workflows.field_unknown"},
		{"an empty check", func(s *work.WorkflowShape) {
			s.Steps[1].Checks = []work.StepCheck{{Key: "amount", Holds: " "}}
		}, "workflows.check_empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			shape := serviceShape()
			c.change(&shape)
			_, err := work.NewWorkflowShape(shape, false, text.Composing{})
			var coded *shared.Error
			if !errors.As(err, &coded) || coded.DetailCode != c.code {
				t.Fatalf("got %v, want %s", err, c.code)
			}
		})
	}

	shape, err := work.NewWorkflowShape(serviceShape(), false, text.Composing{})
	if err != nil {
		t.Fatalf("a good workflow was refused: %v", err)
	}
	if shape.Initial != "todo" || shape.Statuses[0].ColorToken != "slate" {
		t.Fatalf("the first open status starts and an unset colour is slate: %+v", shape)
	}
}

// UC-FLW-02 check 7: the shipped workflow is valid as it is, and its statuses need no name.
func TestTheSystemWorkflowWorksWithoutEditing(t *testing.T) {
	system := work.SystemWorkflow()
	shape, err := work.NewWorkflowShape(system.Published, true, text.Composing{})
	if err != nil {
		t.Fatalf("the shipped workflow is refused: %v", err)
	}
	if shape.Initial != "todo" || shape.FirstDone() != "done" {
		t.Fatalf("starts in todo, ends in done: %+v", shape)
	}
	if len(shape.StepsFrom("todo")) != 2 || len(shape.StepsFrom("done")) != 2 {
		t.Fatalf("from every status a step leads to each other one: %+v", shape.StepsFrom("todo"))
	}
	if _, err := work.NewWorkflowShape(system.Published, false, text.Composing{}); err == nil {
		t.Fatal("a workspace's own workflow names every status")
	}
	set := work.NewWorkflows(nil)
	if found, ok := set.Find(work.SystemWorkflowKey); !ok || !found.System {
		t.Fatal("every workspace has the shipped workflow")
	}
}

// UC-FLW-02 check 3, UC-FLW-05 check 1: a draft changes nothing until it is published.
func TestADraftIsInForceOnlyOncePublished(t *testing.T) {
	created, err := work.NewWorkflow(work.NewWorkflowInput{
		ID: entryTypeID, TenantID: tenant, Key: "service", Name: "Service", Shape: serviceShape(),
		Now: workflowNow, Text: text.Composing{},
	})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	next := serviceShape()
	next.Statuses = slices.Delete(next.Statuses, 2, 3)
	next.Steps[1].From = []string{"doing"}
	drafted, err := created.WithDraft(next, workflowNow, text.Composing{})
	if err != nil {
		t.Fatalf("the draft was refused: %v", err)
	}
	if _, kept := drafted.Published.Status("waiting"); !kept {
		t.Fatal("the shape in force changed with the draft")
	}
	published, err := drafted.Publish(tenant, workflowNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("publishing failed: %v", err)
	}
	if _, kept := published.Published.Status("waiting"); kept || published.Draft != nil {
		t.Fatal("publishing puts the draft in force and ends it")
	}
	if _, err := published.Publish(tenant, workflowNow); err == nil {
		t.Fatal("there is nothing to publish without a draft")
	}
	if _, err := work.NewWorkflow(work.NewWorkflowInput{
		ID: entryTypeID, TenantID: tenant, Key: work.SystemWorkflowKey, Name: "Mine", Shape: serviceShape(),
		Now: workflowNow, Text: text.Composing{},
	}); err == nil {
		t.Fatal("the shipped workflow's key is reserved")
	}
}

// UC-FLW-02 check 4, UC-FLW-05 checks 2 and 6: entering keeps what fits, a removed status goes
// where the mapping says, and no move changes completion.
func TestPlanStatusMoves(t *testing.T) {
	before, _ := work.NewWorkflowShape(serviceShape(), false, text.Composing{})
	after := before
	after.Statuses = slices.Delete(slices.Clone(before.Statuses), 2, 3)
	after.Steps = nil
	groups := []work.StatusGroup{
		{CollectionID: workflowCollection, EntryType: "task", Status: "waiting", Count: 12},
		{CollectionID: workflowCollection, EntryType: "task", Status: "doing", Count: 3},
		{CollectionID: workflowCollection, EntryType: "task", Status: "", Count: 37},
		{CollectionID: workflowCollection, EntryType: "task", Status: "", Completed: true, Count: 5},
	}
	if removed := work.RemovedStatuses(before, after, groups); removed["waiting"] != 12 || len(removed) != 1 {
		t.Fatalf("the publish asks for the removed status with its count: %v", removed)
	}
	moves, err := work.PlanStatusMoves(groups, &before, &after, map[string]string{"waiting": "doing"})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	want := map[string]string{"waiting": "doing", "": "todo"}
	if len(moves) != 3 {
		t.Fatalf("three groups move: %+v", moves)
	}
	for _, move := range moves {
		if move.Completed {
			if move.To != "done" {
				t.Fatalf("a done entry enters the first done status: %+v", move)
			}
			continue
		}
		if want[move.From] != move.To {
			t.Fatalf("%q moved to %q", move.From, move.To)
		}
	}
	if _, err := work.PlanStatusMoves(groups, &before, &after, map[string]string{"waiting": "rejected"}); err == nil {
		t.Fatal("a mapping into done would complete entries without a step")
	}
	unmapped, _ := work.PlanStatusMoves(groups[:1], &before, &after, nil)
	if len(unmapped) != 1 || unmapped[0].To != "todo" {
		t.Fatalf("an unmapped removed status falls back to the initial one: %+v", unmapped)
	}
	leaving, _ := work.PlanStatusMoves(groups[:2], &before, nil, nil)
	if len(leaving) != 2 || leaving[0].To != "" {
		t.Fatalf("leaving every workflow clears the status: %+v", leaving)
	}
}

// The scheme: a collection's row over its hub's, a type's over the row for every type.
func TestResolveWorkflow(t *testing.T) {
	schemes := []work.WorkflowScheme{
		{ContainerID: workflowHub, EntryType: "", WorkflowKey: "hub_all"},
		{ContainerID: workflowHub, EntryType: "bug", WorkflowKey: "hub_bug"},
		{ContainerID: workflowCollection, EntryType: "task", WorkflowKey: "own_task"},
	}
	cases := []struct {
		entryType, want string
	}{{"task", "own_task"}, {"bug", "hub_bug"}, {"feature", "hub_all"}}
	for _, c := range cases {
		got, found := work.ResolveWorkflow(schemes, workflowCollection, workflowHub, c.entryType)
		if !found || got != c.want {
			t.Fatalf("%s: got %q", c.entryType, got)
		}
	}
	if _, found := work.ResolveWorkflow(schemes[2:], workflowCollection, workflowHub, "bug"); found {
		t.Fatal("no row, no workflow")
	}
}

// UC-FLW-03 check 3: a step names each field it still needs and each check that fails.
func TestStepRefusedNamesWhatIsMissing(t *testing.T) {
	shape := serviceShape()
	step := shape.Steps[1]
	missing := work.StepRequirementsMissing(step, work.WorkItem{CustomFields: map[string]any{"resolution": " "}})
	if !slices.Equal(missing, []string{"custom_fields.resolution"}) {
		t.Fatalf("missing: %v", missing)
	}
	err := work.StepRefused(step, missing, []work.StepCheck{{Key: "amount", Message: "Above zero"}})
	var coded *shared.Error
	if !errors.As(err, &coded) || coded.DetailCode != "workflows.step_refused" || len(coded.Fields) != 2 ||
		coded.Fields[1].Params["message"] != "Above zero" {
		t.Fatalf("refusal: %+v", err)
	}
	if !step.LeavesFrom("waiting") || step.LeavesFrom("todo") || shape.Steps[2].LeavesFrom("rejected") {
		t.Fatal("a step leaves from its statuses, and never from its target")
	}
}
