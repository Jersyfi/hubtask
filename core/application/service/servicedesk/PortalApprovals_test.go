// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

func (s *approvalStore) Pending(_ context.Context, _ int) ([]domain.Approval, error) {
	var out []domain.Approval
	for _, a := range s.rows {
		if a.State == domain.ApprovalPending {
			out = append(out, a)
		}
	}
	return out, nil
}

// UC-SVC-09 check 5: an approver who reads nothing of the team's - a requester in the portal -
// finds what waits for their yes and decides it; being counted is what lets them, never a
// permission on the entry.
func TestARequesterCountedAtALevelDecidesFromThePortal(t *testing.T) {
	approvals, store, calls, items := approvalFixture(t)
	approvals.Desk.Authorizer = &gate{allow: false}
	step := workdomain.WorkflowStep{Key: "approve", To: "approved", Approval: &workdomain.StepApproval{
		Levels: []workdomain.ApprovalLevel{{People: []string{requesterID.String()}, Rule: workdomain.ApprovalAny}},
	}}
	if err := approvals.Open(context.Background(), person(handlerID), items.item, workdomain.Container{}, "purchase", step); err != nil {
		t.Fatal(err)
	}
	waiting, err := ListMyApprovals{Approvals: approvals}.Execute(context.Background(), person(requesterID))
	if err != nil || len(waiting) != 1 || waiting[0]["may_decide"] != true {
		t.Fatalf("the requester's list: %v %v", waiting, err)
	}
	var id string
	for key := range store.rows {
		id = key.String()
	}
	out, err := DecideApproval{Approvals: approvals}.Execute(context.Background(), person(requesterID),
		DecideApprovalCommand{ApprovalID: shared.ID(id), Verdict: domain.VerdictApprove})
	if err != nil || out["state"] != string(domain.ApprovalApproved) || len(calls.names) != 1 {
		t.Fatalf("the requester's yes: %v %v %v", out, err, calls.names)
	}
}
