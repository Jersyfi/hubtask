// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package workflow_test

import (
	"context"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var (
	hub        = shared.MustParseID("018f0000-0000-7000-8000-0000000000b1")
	collection = shared.MustParseID("018f0000-0000-7000-8000-0000000000b2")
	entry      = shared.MustParseID("018f0000-0000-7000-8000-0000000000b3")
)

type memoryWorkflows struct {
	rows    []work.Workflow
	schemes []work.WorkflowScheme
	groups  []work.StatusGroup
	moves   []work.StatusMove
}

func (m *memoryWorkflows) List(context.Context) ([]work.Workflow, error) { return m.rows, nil }
func (m *memoryWorkflows) Insert(_ context.Context, w work.Workflow) error {
	m.rows = append(m.rows, w)
	return nil
}

func (m *memoryWorkflows) Update(_ context.Context, w work.Workflow, _ int) error {
	for i := range m.rows {
		if m.rows[i].Key == w.Key {
			w.Version = m.rows[i].Version + 1
			m.rows[i] = w
		}
	}
	return nil
}
func (m *memoryWorkflows) Delete(context.Context, string) error { return nil }
func (m *memoryWorkflows) Schemes(context.Context) ([]work.WorkflowScheme, error) {
	return m.schemes, nil
}

func (m *memoryWorkflows) SetScheme(_ context.Context, scheme work.WorkflowScheme, _ time.Time) error {
	m.schemes = append(m.schemes, scheme)
	return nil
}
func (m *memoryWorkflows) RemoveScheme(context.Context, shared.ID, string) error { return nil }
func (m *memoryWorkflows) StatusGroups(context.Context, []shared.ID) ([]work.StatusGroup, error) {
	return m.groups, nil
}

func (m *memoryWorkflows) MoveStatuses(_ context.Context, move work.StatusMove, _ time.Time) ([]repository.MovedEntry, error) {
	m.moves = append(m.moves, move)
	out := make([]repository.MovedEntry, 0, move.Count)
	for range move.Count {
		out = append(out, repository.MovedEntry{ID: entry, Version: 2})
	}
	return out, nil
}

type memoryContainers struct {
	repository.Containers
	rows map[shared.ID]work.Container
}

func (m memoryContainers) Find(_ context.Context, id shared.ID) (work.Container, error) {
	found, ok := m.rows[id]
	if !ok {
		return work.Container{}, shared.ErrNotFound
	}
	return found, nil
}

func (m memoryContainers) List(_ context.Context, query repository.ContainerQuery) (repository.ContainerPage, error) {
	var out []work.Container
	for _, row := range m.rows {
		if row.ParentID == query.ParentID && row.Type == work.ContainerCollection {
			out = append(out, row)
		}
	}
	return repository.ContainerPage{Containers: out}, nil
}

type history struct{ entries []activity.Entry }

func (h *history) Record(_ context.Context, e activity.Entry) error {
	h.entries = append(h.entries, e)
	return nil
}

type anyCondition struct{}

func (anyCondition) Compile(string, expression.Environment, expression.Result) (expression.Program, error) {
	return nil, nil
}

type deskFixture struct {
	fixture
	desk      workflow.Desk
	workflows *memoryWorkflows
	history   *history
}

func newDesk() deskFixture {
	f := deskFixture{fixture: newFixture(), workflows: &memoryWorkflows{}, history: &history{}}
	containers := memoryContainers{rows: map[shared.ID]work.Container{
		hub:        {ID: hub, TenantID: tenant, Type: work.ContainerHub},
		collection: {ID: collection, TenantID: tenant, Type: work.ContainerCollection, ParentID: hub},
	}}
	f.desk = workflow.Desk{
		Workflows: f.workflows, Containers: containers,
		Mover: workservice.StatusMover{
			Workflows: f.workflows, Activity: workservice.ActivityJournal{Entries: f.history, IDs: &ids{}},
			Changes: f.log, HLC: hlc{},
		},
		Compiler: anyCondition{}, Authorizer: f.auth, Changes: f.log, Audit: f.trail,
		UnitOfWork: unitOfWork{}, Clock: clock{}, IDs: &ids{}, HLC: hlc{}, Text: text.Composing{},
	}
	return f
}

func serviceShape(withWaiting bool) work.WorkflowShape {
	statuses := []work.WorkflowStatus{
		{Key: "todo", Name: "To do", Category: work.CategoryTodo},
		{Key: "doing", Name: "In progress", Category: work.CategoryInProgress},
	}
	if withWaiting {
		statuses = append(statuses, work.WorkflowStatus{Key: "waiting", Name: "Waiting for customer", Category: work.CategoryInProgress})
	}
	statuses = append(statuses, work.WorkflowStatus{Key: "done", Name: "Done", Category: work.CategoryDone})
	return work.WorkflowShape{Statuses: statuses}
}

// UC-FLW-02 checks 3 and 8, UC-FLW-05 checks 1 … 5: a draft changes nothing; publishing one that
// removes a status in use asks where its entries go, then moves them in one change, writes each
// one's history and the counts to the trail.
func TestPublishingADraftMovesTheEntriesOfARemovedStatus(t *testing.T) {
	f := newDesk()
	created, err := workflow.CreateWorkflow{Desk: f.desk}.Execute(t.Context(), actor(), "service", "Service", serviceShape(true))
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	f.workflows.schemes = []work.WorkflowScheme{{ContainerID: hub, WorkflowKey: "service"}}
	f.workflows.groups = []work.StatusGroup{{CollectionID: collection, EntryType: "task", Status: "waiting", Count: 12}}

	shape := serviceShape(false)
	if _, err := (workflow.SaveWorkflowDraft{Desk: f.desk}).Execute(t.Context(), actor(), workflow.SaveWorkflowDraftCommand{
		Key: created.Key, Shape: &shape,
	}); err != nil {
		t.Fatalf("drafting: %v", err)
	}
	if _, kept := f.workflows.rows[0].Published.Status("waiting"); !kept || len(f.workflows.moves) != 0 {
		t.Fatal("a draft changed what is in force")
	}

	publish := workflow.PublishWorkflow{Desk: f.desk}
	if _, _, err := publish.Execute(t.Context(), actor(), "service", nil, 0); detail(err) != "workflows.mapping_required" {
		t.Fatalf("a removed status in use needs its target: %v", err)
	}
	if _, _, err := publish.Execute(t.Context(), actor(), "service", map[string]string{"waiting": "done"}, 0); detail(err) != "workflows.mapping_changes_completion" {
		t.Fatalf("a move into done would complete without a step: %v", err)
	}
	published, moved, err := publish.Execute(t.Context(), actor(), "service", map[string]string{"waiting": "doing"}, 0)
	if err != nil || moved != 12 || published.Draft != nil {
		t.Fatalf("published %+v, moved %d: %v", published, moved, err)
	}
	if len(f.workflows.moves) != 1 || f.workflows.moves[0].To != "doing" {
		t.Fatalf("moves: %+v", f.workflows.moves)
	}
	if len(f.history.entries) != 12 || f.history.entries[0].Verb != activity.ItemWorkflowChanged {
		t.Fatalf("each moved entry's history says the workflow changed: %d", len(f.history.entries))
	}
	last := f.trail.entries[len(f.trail.entries)-1]
	if last.Action != workflow.WorkflowPublishedAction {
		t.Fatalf("the trail records the publish: %+v", last)
	}

	f.auth.refuse = true
	if _, err := (workflow.CreateWorkflow{Desk: f.desk}).Execute(t.Context(), actor(), "other", "Other", serviceShape(true)); err == nil {
		t.Fatal("somebody who may not shape the workspace created a workflow")
	}
}

// UC-FLW-02 checks 4 and 7: using the shipped workflow in a hub moves nothing without the
// confirmation; with it, open entries start in To do and done ones in Done.
func TestUsingAWorkflowMovesTheEntriesOnceConfirmed(t *testing.T) {
	f := newDesk()
	f.workflows.groups = []work.StatusGroup{
		{CollectionID: collection, EntryType: "task", Count: 37},
		{CollectionID: collection, EntryType: "task", Completed: true, Count: 5},
	}
	change := workflow.SchemeChange{ContainerID: hub, WorkflowKey: work.SystemWorkflowKey}
	set := workflow.SetWorkflowScheme{Desk: f.desk}
	if _, err := set.Execute(t.Context(), actor(), change, false); detail(err) != "workflows.confirmation_required" {
		t.Fatalf("nothing moves without the confirmation: %v", err)
	}
	preview, err := workflow.PreviewWorkflowScheme{Desk: f.desk}.Descriptor().Handler.Invoke(t.Context(), actor(), usecase.Input{
		"container_id": hub.String(), "workflow": work.SystemWorkflowKey,
	})
	if err != nil || preview["entries"] != 42 {
		t.Fatalf("the preview counts the entries: %v %v", preview, err)
	}
	moved, err := set.Execute(t.Context(), actor(), change, true)
	if err != nil || moved != 42 {
		t.Fatalf("moved %d: %v", moved, err)
	}
	targets := map[bool]string{}
	for _, move := range f.workflows.moves {
		targets[move.Completed] = move.To
	}
	if targets[false] != "todo" || targets[true] != "done" {
		t.Fatalf("open entries start in todo, done ones in done: %v", targets)
	}
	if len(f.workflows.rows) != 1 || !f.workflows.rows[0].System {
		t.Fatalf("the shipped workflow gains its row when first used: %+v", f.workflows.rows)
	}
}

type roleKeys []string

func (r roleKeys) List(context.Context) ([]identityrepo.CustomRoleRow, error) {
	rows := make([]identityrepo.CustomRoleRow, 0, len(r))
	for _, key := range r {
		rows = append(rows, identityrepo.CustomRoleRow{Role: identity.CustomRole{Key: key}})
	}
	return rows, nil
}

// UC-ID-26 check 4: a step limited to roles names a role of the workspace's own only where the
// workspace defines it; a key it does not define is refused at the step, not left opening to nobody.
func TestAStepNamesOnlyACustomRoleTheWorkspaceDefines(t *testing.T) {
	f := newDesk()
	f.desk.Roles = roleKeys{"service_lead"}
	limited := func(role string) work.WorkflowShape {
		shape := serviceShape(false)
		shape.Steps = []work.WorkflowStep{{Key: "finish", Name: "Done", From: []string{"doing"}, To: "done",
			Who: work.StepWho{Roles: []string{"ADMIN", role}}}}
		return shape
	}
	if _, err := (workflow.CreateWorkflow{Desk: f.desk}).Execute(t.Context(), actor(), "service", "Service", limited("nobody_here")); detail(err) != "workflows.role_unknown" {
		t.Fatalf("creating with an undefined role: %v, want workflows.role_unknown", err)
	}
	created, err := workflow.CreateWorkflow{Desk: f.desk}.Execute(t.Context(), actor(), "service", "Service", limited("service_lead"))
	if err != nil {
		t.Fatalf("creating with a defined role: %v", err)
	}
	draft := limited("gone_role")
	if _, err := (workflow.SaveWorkflowDraft{Desk: f.desk}).Execute(t.Context(), actor(), workflow.SaveWorkflowDraftCommand{
		Key: created.Key, Shape: &draft,
	}); detail(err) != "workflows.role_unknown" {
		t.Fatalf("a draft with an undefined role: %v, want workflows.role_unknown", err)
	}
}

// UC-FLW-05 check 2: the preview answers for a shape it is given as for a stored draft, so a
// configuration plan names the entries a workflow's change moves before anything is saved.
func TestPreviewingAShapeAnswersWithoutADraft(t *testing.T) {
	f := newDesk()
	if _, err := (workflow.CreateWorkflow{Desk: f.desk}).Execute(t.Context(), actor(), "service", "Service", serviceShape(true)); err != nil {
		t.Fatalf("creating: %v", err)
	}
	f.workflows.schemes = []work.WorkflowScheme{{ContainerID: hub, WorkflowKey: "service"}}
	f.workflows.groups = []work.StatusGroup{{CollectionID: collection, EntryType: "task", Status: "waiting", Count: 12}}
	shape := map[string]any(workflow.ShapeOutput(serviceShape(false)))
	preview := workflow.PreviewWorkflowPublish{Desk: f.desk}.Descriptor().Handler

	unmapped, err := preview.Invoke(t.Context(), actor(), usecase.Input{"key": "service", "shape": shape})
	removed, _ := unmapped["removed"].([]usecase.Output)
	if err != nil || len(removed) != 1 || removed[0]["status"] != "waiting" || removed[0]["count"] != 12 {
		t.Fatalf("a shape removing a status in use names it with its count: %+v: %v", unmapped, err)
	}
	out, err := preview.Invoke(t.Context(), actor(), usecase.Input{
		"key": "service", "shape": shape, "mapping": map[string]any{"waiting": "doing"},
	})
	if err != nil || out["entries"] != 12 {
		t.Fatalf("preview %+v: %v", out, err)
	}
	if f.workflows.rows[0].Draft != nil || len(f.workflows.moves) != 0 {
		t.Fatal("previewing a shape saved or moved something")
	}
}
