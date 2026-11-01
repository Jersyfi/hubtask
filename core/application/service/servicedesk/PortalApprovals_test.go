// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// PendingFor is ListPendingApprovalsFor's selection: counted at the current level, or a level
// naming a role, oldest first after the cursor.
func (s *approvalStore) PendingFor(_ context.Context, query repository.PendingQuery) ([]domain.Approval, error) {
	s.pendingReads++
	var out []domain.Approval
	for _, a := range s.rows {
		if a.State != domain.ApprovalPending {
			continue
		}
		after := a.RequestedAt.After(query.AfterRequestedAt) ||
			(a.RequestedAt.Equal(query.AfterRequestedAt) && a.ID > query.AfterID)
		mine := len(a.Levels[a.Current].Level.Roles) > 0
		for _, person := range query.People {
			mine = mine || a.Counts(person)
		}
		if after && mine {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(x, y domain.Approval) int {
		if c := x.RequestedAt.Compare(y.RequestedAt); c != 0 {
			return c
		}
		return strings.Compare(x.ID.String(), y.ID.String())
	})
	if len(out) > query.Limit {
		out = out[:query.Limit]
	}
	return out, nil
}

func (s *approvalStore) DecisionsOf(_ context.Context, ids []shared.ID) (map[shared.ID][]domain.Decision, error) {
	s.decisionReads++
	out := map[shared.ID][]domain.Decision{}
	for _, d := range s.decisions {
		if slices.Contains(ids, d.ApprovalID) {
			out[d.ApprovalID] = append(out[d.ApprovalID], d)
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

// A service desk with 600 approvals waiting: the reader's two - the newest, counting them, and one
// of a person away they decide for - are in their list, nobody else's is, and one list reads the
// waiting approvals, the delegations and the decisions once each, not once per approval.
func TestAReadersApprovalsAreFoundPastTheWorkspacesOldest(t *testing.T) {
	approvals, store, _, items := approvalFixture(t)
	step := workdomain.WorkflowStep{Key: "approve", To: "approved", Approval: &workdomain.StepApproval{
		Levels: []workdomain.ApprovalLevel{{People: []string{handlerID.String()}, Rule: workdomain.ApprovalAny}},
	}}
	open := func(n int, counted shared.ID) domain.Approval {
		approval, err := domain.NewApproval(shared.MustParseID(fmt.Sprintf("01936f2a-7c1e-7000-8000-%012d", n)), items.item,
			"purchase", step, [][]shared.ID{{counted}}, handlerID, monday.Add(time.Duration(n)*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		store.rows[approval.ID] = approval
		return approval
	}
	away := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005fa")
	for n := 1; n <= 600; n++ {
		open(n, handlerID)
	}
	onBehalf := open(601, away)
	newest := open(602, requesterID)
	store.delegations = []domain.Delegation{{AccountID: away, DelegateID: requesterID, From: monday.Add(-time.Hour), Until: monday.Add(time.Hour)}}

	waiting, err := ListMyApprovals{Approvals: approvals}.Execute(context.Background(), person(requesterID))
	if err != nil || len(waiting) != 2 {
		t.Fatalf("the reader's list: %d rows, %v", len(waiting), err)
	}
	if waiting[0]["id"] != onBehalf.ID.String() || waiting[0]["on_behalf_of"] != away.String() ||
		waiting[1]["id"] != newest.ID.String() || waiting[1]["on_behalf_of"] != nil {
		t.Errorf("the list holds %v / %v", waiting[0]["id"], waiting[1]["id"])
	}
	if store.pendingReads != 1 || store.delegationReads != 1 || store.decisionReads != 1 {
		t.Errorf("one list read the pending approvals %d, the delegations %d and the decisions %d times",
			store.pendingReads, store.delegationReads, store.decisionReads)
	}
}

// transacted marks the context it runs in, as the database's unit of work puts its transaction
// there; inTransaction refuses an entry written outside one, as the audit sink does.
type transacted struct{}

type txMark struct{}

func (transacted) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, txMark{}, true))
}

func (transacted) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, txMark{}, true))
}

type inTransaction struct{ entries int }

func (a *inTransaction) Append(ctx context.Context, _ audit.Entry) error {
	if ctx.Value(txMark{}) == nil {
		return shared.ErrInternal.WithDetail("postgres.no_transaction_in_context")
	}
	a.entries++
	return nil
}

// The reader's list writes its read to the trail inside a transaction: the sink refuses an entry
// without one, and the list answered 500 to every approver.
func TestAReadersApprovalListIsRecordedInATransaction(t *testing.T) {
	approvals, _, _, _ := approvalFixture(t)
	sink := &inTransaction{}
	approvals.Desk.UnitOfWork, approvals.Desk.Audit = transacted{}, sink
	if _, err := (ListMyApprovals{Approvals: approvals}).Execute(context.Background(), person(requesterID)); err != nil {
		t.Fatalf("the list: %v", err)
	}
	if sink.entries != 1 {
		t.Fatalf("the read is recorded %d times", sink.entries)
	}
}
