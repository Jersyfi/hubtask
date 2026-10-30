// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
)

func (s *snapshotStore) OwnTimesheets(_ context.Context, accountID, after shared.ID, batch int) ([]timekeeping.Timesheet, error) {
	var own []timekeeping.Timesheet
	for _, sheet := range s.sheets {
		if sheet.AccountID == accountID {
			own = append(own, sheet)
		}
	}
	return pageAfter(own, func(sheet timekeeping.Timesheet) string { return sheet.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) OwnCapacities(_ context.Context, accountID shared.ID, _ int) ([]timekeeping.Capacity, error) {
	var own []timekeeping.Capacity
	for _, c := range s.capacities {
		if c.AccountID == accountID {
			own = append(own, c)
		}
	}
	return own, nil
}

func (s *snapshotStore) TimeAbsences(_ context.Context, after shared.ID, batch int) ([]timekeeping.Absence, error) {
	return pageAfter(s.absences, func(a timekeeping.Absence) string { return a.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) TimeCostRates(context.Context, int) ([]timekeeping.CostRate, error) {
	return s.rates, nil
}

func (s *snapshotStore) TimeBudgets(_ context.Context, after shared.ID, batch int) ([]timekeeping.Budget, error) {
	return pageAfter(s.budgets, func(b timekeeping.Budget) string { return b.PlanID.String() }, after.String(), batch), nil
}

// UC-TIM-02, 06, 07 and 08 offline: the walk hands a reader their own weeks and capacities and
// never another person's, every absence, and the rates and a budget only where they manage costs.
func TestTheWalkCarriesWeeksResourcesAndCostsToTheirReaders(t *testing.T) {
	pull, f, store := walking(t)
	f.auth.workspace = false
	store.sheets = []timekeeping.Timesheet{
		{ID: idAt('c', 1), AccountID: account, State: timekeeping.SheetSubmitted},
		{ID: idAt('c', 2), AccountID: someoneElse, State: timekeeping.SheetSubmitted},
	}
	store.capacities = []timekeeping.Capacity{{AccountID: account}, {AccountID: someoneElse}}
	store.absences = []timekeeping.Absence{
		{ID: idAt('d', 1), AccountID: someoneElse, Kind: timekeeping.AbsenceLeave},
		{ID: idAt('d', 2), AccountID: account, Kind: timekeeping.AbsenceTraining},
	}
	store.rates = []timekeeping.CostRate{{ID: idAt('e', 1), Role: "designer", HourMinor: 6000, Currency: "EUR"}}
	store.budgets = []timekeeping.Budget{{PlanID: readable, Minutes: 600}, {PlanID: hidden, Minutes: 60}}

	kinds := func() map[string][]shared.ID {
		records, _ := wholeWalk(t, pull, 100)
		out := map[string][]shared.ID{}
		for _, record := range records {
			out[record.Entity] = append(out[record.Entity], record.EntityID)
		}
		return out
	}
	got := kinds()
	if !slices.Equal(got[entityTimesheet], []shared.ID{idAt('c', 1)}) {
		t.Errorf("weeks %v, want the reader's own", got[entityTimesheet])
	}
	if !slices.Equal(got[entityCapacities], []shared.ID{account}) {
		t.Errorf("capacities %v, want the reader's own", got[entityCapacities])
	}
	if len(got[entityAbsence]) != 2 {
		t.Errorf("absences %v, want every member's", got[entityAbsence])
	}
	if len(got[entityCostRates]) != 0 || len(got[entityBudget]) != 0 {
		t.Errorf("rates %v and budgets %v reached a reader who does not manage costs", got[entityCostRates], got[entityBudget])
	}

	f.auth.costs = map[shared.ID]bool{"": true, readable: true}
	got = kinds()
	if !slices.Equal(got[entityCostRates], []shared.ID{tenant}) {
		t.Errorf("rates %v, want the workspace's record", got[entityCostRates])
	}
	if !slices.Equal(got[entityBudget], []shared.ID{readable}) {
		t.Errorf("budgets %v, want the one of the plan the reader may read and manage costs at", got[entityBudget])
	}
}

// The delta delivers a week and capacities to their person alone, an absence to every member, the
// rates and a budget only to whoever manages costs there.
func TestTheDeltaDeliversWeeksResourcesAndCostsToTheirReaders(t *testing.T) {
	record := func(seq int64, entity string, container shared.ID, payload map[string]any) repository.Recorded {
		recorded := entry(seq, container)
		recorded.Entity, recorded.Payload = entity, payload
		return recorded
	}
	held := func(holder shared.ID) map[string]any { return map[string]any{"account_id": holder.String()} }
	pull, f := pulling(t,
		record(1, entityTimesheet, "", held(account)),
		record(2, entityTimesheet, "", held(someoneElse)),
		record(3, entityCapacities, "", held(someoneElse)),
		record(4, entityAbsence, "", map[string]any{"account_id": someoneElse.String(), "kind": "LEAVE"}),
		record(5, entityCostRates, "", map[string]any{"data": []any{}}),
		record(6, entityBudget, readable, map[string]any{"plan_id": readable.String()}),
	)
	f.auth.workspace = false
	f.auth.costs = map[shared.ID]bool{readable: true}

	page, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Cursor: cursorAt(f, 0), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var seqs []int64
	for _, delivered := range page.Records {
		seqs = append(seqs, delivered.Seq)
	}
	if !slices.Equal(seqs, []int64{1, 4, 6}) {
		t.Errorf("the delta delivered %v, want the reader's week, the absence and the plan's budget", seqs)
	}
}
