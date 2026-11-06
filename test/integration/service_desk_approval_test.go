// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// An approval, its decisions and a delegation are read as written, one approval waits on an
// entry at a time, and the workspace next door reads, decides and delegates none of them
// (migration 0275, UC-SVC-08 check 4, UC-SVC-10).
func TestAnApprovalStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	approvals := postgres.NewServiceDeskRepository().Approvals()
	now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	item := seedWorkItemForRules(ctx, t, tenantA)
	ana, ben := freshID(t), freshID(t)
	step := work.WorkflowStep{Key: "approve", To: "approved", Approval: &work.StepApproval{
		DeclineTo: "declined", DueHours: 24, Levels: []work.ApprovalLevel{{People: []string{ana.String()}, Rule: work.ApprovalAll}},
	}}
	opened, err := servicedesk.NewApproval(freshID(t), work.WorkItem{ID: item, TenantID: tenantA, Version: 3}, "purchase", step,
		[][]shared.ID{{ana}}, ben, now)
	if err != nil {
		t.Fatal(err)
	}
	decided, decision, err := servicedesk.Decide(opened, nil, freshID(t), ana, servicedesk.Eligibility{Direct: true},
		servicedesk.VerdictApprove, "", 3, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	delegation, _ := servicedesk.NewDelegation(tenantA, ana, ben, now, now.Add(72*time.Hour))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return approvals.Insert(ctx, opened)
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	// The refused insert aborts its transaction, so it is the transaction's answer.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		second := opened
		second.ID = freshID(t)
		return approvals.Insert(ctx, second)
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second approval waits on the entry: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := approvals.Record(ctx, decision); err != nil {
			return err
		}
		if err := approvals.Update(ctx, decided, opened.Version); err != nil {
			return err
		}
		return approvals.Delegate(ctx, delegation)
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := approvals.Find(ctx, opened.ID)
		if err != nil || found.State != servicedesk.ApprovalApproved || len(found.Levels[0].Counted) != 1 || found.Levels[0].Level.Rule != work.ApprovalAll {
			t.Errorf("read back %+v %v", found, err)
		}
		if _, err := approvals.PendingOf(ctx, item); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("a decided approval still waits: %v", err)
		}
		trail, err := approvals.Decisions(ctx, opened.ID)
		if err != nil || len(trail) != 1 || trail[0].ApproverID != ana || trail[0].ItemVersion != 3 {
			t.Errorf("the trail %+v %v", trail, err)
		}
		to, err := approvals.DelegationsTo(ctx, ben)
		if err != nil || len(to) != 1 || to[0].AccountID != ana {
			t.Errorf("the delegations to ben %+v %v", to, err)
		}
		// The walk carries the approval with its trail under the entry's collection.
		walked, err := postgres.SnapshotRepository{}.Approvals(ctx, "", 500)
		if err != nil {
			return err
		}
		carried := false
		for _, row := range walked {
			if row.Approval.ID == opened.ID {
				carried = len(row.Decisions) == 1 && !row.CollectionID.IsZero()
			}
		}
		if !carried {
			t.Errorf("the walk did not carry the approval with its trail and collection: %+v", walked)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := approvals.Find(ctx, opened.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace reads the approval: %v", err)
		}
		if trail, _ := approvals.Decisions(ctx, opened.ID); len(trail) != 0 {
			t.Errorf("the other workspace reads the trail")
		}
		if pending, _ := approvals.PendingFor(ctx, repository.PendingQuery{People: []shared.ID{ana, ben}, Limit: 10}); len(pending) != 0 {
			t.Errorf("the other workspace lists the approvals")
		}
		walked, _ := postgres.SnapshotRepository{}.Approvals(ctx, "", 500)
		for _, row := range walked {
			if row.Approval.ID == opened.ID {
				t.Errorf("the other workspace's walk carries the approval")
			}
		}
		if _, err := approvals.DelegationOf(ctx, ana); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace reads the delegation: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	stray := opened
	stray.ID, stray.TenantID = freshID(t), tenantB
	if err := write(ctx, t, tenantB, func(ctx context.Context) error { return approvals.Insert(ctx, stray) }); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the other workspace opened an approval on the entry: %v", err)
	}
}

// The list's selection is the database's: a reader gets the waiting approvals whose current level
// counts one of their people or names a role - not somebody else's - oldest first after the
// cursor, with every page's decisions in one read; the workspace next door gets none of them.
func TestThePendingApprovalsAreSelectedForTheReader(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	approvals := postgres.NewServiceDeskRepository().Approvals()
	now := time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC)
	ana, ben, cleo := freshID(t), freshID(t), freshID(t)
	open := func(at time.Time, level work.ApprovalLevel, counted []shared.ID) servicedesk.Approval {
		item := seedWorkItemForRules(ctx, t, tenantA)
		step := work.WorkflowStep{Key: "approve", To: "approved", Approval: &work.StepApproval{Levels: []work.ApprovalLevel{level}}}
		opened, err := servicedesk.NewApproval(freshID(t), work.WorkItem{ID: item, TenantID: tenantA, Version: 1}, "purchase", step,
			[][]shared.ID{counted}, cleo, at)
		if err != nil {
			t.Fatal(err)
		}
		if err := write(ctx, t, tenantA, func(ctx context.Context) error { return approvals.Insert(ctx, opened) }); err != nil {
			t.Fatal(err)
		}
		return opened
	}
	anas := open(now, work.ApprovalLevel{People: []string{ana.String()}, Rule: work.ApprovalAny}, []shared.ID{ana})
	_ = open(now.Add(time.Minute), work.ApprovalLevel{People: []string{ben.String()}, Rule: work.ApprovalAny}, []shared.ID{ben})
	byRole := open(now.Add(2*time.Minute), work.ApprovalLevel{Roles: []string{"approver"}, Rule: work.ApprovalAny}, nil)
	_, decision, err := servicedesk.Decide(anas, nil, freshID(t), ana, servicedesk.Eligibility{Direct: true},
		servicedesk.VerdictApprove, "", 1, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return approvals.Record(ctx, decision) }); err != nil {
		t.Fatal(err)
	}

	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		first, err := approvals.PendingFor(ctx, repository.PendingQuery{People: []shared.ID{ana}, Limit: 1})
		if err != nil || len(first) != 1 || first[0].ID != anas.ID {
			t.Errorf("the first page %+v %v", first, err)
		}
		rest, err := approvals.PendingFor(ctx, repository.PendingQuery{
			People: []shared.ID{ana}, AfterRequestedAt: anas.RequestedAt, AfterID: anas.ID, Limit: 10,
		})
		if err != nil || len(rest) != 1 || rest[0].ID != byRole.ID {
			t.Errorf("after the cursor %+v %v - ben's approval must not be ana's", rest, err)
		}
		trails, err := approvals.DecisionsOf(ctx, []shared.ID{anas.ID, byRole.ID})
		if err != nil || len(trails[anas.ID]) != 1 || len(trails[byRole.ID]) != 0 {
			t.Errorf("the decisions %+v %v", trails, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if pending, _ := approvals.PendingFor(ctx, repository.PendingQuery{People: []shared.ID{ana}, Limit: 10}); len(pending) != 0 {
			t.Errorf("the other workspace lists %d approvals", len(pending))
		}
		if trails, _ := approvals.DecisionsOf(ctx, []shared.ID{anas.ID}); len(trails) != 0 {
			t.Errorf("the other workspace reads the decisions")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
