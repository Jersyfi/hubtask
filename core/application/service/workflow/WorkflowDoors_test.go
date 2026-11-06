// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package workflow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var reviewers = shared.MustParseID("018f0000-0000-7000-8000-0000000000c1")

type groups map[shared.ID]bool

func (g groups) Find(_ context.Context, id shared.ID) (identity.Group, error) {
	if !g[id] {
		return identity.Group{}, shared.ErrNotFound
	}
	return identity.Group{ID: id}, nil
}

func workflowDoors(t *testing.T, desk workflow.Desk) *usecase.Registry {
	t.Helper()
	registry, err := usecase.NewRegistry(nil,
		workflow.ListWorkflows{Desk: desk}.Descriptor(),
		workflow.CreateWorkflow{Desk: desk}.Descriptor(),
		workflow.SaveWorkflowDraft{Desk: desk}.Descriptor(),
		workflow.PreviewWorkflowPublish{Desk: desk}.Descriptor(),
		workflow.PublishWorkflow{Desk: desk}.Descriptor(),
		workflow.DeleteWorkflow{Desk: desk}.Descriptor(),
		workflow.SetWorkflowScheme{Desk: desk}.Descriptor(),
		workflow.TryWorkflowStep{Desk: desk}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return registry
}

// The shape as a door hands it over: JSON numbers arrive as float64.
func doorShape(group string) map[string]any {
	return map[string]any{
		"initial": "todo",
		"statuses": []any{
			map[string]any{"key": "todo", "name": "To do", "category": "TODO"},
			map[string]any{"key": "review", "name": "In review", "category": "IN_PROGRESS"},
			map[string]any{"key": "done", "name": "Done", "category": "DONE"},
		},
		"steps": []any{
			map[string]any{
				"key": "submit", "name": "Submit", "to": "review", "from": []any{"todo"},
				"who": map[string]any{"groups": []any{group}},
				"checks": []any{map[string]any{
					"key": "estimated", "holds": "true", "messages": map[string]any{"en": "Estimate first", "de": "Erst schätzen"},
				}},
			},
			map[string]any{
				"key": "approve", "name": "Approve", "to": "done", "from": []any{"review"},
				"approval": map[string]any{
					"decline_to": "todo", "due_hours": float64(48),
					"levels": []any{map[string]any{"groups": []any{group}, "rule": "ANY", "count": float64(1)}},
				},
			},
		},
	}
}

// UC-FLW-02 checks 1 and 3 and UC-FLW-04 checks 1-2 through the doors: a workflow created,
// drafted, previewed, published and deleted by the descriptors REST and MCP call, with the
// shape read from JSON - check words per language and an approval's numbers included.
func TestAWorkflowRunsItsWholeLifeThroughTheDoors(t *testing.T) {
	f := newDesk()
	f.desk.Groups = groups{reviewers: true}
	doors := workflowDoors(t, f.desk)
	ctx, who := t.Context(), actor()

	created, err := doors.Invoke(ctx, workflow.CreateWorkflowName, who, usecase.Input{
		"key": "review", "name": "Review", "shape": doorShape(reviewers.String()),
	})
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	published := created["published"].(usecase.Output)
	steps := published["steps"].([]any)
	if len(steps) != 2 {
		t.Fatalf("the steps came back as %v", steps)
	}
	check := steps[0].(map[string]any)["checks"].([]any)[0].(map[string]any)
	if messages := check["messages"].(map[string]any); messages["de"] != "Erst schätzen" || messages["en"] != "Estimate first" {
		t.Errorf("a check keeps its words per language: %v", messages)
	}
	approval := steps[1].(map[string]any)["approval"].(map[string]any)
	if approval["due_hours"] != 48 || approval["decline_to"] != "todo" {
		t.Errorf("the approval's numbers survived the door: %v", approval)
	}

	listed, err := doors.Invoke(ctx, workflow.ListWorkflowsName, who, usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if data := listed["data"].([]usecase.Output); len(data) == 0 {
		t.Fatal("the created workflow is not listed")
	}

	draftShape := doorShape(reviewers.String())
	draftShape["statuses"] = append(draftShape["statuses"].([]any)[:1:1],
		draftShape["statuses"].([]any)[2], map[string]any{"key": "checking", "name": "Checking", "category": "IN_PROGRESS"})
	draftShape["steps"] = []any{map[string]any{"key": "check", "name": "Check", "to": "checking", "from": []any{"todo"}}}
	drafted, err := doors.Invoke(ctx, workflow.SaveWorkflowDraftName, who, usecase.Input{
		"key": "review", "name": "Careful review", "shape": draftShape,
	})
	if err != nil || drafted["draft"] == nil {
		t.Fatalf("drafting: %v %v", drafted, err)
	}
	if _, err := doors.Invoke(ctx, workflow.PreviewWorkflowPublishName, who, usecase.Input{"key": "review"}); err != nil {
		t.Fatalf("previewing the draft: %v", err)
	}
	if _, err := doors.Invoke(ctx, workflow.PublishWorkflowName, who, usecase.Input{"key": "review"}); err != nil {
		t.Fatalf("publishing: %v", err)
	}
	if f.workflows.rows[0].Draft != nil {
		t.Error("the published draft is still a draft")
	}
	if _, err := doors.Invoke(ctx, workflow.DeleteWorkflowName, who, usecase.Input{"key": "review"}); err != nil {
		t.Fatalf("deleting a workflow nothing uses: %v", err)
	}

	actions := map[string]bool{}
	for _, entry := range f.trail.entries {
		actions[string(entry.Action)] = true
	}
	for _, want := range []string{"workflow.created", "workflow.drafted", "workflow.published", "workflow.deleted"} {
		if !actions[want] {
			t.Errorf("the trail misses %s: %v", want, actions)
		}
	}
}

// Refusal instead of corruption: a shape that is not one, a step naming a group the workspace
// does not hold (UC-FLW-04 check 1), and an unknown workflow are refused before anything is
// written.
func TestTheDoorsRefuseAMalformedOrUnknownShape(t *testing.T) {
	f := newDesk()
	f.desk.Groups = groups{reviewers: true}
	doors := workflowDoors(t, f.desk)
	ctx, who := t.Context(), actor()

	cases := []struct {
		name  string
		input usecase.Input
		code  string
	}{
		{"a shape that is no object", usecase.Input{"key": "a", "name": "A", "shape": "statuses"}, "usecase.input_invalid"},
		{"a status that is no object", usecase.Input{"key": "a", "name": "A", "shape": map[string]any{"statuses": []any{"todo"}}}, "workflows.malformed"},
		{"a step that is no object", usecase.Input{"key": "a", "name": "A", "shape": map[string]any{"steps": []any{1.0}}}, "workflows.malformed"},
		{"a check that is no object", usecase.Input{"key": "a", "name": "A", "shape": map[string]any{
			"steps": []any{map[string]any{"key": "s", "checks": []any{"x"}}},
		}}, "workflows.malformed"},
		{"a group the workspace does not hold", usecase.Input{"key": "a", "name": "A",
			"shape": doorShape("018f0000-0000-7000-8000-0000000000ff")}, "workflows.group_unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := doors.Invoke(ctx, workflow.CreateWorkflowName, who, c.input)
			if !errors.Is(err, shared.ErrValidation) || detail(err) != c.code {
				t.Fatalf("got %v, want %s", err, c.code)
			}
		})
	}
	if len(f.workflows.rows) != 0 || len(f.trail.entries) != 0 {
		t.Fatalf("a refused shape left %d rows and %d trail entries", len(f.workflows.rows), len(f.trail.entries))
	}

	if _, err := doors.Invoke(ctx, workflow.PublishWorkflowName, who, usecase.Input{"key": "nowhere"}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("publishing an unknown workflow: %v", err)
	}
	if _, err := doors.Invoke(ctx, workflow.TryWorkflowStepName, who, usecase.Input{
		"key": "nowhere", "step": "s", "item_id": entry.String(),
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("trying a step of an unknown workflow: %v", err)
	}

	// Without the group finder wired, a shape naming a group is an internal fault, never a
	// silently accepted step that opens to nobody.
	f.desk.Groups = nil
	bare := workflowDoors(t, f.desk)
	if _, err := bare.Invoke(ctx, workflow.CreateWorkflowName, who, usecase.Input{
		"key": "a", "name": "A", "shape": doorShape(reviewers.String()),
	}); !errors.Is(err, shared.ErrInternal) {
		t.Errorf("a missing group finder: %v", err)
	}
}

// Rule 2: whoever may not shape the workspace is refused by every writing door, and nothing is
// written.
func TestEveryWorkflowWriteIsRefusedWithoutThePermission(t *testing.T) {
	f := newDesk()
	f.desk.Groups = groups{reviewers: true}
	doors := workflowDoors(t, f.desk)
	ctx, who := t.Context(), actor()
	if _, err := doors.Invoke(ctx, workflow.CreateWorkflowName, who, usecase.Input{
		"key": "review", "name": "Review", "shape": doorShape(reviewers.String()),
	}); err != nil {
		t.Fatalf("creating: %v", err)
	}
	before := len(f.trail.entries)
	f.auth.refuse = true

	for name, input := range map[string]usecase.Input{
		workflow.CreateWorkflowName:         {"key": "other", "name": "Other", "shape": doorShape(reviewers.String())},
		workflow.SaveWorkflowDraftName:      {"key": "review", "discard": true},
		workflow.PreviewWorkflowPublishName: {"key": "review"},
		workflow.PublishWorkflowName:        {"key": "review"},
		workflow.DeleteWorkflowName:         {"key": "review"},
		workflow.SetWorkflowSchemeName:      {"container_id": hub.String(), "workflow": "review", "confirm": true},
		workflow.TryWorkflowStepName:        {"key": "review", "step": "submit", "item_id": entry.String()},
	} {
		if _, err := doors.Invoke(ctx, name, who, input); !errors.Is(err, shared.ErrForbidden) {
			t.Errorf("%s without the permission: %v", name, err)
		}
	}
	if len(f.workflows.rows) != 1 || len(f.trail.entries) != before {
		t.Errorf("a refused write changed something: %d rows, %d trail entries", len(f.workflows.rows), len(f.trail.entries))
	}
}
