// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// An approval, its decisions and a delegation are read as written, one approval waits on an
// entry at a time, and the workspace next door reads, decides and delegates none of them
// (migration 0268, UC-SVC-08 to 03).
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
		if err := approvals.Insert(ctx, opened); err != nil {
			return err
		}
		second := opened
		second.ID = freshID(t)
		if err := approvals.Insert(ctx, second); !errors.Is(err, shared.ErrConflict) {
			t.Errorf("a second approval waits on the entry: %v", err)
		}
		if err := approvals.Record(ctx, tenantA, decision); err != nil {
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
		if pending, _ := approvals.Pending(ctx, 10); len(pending) != 0 {
			t.Errorf("the other workspace lists the approvals")
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
