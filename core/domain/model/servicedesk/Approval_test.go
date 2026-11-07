// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	approvedAt = time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	ana        = shared.ID("0190a4c2-7b1e-7000-8000-00000000000a")
	ben        = shared.ID("0190a4c2-7b1e-7000-8000-00000000000b")
	cem        = shared.ID("0190a4c2-7b1e-7000-8000-00000000000c")
	dora       = shared.ID("0190a4c2-7b1e-7000-8000-00000000000d")
)

func approvalOf(t *testing.T, levels []work.ApprovalLevel, counted [][]shared.ID) servicedesk.Approval {
	t.Helper()
	step := work.WorkflowStep{Key: "approve", To: "approved", Approval: &work.StepApproval{Levels: levels, DeclineTo: "declined", DueHours: 24}}
	a, err := servicedesk.NewApproval("0190a4c2-7b1e-7000-8000-0000000000f1", work.WorkItem{ID: "0190a4c2-7b1e-7000-8000-0000000000e1", TenantID: "0190a4c2-7b1e-7000-8000-0000000000a1", Version: 4},
		"purchase", step, counted, dora, approvedAt)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func decide(t *testing.T, a servicedesk.Approval, decided []servicedesk.Decision, who shared.ID, verdict servicedesk.Verdict) (servicedesk.Approval, []servicedesk.Decision) {
	t.Helper()
	next, decision, err := servicedesk.Decide(a, decided, shared.ID("0190a4c2-7b1e-7000-8000-0000000000d"+string(rune('0'+len(decided)))),
		who, servicedesk.Eligibility{Direct: true}, verdict, "because", a.ItemVersion, approvedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("%s deciding: %v", who, err)
	}
	return next, append(decided, decision)
}

// UC-SVC-08 check 2: any, all, N of M and a majority pass a level by their own count.
func TestALevelPassesByItsRule(t *testing.T) {
	cases := []struct {
		rule  work.ApprovalRule
		count int
		yes   int
	}{
		{work.ApprovalAny, 0, 1}, {work.ApprovalAll, 0, 3}, {work.ApprovalNOf, 2, 2}, {work.ApprovalMajority, 0, 2},
	}
	for _, c := range cases {
		a := approvalOf(t, []work.ApprovalLevel{{Rule: c.rule, Count: c.count}}, [][]shared.ID{{ana, ben, cem}})
		var decided []servicedesk.Decision
		for i, who := range []shared.ID{ana, ben, cem}[:c.yes] {
			if a.State != servicedesk.ApprovalPending {
				t.Fatalf("%s was decided after %d yeses", c.rule, i)
			}
			a, decided = decide(t, a, decided, who, servicedesk.VerdictApprove)
		}
		if a.State != servicedesk.ApprovalApproved {
			t.Errorf("%s after %d yeses is %s", c.rule, c.yes, a.State)
		}
	}
}

// UC-SVC-08 check 3: levels decide one after another; one decline declines the approval.
func TestLevelsDecideInTurnAndOneDeclineDeclines(t *testing.T) {
	a := approvalOf(t, []work.ApprovalLevel{{Rule: work.ApprovalAny}, {Rule: work.ApprovalAll}}, [][]shared.ID{{ana}, {ben, cem}})
	a, decided := decide(t, a, nil, ana, servicedesk.VerdictApprove)
	if a.Current != 1 || a.State != servicedesk.ApprovalPending || !a.Counts(ben) || a.Counts(ana) {
		t.Fatalf("after the first level the approval is %+v", a)
	}
	a, _ = decide(t, a, decided, ben, servicedesk.VerdictDecline)
	if a.State != servicedesk.ApprovalDeclined || a.DecidedAt == nil {
		t.Errorf("a decline left the approval %s", a.State)
	}
}

// UC-SVC-09 checks 1 and 2: a decline needs a reason, nobody decides twice, and nobody decides
// who is not an approver.
func TestADecisionIsHeldToItsRules(t *testing.T) {
	a := approvalOf(t, []work.ApprovalLevel{{Rule: work.ApprovalAll}}, [][]shared.ID{{ana, ben}})
	_, _, err := servicedesk.Decide(a, nil, "0190a4c2-7b1e-7000-8000-0000000000d9", ana, servicedesk.Eligibility{Direct: true},
		servicedesk.VerdictDecline, " ", 4, approvedAt)
	if code(err) != "approvals.reason_required" {
		t.Errorf("a decline without a reason: %v", err)
	}
	a, decided := decide(t, a, nil, ana, servicedesk.VerdictApprove)
	if _, _, err := servicedesk.Decide(a, decided, "0190a4c2-7b1e-7000-8000-0000000000d8", ana, servicedesk.Eligibility{Direct: true},
		servicedesk.VerdictApprove, "", 4, approvedAt); code(err) != "approvals.already_decided" {
		t.Errorf("a second answer: %v", err)
	}
	if _, _, err := servicedesk.Decide(a, decided, "0190a4c2-7b1e-7000-8000-0000000000d7", cem, servicedesk.Eligibility{},
		servicedesk.VerdictApprove, "", 4, approvedAt); code(err) != "approvals.not_an_approver" {
		t.Errorf("somebody else's answer: %v", err)
	}
}

// UC-SVC-10 checks 1 and 2: a delegate decides for the person away, once for both of them.
func TestADelegateDecidesForThePersonAway(t *testing.T) {
	a := approvalOf(t, []work.ApprovalLevel{{Rule: work.ApprovalAll}}, [][]shared.ID{{ana, ben}})
	next, decision, err := servicedesk.Decide(a, nil, "0190a4c2-7b1e-7000-8000-0000000000d1", cem, servicedesk.Eligibility{OnBehalfOf: ana},
		servicedesk.VerdictApprove, "", 4, approvedAt)
	if err != nil || decision.OnBehalfOf != ana || decision.TenantID != "0190a4c2-7b1e-7000-8000-0000000000a1" || next.State != servicedesk.ApprovalPending {
		t.Fatalf("the delegate's answer: %+v %v", decision, err)
	}
	if _, _, err := servicedesk.Decide(next, []servicedesk.Decision{decision}, "0190a4c2-7b1e-7000-8000-0000000000d2", ana,
		servicedesk.Eligibility{Direct: true}, servicedesk.VerdictApprove, "", 4, approvedAt); code(err) != "approvals.already_decided" {
		t.Errorf("the person away answered again: %v", err)
	}
	if _, err := servicedesk.NewDelegation("t", ana, ana, approvedAt, approvedAt.Add(time.Hour)); code(err) != "approvals.delegate_self" {
		t.Errorf("a delegation to oneself: %v", err)
	}
	d, err := servicedesk.NewDelegation("t", ana, cem, approvedAt, approvedAt.Add(48*time.Hour))
	if err != nil || !d.Covers(approvedAt.Add(time.Hour)) || d.Covers(approvedAt.Add(49*time.Hour)) {
		t.Errorf("the delegation %+v %v", d, err)
	}
}

// UC-SVC-08 check 1: a level counting all or a majority opens only with people to count.
func TestACountedLevelNeedsPeople(t *testing.T) {
	step := work.WorkflowStep{Key: "approve", To: "approved", Approval: &work.StepApproval{Levels: []work.ApprovalLevel{{Rule: work.ApprovalAll}}}}
	if _, err := servicedesk.NewApproval("x", work.WorkItem{}, "w", step, [][]shared.ID{{}}, dora, approvedAt); code(err) != "approvals.too_few_approvers" {
		t.Errorf("an empty level opened: %v", err)
	}
}

func code(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		if len(e.Fields) > 0 {
			return e.Fields[0].Code
		}
		return e.DetailCode
	}
	return ""
}
