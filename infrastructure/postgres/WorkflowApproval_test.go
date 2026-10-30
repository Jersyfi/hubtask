// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"reflect"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-SVC-08 check 1: a step's approval is kept in the shape's column as written.
func TestAStepsApprovalIsKeptInTheShape(t *testing.T) {
	approval := &work.StepApproval{DeclineTo: "declined", DueHours: 24, Levels: []work.ApprovalLevel{
		{People: []string{"0190a4c2-7b1e-7000-8000-00000000000a"}, Rule: work.ApprovalAll},
		{Roles: []string{"ADMIN"}, Rule: work.ApprovalNOf, Count: 2},
	}}
	column, err := shapeColumn(work.WorkflowShape{Initial: "a", Steps: []work.WorkflowStep{{Key: "s", To: "b", Approval: approval}, {Key: "t", To: "a"}}})
	if err != nil {
		t.Fatal(err)
	}
	shape, err := shapeFrom(column)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(shape.Steps[0].Approval, approval) || shape.Steps[1].Approval != nil {
		t.Errorf("kept %+v and %+v", shape.Steps[0].Approval, shape.Steps[1].Approval)
	}
}
