// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// withholding refuses the permissions it names and asks the harness's authorizer for the rest.
type withholding struct {
	*authorizer
	refused []service.Permission
}

func (w withholding) Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error {
	if slices.Contains(w.refused, request.Permission) {
		return shared.ErrForbidden
	}
	return w.authorizer.Authorize(ctx, actor, request)
}

func blockedKeys(steps EntrySteps) []string {
	var keys []string
	for _, verdict := range steps.Blocked {
		keys = append(keys, verdict.Step.Key)
	}
	return keys
}

// UC-FLW-06 checks 1, 2 and 3: beside the offered steps the entry names the steps the person may
// not take with the reason and who may, the fewest steps into done, and the workflow's picture.
func TestTheEntryNamesTheStepsThePersonMayNotTakeAndWhoMay(t *testing.T) {
	h, _ := workflowHarness(t)
	assignee := shared.MustParseID("0192f000-0000-7000-8000-0000000007a1")
	task := h.items.stored[taskID]
	task.AssigneeID = assignee
	h.items.stored[taskID] = task
	workflow := reviewWorkflow()
	workflow.Published.Steps[2].Who = domain.StepWho{PersonFields: []string{"assignee"}}
	workflow.Published.Steps = append(workflow.Published.Steps, domain.WorkflowStep{
		Key: "escalate", From: []string{"review"}, To: "todo", Who: domain.StepWho{Condition: "entry.urgent"},
		Approval: &domain.StepApproval{Levels: []domain.ApprovalLevel{{Rule: domain.ApprovalAny}}},
	})
	h.writer.Workflow.Workflows = &workflowStore{
		rows:    []domain.Workflow{workflow},
		schemes: []domain.WorkflowScheme{{ContainerID: hubID, WorkflowKey: "review"}},
	}
	steps, err := ListEntrySteps{Completion: h.writer}.Execute(t.Context(), actorFixture(), taskID)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if keys := blockedKeys(steps); !slices.Equal(keys, []string{"back", "reject", "escalate"}) {
		t.Fatalf("blocked %v, want back, reject, escalate", keys)
	}
	back, reject, escalate := steps.Blocked[0], steps.Blocked[1], steps.Blocked[2]
	if back.Reason != StepReasonWhoUnmet || !slices.Equal(back.People, []shared.ID{assignee}) {
		t.Fatalf("the assignee's step names its person: %+v", back)
	}
	if reject.Reason != StepReasonWhoUnmet || !slices.Equal(reject.Step.Who.Roles, []string{"OWNER"}) {
		t.Fatalf("the owner's step names its role: %+v", reject)
	}
	if escalate.Reason != StepReasonConditionUnmet {
		t.Fatalf("a condition that fails is its reason: %+v", escalate)
	}
	if len(steps.DoneRoute) != 1 || steps.DoneRoute[0].Key != "accept" {
		t.Fatalf("done is one step away: %+v", steps.DoneRoute)
	}
	if steps.ByDefault || len(steps.Picture.Statuses) != 4 || len(steps.Picture.Steps) != 6 {
		t.Fatalf("the picture is the workflow's: %+v", steps.Picture)
	}
	out, err := ListEntrySteps{Completion: h.writer}.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{"item_id": taskID.String()})
	if err != nil {
		t.Fatalf("handling: %v", err)
	}
	if blocked := out["blocked"].([]usecase.Output); blocked[0]["reason"] != "workflows.step_who_unmet" ||
		blocked[1]["permission"] != "TAKE_STEP:review/reject" {
		t.Fatalf("the door spells the blocked steps: %v", blocked)
	}
	pictured := out["picture"].(usecase.Output)["steps"].([]usecase.Output)
	if pictured[3]["limited"] != true || pictured[5]["approval"] != true || pictured[0]["limited"] != false {
		t.Fatalf("the picture marks limited and approval steps: %v", pictured)
	}
}

// UC-FLW-06 check 4: a reader sees the picture and no step; the designer opens only to whoever
// may change the workspace's structure.
func TestAReaderSeesThePictureAndOnlyTheShaperTheDesigner(t *testing.T) {
	h, _ := workflowHarness(t)
	base := h.writer.Authorizer.(*authorizer)
	h.writer.Authorizer = withholding{authorizer: base, refused: []service.Permission{service.PermissionWriteItems, service.PermissionStructure}}
	steps, err := ListEntrySteps{Completion: h.writer}.Execute(t.Context(), actorFixture(), taskID)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(steps.Offered) != 0 || len(steps.Blocked) != 0 || steps.MayDesign {
		t.Fatalf("a reader gets no step and no designer: %+v", steps)
	}
	if len(steps.Picture.Statuses) != 4 {
		t.Fatalf("a reader sees the picture: %+v", steps.Picture)
	}
	h.writer.Authorizer = withholding{authorizer: base}
	h.writer.Workflow.Roles = roleOf{role: identity.RoleAdmin}
	steps, err = ListEntrySteps{Completion: h.writer}.Execute(t.Context(), actorFixture(), taskID)
	if err != nil || !steps.MayDesign {
		t.Fatalf("whoever shapes the workspace opens the designer: %+v, %v", steps.MayDesign, err)
	}
}

// UC-FLW-06 check 5: where no workflow runs the default holds - to do, in progress, done, every
// status reachable - and completing reaches done.
func TestWithoutAWorkflowTheDefaultHolds(t *testing.T) {
	h := newCompletionHarness(domain.CompletionManual)
	steps, err := ListEntrySteps{Completion: h.writer}.Execute(t.Context(), actorFixture(), taskID)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if steps.InForce || !steps.ByDefault || len(steps.Blocked) != 0 || len(steps.DoneRoute) != 0 {
		t.Fatalf("no workflow runs: %+v", steps)
	}
	var statuses []string
	for _, status := range steps.Picture.Statuses {
		statuses = append(statuses, status.Key)
	}
	if !slices.Equal(statuses, []string{"todo", "in_progress", "done"}) {
		t.Fatalf("the default picture: %v", statuses)
	}
	for _, step := range steps.Picture.Steps {
		if len(step.From) != 0 || !step.Who.IsOpen() {
			t.Fatalf("every status is reachable by everybody: %+v", step)
		}
	}
}
