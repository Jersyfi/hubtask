// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"testing"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
)

type changes struct{ recorded []changelog.Change }

func (c *changes) Record(_ context.Context, change changelog.Change) error {
	c.recorded = append(c.recorded, change)
	return nil
}

type hlc struct{}

func (hlc) Next() shared.HLC { return shared.HLC{} }

func (c *changes) last(entity string) changelog.Change {
	for i := len(c.recorded) - 1; i >= 0; i-- {
		if c.recorded[i].Entity == entity {
			return c.recorded[i]
		}
	}
	return changelog.Change{}
}

// UC-TIM-01 offline: a person's records and running timer reach the change log addressed to them
// under no container - a removal as well -, and the kinds of work and the policy as one record each
// under the workspace.
func TestTimeIsToldToItsPersonsDevices(t *testing.T) {
	f := newFixture()
	log := &changes{}
	f.keeper.Changes, f.keeper.HLC = log, hlc{}
	ctx := context.Background()

	r := f.log(t, ana, checkout, "2026-11-02", 90)
	got := log.last(RecordEntity)
	if got.EntityID != r.ID || got.Op != changelog.Upsert || !got.ContainerID.IsZero() || got.Payload["account_id"] != ana.String() || got.Payload["minutes"] != 90 {
		t.Fatalf("a record's change = %+v", got)
	}
	if err := (DeleteTimeRecord{Keeper: f.keeper}).Execute(ctx, as(ana), r.ID, r.Version); err != nil {
		t.Fatal(err)
	}
	if got := log.last(RecordEntity); got.Op != changelog.Delete || got.EntityID != r.ID || got.Payload["account_id"] != ana.String() {
		t.Errorf("a removal = %+v, want it addressed to its person", got)
	}

	if _, err := (StartTimer{Keeper: f.keeper}).Execute(ctx, as(ana), checkout, "development", "", nil); err != nil {
		t.Fatal(err)
	}
	if got := log.last(TimerEntity); got.EntityID != ana || got.Op != changelog.Upsert || got.Payload["entry_id"] != checkout.String() {
		t.Errorf("a timer started = %+v", got)
	}
	f.clock.now = monday.Add(20 * time.Minute)
	stopped, err := StopTimer{Keeper: f.keeper}.Execute(ctx, as(ana), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if got := log.last(TimerEntity); got.Op != changelog.Delete || got.Payload["account_id"] != ana.String() {
		t.Errorf("a timer stopped = %+v, want its removal", got)
	}
	if got := log.last(RecordEntity); got.EntityID != stopped.ID || got.Op != changelog.Upsert {
		t.Errorf("the stopped timer's record = %+v", got)
	}

	if _, err := (DefineWorkType{Keeper: f.keeper}).Execute(ctx, as(lead), domain.WorkType{Key: "travel", Name: "Travel"}); err != nil {
		t.Fatal(err)
	}
	got = log.last(WorkTypesEntity)
	kinds, _ := got.Payload["data"].([]usecase.Output)
	found := false
	for _, k := range kinds {
		found = found || k["key"] == "travel"
	}
	if got.EntityID != tenantID || !found {
		t.Errorf("the kinds of work = %+v, want the workspace's list with the new one", got)
	}
}

// UC-TIM-02, 06, 07 and 08 offline: a week and a person's capacities reach the change log addressed
// to their person - a week decided by an approver as well -, an absence for every member, the rates
// as one record under the workspace and a budget under its plan, a removal of each recorded.
func TestWeeksResourcesAndCostsAreToldToTheirDevices(t *testing.T) {
	f := newFixture()
	f.withResources()
	log := &changes{}
	f.keeper.Changes, f.keeper.HLC = log, hlc{}
	ctx := context.Background()

	r := f.log(t, ana, checkout, "2026-11-02", 240)
	submitted, err := SubmitTimesheet{Keeper: f.keeper}.Execute(ctx, as(ana), r.Day)
	if err != nil {
		t.Fatal(err)
	}
	got := log.last(SheetEntity)
	if submitted.ID.IsZero() || got.EntityID != submitted.ID || got.Payload["account_id"] != ana.String() || got.Payload["state"] != "SUBMITTED" || got.Payload["version"] != 1 {
		t.Fatalf("a week handed in = %+v", got)
	}
	if _, err := (RejectTimesheet{Keeper: f.keeper}).Execute(ctx, as(lead), ana, r.Day, "Tuesday is missing"); err != nil {
		t.Fatal(err)
	}
	if got := log.last(SheetEntity); got.EntityID != submitted.ID || got.Payload["account_id"] != ana.String() || got.Payload["state"] != "REJECTED" || got.Payload["version"] != 2 {
		t.Errorf("a week sent back = %+v, want it addressed to its person", got)
	}

	week := [7]int{360, 360, 360, 360}
	if _, err := (SetCapacity{Keeper: f.keeper}).Execute(ctx, as(ana), domain.Capacity{From: day(t, "2026-11-02"), WeekMinutes: week}); err != nil {
		t.Fatal(err)
	}
	got = log.last(CapacitiesEntity)
	list, _ := got.Payload["data"].([]usecase.Output)
	if got.EntityID != ana || got.Payload["account_id"] != ana.String() || len(list) != 1 {
		t.Fatalf("a capacity set = %+v", got)
	}
	if err := (RemoveCapacity{Keeper: f.keeper}).Execute(ctx, as(ana), "", day(t, "2026-11-02")); err != nil {
		t.Fatal(err)
	}
	if list, _ := log.last(CapacitiesEntity).Payload["data"].([]usecase.Output); len(list) != 0 {
		t.Errorf("a capacity removed leaves %v", list)
	}

	leave, err := RecordAbsence{Keeper: f.keeper}.Execute(ctx, as(ana), domain.Absence{From: day(t, "2026-11-09"), To: day(t, "2026-11-13"), Kind: domain.AbsenceLeave})
	if err != nil {
		t.Fatal(err)
	}
	if got := log.last(AbsenceEntity); got.EntityID != leave.ID || got.Op != changelog.Upsert || !got.ContainerID.IsZero() || got.Payload["kind"] != "LEAVE" {
		t.Errorf("an absence = %+v", got)
	}
	if err := (RemoveAbsence{Keeper: f.keeper}).Execute(ctx, as(ana), leave.ID, leave.Version); err != nil {
		t.Fatal(err)
	}
	if got := log.last(AbsenceEntity); got.Op != changelog.Delete || got.EntityID != leave.ID {
		t.Errorf("an absence removed = %+v", got)
	}

	f.gate.approvers[lead] = append(f.gate.approvers[lead], tenantID)
	if _, err := (SetCostRate{Keeper: f.keeper}).Execute(ctx, as(lead), domain.CostRate{Role: "designer", HourMinor: 6000, Currency: "EUR", From: day(t, "2026-01-01")}, 0); err != nil {
		t.Fatal(err)
	}
	got = log.last(RatesEntity)
	if rates, _ := got.Payload["data"].([]usecase.Output); got.EntityID != tenantID || len(rates) != 1 || rates[0]["hour_minor"] != int64(6000) {
		t.Errorf("the rates = %+v", got)
	}
	if _, err := (SetBudget{Keeper: f.keeper}).Execute(ctx, as(lead), domain.Budget{PlanID: hubID, Minutes: 180}, 0); err != nil {
		t.Fatal(err)
	}
	got = log.last(BudgetEntity)
	if got.EntityID != hubID || got.ContainerID != hubID || got.Payload["budget_minutes"] != int64(180) || got.Payload["version"] != 1 {
		t.Errorf("a budget = %+v, want it filed under its plan", got)
	}
	if _, has := got.Payload["weeks"]; has {
		t.Errorf("a budget's record carries its burn: %+v", got.Payload)
	}
	if err := (RemoveBudget{Keeper: f.keeper}).Execute(ctx, as(lead), hubID, 1); err != nil {
		t.Fatal(err)
	}
	if got := log.last(BudgetEntity); got.Op != changelog.Delete || got.ContainerID != hubID {
		t.Errorf("a budget removed = %+v", got)
	}
}
