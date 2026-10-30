// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"slices"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

type levelClocks struct {
	armed   map[string]time.Duration
	stopped []string
}

func (c *levelClocks) Arm(_ context.Context, _ shared.ID, purpose calendar.Purpose, _ shared.ID, name string,
	_ calendar.BusinessCalendar, budget time.Duration) (calendar.Timer, error) {
	if purpose != calendar.PurposeApproval {
		panic(purpose)
	}
	c.armed[name] = budget
	return calendar.Timer{}, nil
}

func (c *levelClocks) Stop(_ context.Context, _ calendar.Purpose, _ shared.ID, name string) (calendar.Timer, error) {
	if _, held := c.armed[name]; !held {
		return calendar.Timer{}, shared.ErrNotFound
	}
	delete(c.armed, name)
	c.stopped = append(c.stopped, name)
	return calendar.Timer{}, nil
}

type toldPeople struct{ asked, reminded [][]shared.ID }

func (t *toldPeople) ApprovalAsked(_ context.Context, _ domain.Approval, people []shared.ID, _ shared.ID) error {
	t.asked = append(t.asked, slices.Clone(people))
	return nil
}

func (t *toldPeople) ApprovalReminded(_ context.Context, _ domain.Approval, people []shared.ID) error {
	t.reminded = append(t.reminded, slices.Clone(people))
	return nil
}

// UC-SVC-08 check 5, UC-SVC-09 check 4: a level's people are told when it begins, reminded halfway
// through its hours and at their end, when the person who asked is told too; the next level is
// told when the first passes, and a decided approval stops its clocks.
func TestALevelsPeopleAreToldRemindedAndTheAskerToldWhenOverdue(t *testing.T) {
	approvals, store, _, items := approvalFixture(t)
	clocks, told := &levelClocks{armed: map[string]time.Duration{}}, &toldPeople{}
	approvals.Timers, approvals.Notify = clocks, told
	step := workdomain.WorkflowStep{Key: "approve", To: "approved", Approval: &workdomain.StepApproval{DueHours: 8,
		Levels: []workdomain.ApprovalLevel{
			{People: []string{handlerID.String()}, Rule: workdomain.ApprovalAny},
			{People: []string{requesterID.String()}, Rule: workdomain.ApprovalAny},
		}}}
	if err := approvals.Open(context.Background(), person(requesterID), items.item, workdomain.Container{}, "purchase", step); err != nil {
		t.Fatal(err)
	}
	if len(told.asked) != 1 || !slices.Equal(told.asked[0], []shared.ID{handlerID}) {
		t.Fatalf("asked %v", told.asked)
	}
	if clocks.armed["level_0_remind"] != 4*time.Hour || clocks.armed["level_0_due"] != 8*time.Hour {
		t.Fatalf("armed %v", clocks.armed)
	}
	var id shared.ID
	for key := range store.rows {
		id = key
	}
	elapsed := ApprovalElapsed{Store: store, Notify: told}
	for _, name := range []string{"level_0_remind", "level_0_due", "level_1_due", "elsewhere"} {
		if err := elapsed.Elapsed(context.Background(), appshared.ActorContext{},
			calendar.Timer{TenantID: tenantID, SubjectID: id, Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	if len(told.reminded) != 2 || !slices.Equal(told.reminded[0], []shared.ID{handlerID}) ||
		!slices.Equal(told.reminded[1], []shared.ID{handlerID, requesterID}) {
		t.Fatalf("reminded %v; a timer of another level or name tells nobody", told.reminded)
	}
	if _, err := (DecideApproval{Approvals: approvals}).Execute(context.Background(), person(handlerID),
		DecideApprovalCommand{ApprovalID: id, Verdict: domain.VerdictApprove}); err != nil {
		t.Fatal(err)
	}
	if len(told.asked) != 2 || !slices.Equal(told.asked[1], []shared.ID{requesterID}) {
		t.Errorf("the second level asked %v", told.asked)
	}
	if _, held := clocks.armed["level_0_due"]; held || clocks.armed["level_1_due"] != 8*time.Hour {
		t.Errorf("after the first level passed the clocks are %v", clocks.armed)
	}
	if _, err := (DecideApproval{Approvals: approvals}).Execute(context.Background(), person(requesterID),
		DecideApprovalCommand{ApprovalID: id, Verdict: domain.VerdictApprove}); err != nil {
		t.Fatal(err)
	}
	if len(clocks.armed) != 0 {
		t.Errorf("a decided approval left clocks running: %v", clocks.armed)
	}
	if err := elapsed.Elapsed(context.Background(), appshared.ActorContext{},
		calendar.Timer{TenantID: tenantID, SubjectID: id, Name: "level_1_due"}); err != nil || len(told.reminded) != 2 {
		t.Errorf("a decided approval reminded somebody: %v %v", told.reminded, err)
	}
}
