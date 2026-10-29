// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// workflowStore is an in-memory Workflows: rows and scheme, and the moves asked of it.
type workflowStore struct {
	rows    []domain.Workflow
	schemes []domain.WorkflowScheme
	groups  []domain.StatusGroup
	moves   []domain.StatusMove
}

func (s *workflowStore) List(context.Context) ([]domain.Workflow, error) { return s.rows, nil }
func (s *workflowStore) Insert(_ context.Context, w domain.Workflow) error {
	s.rows = append(s.rows, w)
	return nil
}

func (s *workflowStore) Update(_ context.Context, w domain.Workflow, _ int) error {
	for i := range s.rows {
		if s.rows[i].Key == w.Key {
			s.rows[i] = w
		}
	}
	return nil
}
func (s *workflowStore) Delete(context.Context, string) error { return nil }
func (s *workflowStore) Schemes(context.Context) ([]domain.WorkflowScheme, error) {
	return s.schemes, nil
}

func (s *workflowStore) SetScheme(_ context.Context, scheme domain.WorkflowScheme, _ time.Time) error {
	s.schemes = append(s.schemes, scheme)
	return nil
}
func (s *workflowStore) RemoveScheme(context.Context, shared.ID, string) error { return nil }
func (s *workflowStore) StatusGroups(context.Context, []shared.ID) ([]domain.StatusGroup, error) {
	return s.groups, nil
}

func (s *workflowStore) MoveStatuses(_ context.Context, move domain.StatusMove, _ time.Time) ([]repository.MovedEntry, error) {
	s.moves = append(s.moves, move)
	return []repository.MovedEntry{{ID: taskID, Version: 2}}, nil
}

// roleOf answers one role for everybody.
type roleOf struct{ role identity.Role }

func (r roleOf) RoleAlong(context.Context, appshared.ActorContext, []identity.Scope) (identity.Role, bool, error) {
	return r.role, r.role != "", nil
}

func reviewWorkflow() domain.Workflow {
	return domain.Workflow{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000004f1"), TenantID: tenantID,
		Key: "review", Name: "Review", Version: 1,
		Published: domain.WorkflowShape{
			Statuses: []domain.WorkflowStatus{
				{Key: "todo", Name: "To do", Category: domain.CategoryTodo},
				{Key: "review", Name: "In review", Category: domain.CategoryInProgress},
				{Key: "done", Name: "Done", Category: domain.CategoryDone},
				{Key: "rejected", Name: "Rejected", Category: domain.CategoryDone},
			},
			Initial: "todo",
			Steps: []domain.WorkflowStep{
				{Key: "submit", From: []string{"todo"}, To: "review"},
				{Key: "accept", From: []string{"review"}, To: "done", Requires: []string{"custom_fields.resolution"}},
				{Key: "back", From: []string{"review"}, To: "todo"},
				{Key: "reject", To: "rejected", Who: domain.StepWho{Roles: []string{"OWNER"}}},
				{Key: "reopen", From: []string{"done", "rejected"}, To: "todo"},
			},
		},
	}
}

func workflowHarness(t *testing.T) (*completionHarness, *workflowStore) {
	t.Helper()
	h := newCompletionHarness(domain.CompletionManual)
	store := &workflowStore{
		rows:    []domain.Workflow{reviewWorkflow()},
		schemes: []domain.WorkflowScheme{{ContainerID: hubID, WorkflowKey: "review"}},
	}
	h.writer.Workflow = WorkflowGate{Workflows: store, Compiler: &answers{of: map[string]bool{}}, Roles: roleOf{identity.RoleMember}}
	task := h.items.stored[taskID]
	task.Status = "review"
	h.items.stored[taskID] = task
	return h, store
}

func refusedWith(t *testing.T, err error, code string) *shared.Error {
	t.Helper()
	var coded *shared.Error
	if !errors.As(err, &coded) || coded.DetailCode != code {
		t.Fatalf("answered %v, want %s", err, code)
	}
	return coded
}

// UC-FLW-03 check 1: the entry offers exactly the steps the person may take from its status.
func TestTheEntryOffersOnlyTheStepsThePersonMayTake(t *testing.T) {
	h, _ := workflowHarness(t)
	steps, err := ListEntrySteps{Completion: h.writer}.Execute(t.Context(), actorFixture(), taskID)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if !steps.InForce || steps.Status != "review" {
		t.Fatalf("the hub's workflow runs for the entry: %+v", steps)
	}
	var keys []string
	for _, verdict := range steps.Offered {
		keys = append(keys, verdict.Step.Key)
	}
	if len(keys) != 2 || keys[0] != "accept" || keys[1] != "back" {
		t.Fatalf("offered %v: reject is the owner's only", keys)
	}
}

// UC-FLW-03 checks 3 and 6, D-02: a step that needs a field is refused naming it; given it, the
// step is taken into done, which completes the entry, with the step in its history.
func TestTakingAStepIntoDoneCompletesTheEntry(t *testing.T) {
	h, _ := workflowHarness(t)
	take := TakeStep{Completion: h.writer}
	_, err := take.Execute(t.Context(), actorFixture(), TakeStepCommand{ItemID: taskID, Step: "accept"})
	refusal := refusedWith(t, err, "workflows.step_refused")
	if len(refusal.Fields) != 1 || refusal.Fields[0].Params["key"] != "custom_fields.resolution" {
		t.Fatalf("the refusal names the field: %+v", refusal.Fields)
	}

	task := h.items.stored[taskID]
	task.CustomFields = map[string]any{"resolution": "fixed"}
	h.items.stored[taskID] = task
	item, err := take.Execute(t.Context(), actorFixture(), TakeStepCommand{ItemID: taskID, Step: "accept"})
	if err != nil {
		t.Fatalf("the step was refused: %v", err)
	}
	if item.Status != "done" || !item.Completion.IsCompleted {
		t.Fatalf("into done completes: %+v", item)
	}
	var verbs []activity.Verb
	for _, entry := range h.history.entries {
		verbs = append(verbs, entry.Verb)
	}
	if len(verbs) != 2 || verbs[0] != activity.ItemCompleted || verbs[1] != activity.ItemStepTaken {
		t.Fatalf("the history records the completion and the step: %v", verbs)
	}
	step := h.history.entries[1].ChangeSet
	if status, _ := step["status"].(map[string]any); status["from"] != "review" || status["to"] != "done" {
		t.Fatalf("the history says from which status: %v", step)
	}
}

// UC-FLW-03 check 2: a step the person may not take, or that does not leave the status, is
// refused whatever door asks.
func TestAStepNotOfferedIsRefused(t *testing.T) {
	h, _ := workflowHarness(t)
	take := TakeStep{Completion: h.writer}
	_, err := take.Execute(t.Context(), actorFixture(), TakeStepCommand{ItemID: taskID, Step: "reject"})
	refusedWith(t, err, "workflows.step_not_allowed")
	_, err = take.Execute(t.Context(), actorFixture(), TakeStepCommand{ItemID: taskID, Step: "submit"})
	refusedWith(t, err, "workflows.step_not_offered")

	item, err := take.Execute(t.Context(), actorFixture(), TakeStepCommand{ItemID: taskID, ToStatus: "todo"})
	if err != nil || item.Status != "todo" || item.Completion.IsCompleted {
		t.Fatalf("a drop on a column takes the step there: %+v %v", item, err)
	}
}

// UC-FLW-03 check 4: completing on any door takes a step into done the person may take, or is
// refused with the reason.
func TestCompletingInAWorkflowCollectionTakesAStep(t *testing.T) {
	h, _ := workflowHarness(t)
	_, err := CompleteWorkItem{Completion: h.writer}.Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: taskID})
	refusedWith(t, err, "workflows.step_refused")

	task := h.items.stored[taskID]
	task.CustomFields = map[string]any{"resolution": "fixed"}
	h.items.stored[taskID] = task
	item, err := CompleteWorkItem{Completion: h.writer}.Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: taskID})
	if err != nil || item.Status != "done" || !item.Completion.IsCompleted {
		t.Fatalf("completing took accept: %+v %v", item, err)
	}
	item, err = ReopenWorkItem{Completion: h.writer}.Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: taskID})
	if err != nil || item.Status != "todo" || item.Completion.IsCompleted {
		t.Fatalf("reopening took reopen: %+v %v", item, err)
	}
	_, err = CompleteWorkItem{Completion: h.writer}.Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: taskID})
	refusedWith(t, err, "workflows.no_step_to_done")
}

// UC-FLW-04 checks 1 and 2: a step limited by a person field, and a check with the team's words.
func TestAStepsWhoAndChecks(t *testing.T) {
	h, store := workflowHarness(t)
	compiler := &answers{of: map[string]bool{"entry.custom_fields.amount > 0": false}}
	h.writer.Workflow.Compiler = compiler
	shape := store.rows[0].Published
	shape.Steps[2].Who = domain.StepWho{PersonFields: []string{"assignee"}}
	shape.Steps[2].Checks = []domain.StepCheck{{Key: "amount", Holds: "entry.custom_fields.amount > 0", Message: "An amount above zero"}}
	store.rows[0].Published = shape

	take := TakeStep{Completion: h.writer}
	_, err := take.Execute(t.Context(), actorFixture(), TakeStepCommand{ItemID: taskID, Step: "back"})
	refusedWith(t, err, "workflows.step_not_allowed")

	task := h.items.stored[taskID]
	task.AssigneeID = actorFixture().AccountID
	h.items.stored[taskID] = task
	_, err = take.Execute(t.Context(), actorFixture(), TakeStepCommand{ItemID: taskID, Step: "back"})
	refusal := refusedWith(t, err, "workflows.step_refused")
	if refusal.Fields[0].Params["message"] != "An amount above zero" {
		t.Fatalf("the refusal shows the team's words: %+v", refusal.Fields)
	}
	compiler.of["entry.custom_fields.amount > 0"] = true
	if _, err := take.Execute(t.Context(), actorFixture(), TakeStepCommand{ItemID: taskID, Step: "back"}); err != nil {
		t.Fatalf("the step holds: %v", err)
	}
}

// UC-FLW-02: a new entry in a workflow collection starts in the initial status.
func TestTheInitialStatusOfANewEntry(t *testing.T) {
	h, _ := workflowHarness(t)
	collection := h.containers.stored[collectionID]
	status, err := h.writer.Workflow.initialStatus(t.Context(), collection, domain.WorkItem{Type: domain.ItemTask})
	if err != nil || status != "todo" {
		t.Fatalf("starts in todo: %q %v", status, err)
	}
	h.writer.Workflow.Workflows = &workflowStore{}
	if status, _ := h.writer.Workflow.initialStatus(t.Context(), collection, domain.WorkItem{Type: domain.ItemTask}); status != "" {
		t.Fatalf("no workflow, no status: %q", status)
	}
}

// UC-FLW-05 checks 3, 4 and 6: the mover moves each group and writes a workflow change into each
// moved entry's history, taking no step.
func TestTheMoverRecordsAWorkflowChange(t *testing.T) {
	h, store := workflowHarness(t)
	mover := StatusMover{Workflows: store, Activity: h.writer.Activity, Changes: h.changes, HLC: &hlcSource{}}
	moved, err := mover.Move(t.Context(), actorFixture(), []domain.StatusMove{
		{CollectionID: collectionID, EntryType: "task", From: "waiting", To: "review", Count: 1},
	}, now)
	if err != nil || moved != 1 {
		t.Fatalf("moved %d: %v", moved, err)
	}
	if len(h.history.entries) != 1 || h.history.entries[0].Verb != activity.ItemWorkflowChanged {
		t.Fatalf("history: %+v", h.history.entries)
	}
}
