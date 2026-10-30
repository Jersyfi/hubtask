// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package workflow

import (
	"encoding/json"
	"fmt"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-APR-01 check 1: a step's approval is read from a door and answered as written.
func TestAnApprovalStepIsReadAsItIsAnswered(t *testing.T) {
	approval := &domain.StepApproval{DeclineTo: "declined", DueHours: 48, Levels: []domain.ApprovalLevel{
		{People: []string{"0190a4c2-7b1e-7000-8000-00000000000a"}, Groups: []string{}, Roles: []string{}, PersonFields: []string{"custom_fields.manager"}, Rule: domain.ApprovalAny},
		{People: []string{}, Groups: []string{}, Roles: []string{"cab_member"}, PersonFields: []string{}, Rule: domain.ApprovalNOf, Count: 2},
	}}
	shape := domain.WorkflowShape{Initial: "asked", Steps: []domain.WorkflowStep{{Key: "approve", To: "approved", Approval: approval}}}
	encoded, err := json.Marshal(ShapeOutput(shape))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	read, err := ShapeFromInput(document)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%+v", read.Steps[0].Approval) != fmt.Sprintf("%+v", approval) {
		t.Errorf("read back %+v", read.Steps[0].Approval)
	}
	if ShapeOutput(domain.WorkflowShape{Steps: []domain.WorkflowStep{{Key: "go", To: "x"}}})["steps"].([]any)[0].(map[string]any)["approval"] != nil {
		t.Errorf("a step without an approval answers one")
	}
}
