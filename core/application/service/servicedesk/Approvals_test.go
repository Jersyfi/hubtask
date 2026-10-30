// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

type approvalStore struct {
	repository.Approvals
	rows        map[shared.ID]domain.Approval
	decisions   []domain.Decision
	delegations []domain.Delegation
}

func (s *approvalStore) Insert(_ context.Context, a domain.Approval) error {
	s.rows[a.ID] = a
	return nil
}
func (s *approvalStore) Update(_ context.Context, a domain.Approval, expected int) error {
	if s.rows[a.ID].Version != expected {
		return shared.ErrConflict
	}
	s.rows[a.ID] = a
	return nil
}
func (s *approvalStore) Find(_ context.Context, id shared.ID) (domain.Approval, error) {
	a, ok := s.rows[id]
	if !ok {
		return domain.Approval{}, shared.ErrNotFound
	}
	return a, nil
}
func (s *approvalStore) PendingOf(_ context.Context, item shared.ID) (domain.Approval, error) {
	for _, a := range s.rows {
		if a.ItemID == item && a.State == domain.ApprovalPending {
			return a, nil
		}
	}
	return domain.Approval{}, shared.ErrNotFound
}
func (s *approvalStore) Record(_ context.Context, d domain.Decision) error {
	s.decisions = append(s.decisions, d)
	return nil
}
func (s *approvalStore) Decisions(_ context.Context, id shared.ID) ([]domain.Decision, error) {
	var out []domain.Decision
	for _, d := range s.decisions {
		if d.ApprovalID == id {
			out = append(out, d)
		}
	}
	return out, nil
}
func (s *approvalStore) DelegationsTo(_ context.Context, delegate shared.ID) ([]domain.Delegation, error) {
	var out []domain.Delegation
	for _, d := range s.delegations {
		if d.DelegateID == delegate {
			out = append(out, d)
		}
	}
	return out, nil
}

type stepCalls struct {
	names  []string
	actors []appshared.ActorContext
	inputs []usecase.Input
	marked []bool
}

func (c *stepCalls) Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	c.names, c.actors, c.inputs = append(c.names, name), append(c.actors, actor), append(c.inputs, in)
	c.marked = append(c.marked, work.TakenByApproval(ctx))
	return usecase.Output{}, nil
}

func approvalFixture(t *testing.T) (Approvals, *approvalStore, *stepCalls, *itemStore) {
	t.Helper()
	f := newFixture(t)
	store := &approvalStore{rows: map[shared.ID]domain.Approval{}}
	calls := &stepCalls{}
	return Approvals{
		Store: store, Items: f.items, Containers: containerStore{}, Steps: calls,
		Desk: Desk{Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday), IDs: f.ids},
	}, store, calls, f.items
}

func person(id shared.ID) appshared.ActorContext {
	return appshared.ActorContext{TenantID: tenantID, AccountID: id, Kind: appshared.ActorUser, Scopes: []string{itemsWriteScope}}
}

// UC-SVC-08 checks 1 and 3, UC-SVC-09 checks 1 and 2: the step opens an approval counting the
// person its field names; somebody else is told nothing; the last yes takes the step as the
// person who asked, marked as the approval's.
func TestTheLastYesTakesTheStep(t *testing.T) {
	approvals, store, calls, items := approvalFixture(t)
	items.item.CustomFields["manager"] = handlerID.String()
	step := workdomain.WorkflowStep{Key: "approve", To: "approved", Approval: &workdomain.StepApproval{
		Levels: []workdomain.ApprovalLevel{{PersonFields: []string{"custom_fields.manager"}, Rule: workdomain.ApprovalAll}},
	}}
	if err := approvals.Open(context.Background(), person(requesterID), items.item, workdomain.Container{}, "purchase", step); err != nil {
		t.Fatal(err)
	}
	var opened domain.Approval
	for _, a := range store.rows {
		opened = a
	}
	if !opened.Counts(handlerID) || opened.RequestedBy != requesterID {
		t.Fatalf("the approval opened as %+v", opened)
	}
	decide := DecideApproval{Approvals: approvals}
	if _, err := decide.Execute(context.Background(), person(requesterID), DecideApprovalCommand{ApprovalID: opened.ID, Verdict: domain.VerdictApprove}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("somebody who is no approver decided: %v", err)
	}
	out, err := decide.Execute(context.Background(), person(handlerID), DecideApprovalCommand{ApprovalID: opened.ID, Verdict: domain.VerdictApprove})
	if err != nil || out["state"] != string(domain.ApprovalApproved) {
		t.Fatalf("the yes: %v %v", out, err)
	}
	if len(calls.names) != 1 || calls.names[0] != takeStepUseCase || calls.inputs[0]["step"] != "approve" ||
		calls.actors[0].AccountID != requesterID || calls.actors[0].Kind != appshared.ActorAutomation || !calls.marked[0] {
		t.Errorf("the step was taken as %+v %+v %v", calls.actors, calls.inputs, calls.marked)
	}
}

// UC-SVC-10 check 2: the delegate decides for the person away, and the trail names both.
func TestTheDelegateDecidesWhileThePersonIsAway(t *testing.T) {
	approvals, store, calls, items := approvalFixture(t)
	step := workdomain.WorkflowStep{Key: "approve", To: "approved", Approval: &workdomain.StepApproval{
		DeclineTo: "declined", Levels: []workdomain.ApprovalLevel{{People: []string{handlerID.String()}, Rule: workdomain.ApprovalAny}},
	}}
	if err := approvals.Open(context.Background(), person(requesterID), items.item, workdomain.Container{}, "purchase", step); err != nil {
		t.Fatal(err)
	}
	deputy := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005f9")
	store.delegations = []domain.Delegation{{AccountID: handlerID, DelegateID: deputy, From: monday.Add(-time.Hour), Until: monday.Add(time.Hour)}}
	var id shared.ID
	for key := range store.rows {
		id = key
	}
	out, err := DecideApproval{Approvals: approvals}.Execute(context.Background(), person(deputy),
		DecideApprovalCommand{ApprovalID: id, Verdict: domain.VerdictDecline, Reason: "Over budget."})
	if err != nil || out["state"] != string(domain.ApprovalDeclined) {
		t.Fatalf("the delegate's decline: %v %v", out, err)
	}
	if len(store.decisions) != 1 || store.decisions[0].OnBehalfOf != handlerID || store.decisions[0].ApproverID != deputy {
		t.Errorf("the trail %+v", store.decisions)
	}
	if len(calls.inputs) != 1 || calls.inputs[0]["to_status"] != "declined" {
		t.Errorf("the decline moved %+v", calls.inputs)
	}
}
