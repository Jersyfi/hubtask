// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

type approvalStore struct {
	repository.Approvals
	rows        map[shared.ID]domain.Approval
	decisions   []domain.Decision
	delegations []domain.Delegation
	// Reads of the list's queries, to bound what one list costs.
	pendingReads, decisionReads, delegationReads int
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
func (s *approvalStore) OfItem(_ context.Context, item shared.ID) ([]domain.Approval, error) {
	var out []domain.Approval
	for _, a := range s.rows {
		if a.ItemID == item {
			out = append(out, a)
		}
	}
	return out, nil
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
	s.delegationReads++
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

// UC-SVC-09 check 3: an entry's readers read its approvals with the levels, the people counted,
// where it stands and the trail - who decided, for whom, when, why, on which version; somebody
// who may not read the entry is told it does not exist.
func TestAnEntrysReadersReadItsApprovalsAndTrail(t *testing.T) {
	approvals, store, _, items := approvalFixture(t)
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
	if _, err := (DecideApproval{Approvals: approvals}).Execute(context.Background(), person(deputy),
		DecideApprovalCommand{ApprovalID: id, Verdict: domain.VerdictDecline, Reason: "Over budget."}); err != nil {
		t.Fatal(err)
	}

	read := ReadEntryApprovals{Approvals: approvals}
	rows, err := read.Execute(context.Background(), person(requesterID), items.item.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("read %v, %v", rows, err)
	}
	gate := approvals.Desk.Authorizer.(*gate)
	if asked := gate.asked[len(gate.asked)-1]; asked.Permission != service.PermissionRead || asked.TargetID != items.item.ID {
		t.Errorf("reading asked %+v, want READ on the entry", asked)
	}
	row := rows[0]
	if row["state"] != string(domain.ApprovalDeclined) || row["item_version"] != items.item.Version {
		t.Errorf("where it stands: %v, version %v", row["state"], row["item_version"])
	}
	levels, _ := row["levels"].([]usecase.Output)
	if len(levels) != 1 || !slices.Equal(levels[0]["approvers"].([]string), []string{handlerID.String()}) {
		t.Errorf("the levels and the people counted: %v", levels)
	}
	trail, _ := row["decisions"].([]usecase.Output)
	if len(trail) != 1 {
		t.Fatalf("the trail: %v", trail)
	}
	d := trail[0]
	if d["approver_id"] != deputy.String() || d["on_behalf_of"] != handlerID.String() || d["reason"] != "Over budget." ||
		d["verdict"] != string(domain.VerdictDecline) || d["item_version"] != items.item.Version || d["decided_at"] != monday.UTC() {
		t.Errorf("the trail names %+v", d)
	}

	gate.allow = false
	if _, err := read.Execute(context.Background(), person(requesterID), items.item.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("somebody who may not read the entry read its approvals: %v", err)
	}
}

// undoing is a unit of work that rolls the approvals back when its work fails, as a transaction
// does.
type undoing struct {
	inline
	store *approvalStore
}

func (u undoing) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	rows, decisions := maps.Clone(u.store.rows), slices.Clone(u.store.decisions)
	if err := fn(ctx); err != nil {
		u.store.rows, u.store.decisions = rows, decisions
		return err
	}
	return nil
}

type refusedStep struct{ err error }

func (r refusedStep) Invoke(context.Context, string, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
	return nil, r.err
}

// UC-SVC-09 check 2: a yes whose step cannot be taken - the entry's form now misses an answer -
// is refused whole: the approval still waits, so the approver can answer again once it can move,
// and it is never approved with its entry left where it stood.
func TestAYesWhoseStepIsRefusedLeavesTheApprovalWaiting(t *testing.T) {
	approvals, store, _, items := approvalFixture(t)
	approvals.Desk.UnitOfWork = undoing{store: store}
	refusal := shared.ErrValidation.WithDetail("forms.required_missing")
	approvals.Steps = refusedStep{err: refusal}
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
	decide := DecideApproval{Approvals: approvals}
	if _, err := decide.Execute(context.Background(), person(handlerID), DecideApprovalCommand{ApprovalID: opened.ID, Verdict: domain.VerdictApprove}); !errors.Is(err, refusal) {
		t.Fatalf("the yes answered %v", err)
	}
	if got := store.rows[opened.ID]; got.State != domain.ApprovalPending || len(store.decisions) != 0 {
		t.Errorf("after a refused step the approval stands %s with %d decisions", got.State, len(store.decisions))
	}
	approvals.Steps = &stepCalls{}
	decide = DecideApproval{Approvals: approvals}
	if out, err := decide.Execute(context.Background(), person(handlerID), DecideApprovalCommand{ApprovalID: opened.ID, Verdict: domain.VerdictApprove}); err != nil || out["state"] != string(domain.ApprovalApproved) {
		t.Errorf("the yes again: %v %v", out, err)
	}
}
