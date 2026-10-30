// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	planrepo "github.com/Jersyfi/hubtask/core/application/repository/planning"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	idmodel "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
)

// resources keeps capacities, absences, rates and budgets in memory.
type resources struct {
	capacities []domain.Capacity
	absences   map[shared.ID]domain.Absence
	rates      map[shared.ID]domain.CostRate
	budgets    map[shared.ID]domain.Budget
}

func newResources() *resources {
	return &resources{absences: map[shared.ID]domain.Absence{}, rates: map[shared.ID]domain.CostRate{}, budgets: map[shared.ID]domain.Budget{}}
}

func (r *resources) Capacities(_ context.Context, accounts []shared.ID, _ int) ([]domain.Capacity, error) {
	var out []domain.Capacity
	for _, c := range r.capacities {
		if len(accounts) == 0 || accounts[0] == c.AccountID {
			out = append(out, c)
		}
	}
	return out, nil
}
func (r *resources) PutCapacity(_ context.Context, _ shared.ID, c domain.Capacity, _ shared.ID, _ time.Time) error {
	for i, held := range r.capacities {
		if held.AccountID == c.AccountID && held.From == c.From {
			r.capacities[i] = c
			return nil
		}
	}
	r.capacities = append(r.capacities, c)
	return nil
}
func (r *resources) RemoveCapacity(_ context.Context, account shared.ID, from domain.Day) (bool, error) {
	for i, held := range r.capacities {
		if held.AccountID == account && held.From == from {
			r.capacities = append(r.capacities[:i], r.capacities[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}
func (r *resources) Absences(_ context.Context, account shared.ID, from, to domain.Day, _ int) ([]domain.Absence, error) {
	var out []domain.Absence
	for _, a := range r.absences {
		if (account.IsZero() || a.AccountID == account) && a.From <= to && a.To >= from {
			out = append(out, a)
		}
	}
	return out, nil
}
func (r *resources) FindAbsence(_ context.Context, id shared.ID) (domain.Absence, error) {
	a, ok := r.absences[id]
	if !ok {
		return a, shared.ErrNotFound
	}
	return a, nil
}
func (r *resources) PutAbsence(_ context.Context, a domain.Absence, expected int, _ shared.ID, _ time.Time) error {
	if r.absences[a.ID].Version != expected {
		return shared.ErrVersionConflict
	}
	a.Version = expected + 1
	r.absences[a.ID] = a
	return nil
}
func (r *resources) RemoveAbsence(_ context.Context, id shared.ID, expected int) error {
	if r.absences[id].Version != expected {
		return shared.ErrVersionConflict
	}
	delete(r.absences, id)
	return nil
}
func (r *resources) Rates(context.Context, int) ([]domain.CostRate, error) {
	var out []domain.CostRate
	for _, rate := range r.rates {
		out = append(out, rate)
	}
	return out, nil
}
func (r *resources) FindRate(_ context.Context, id shared.ID) (domain.CostRate, error) {
	rate, ok := r.rates[id]
	if !ok {
		return rate, shared.ErrNotFound
	}
	return rate, nil
}
func (r *resources) PutRate(_ context.Context, rate domain.CostRate, expected int, _ shared.ID, _ time.Time) error {
	if r.rates[rate.ID].Version != expected {
		return shared.ErrVersionConflict
	}
	rate.Version = expected + 1
	r.rates[rate.ID] = rate
	return nil
}
func (r *resources) RemoveRate(_ context.Context, id shared.ID, expected int) error {
	if r.rates[id].Version != expected {
		return shared.ErrVersionConflict
	}
	delete(r.rates, id)
	return nil
}
func (r *resources) Budget(_ context.Context, plan shared.ID) (domain.Budget, bool, error) {
	b, ok := r.budgets[plan]
	return b, ok, nil
}
func (r *resources) PutBudget(_ context.Context, _ shared.ID, b domain.Budget, expected int, _ shared.ID, _ time.Time) error {
	b.Version = expected + 1
	r.budgets[b.PlanID] = b
	return nil
}
func (r *resources) RemoveBudget(_ context.Context, plan shared.ID, _ int) error {
	delete(r.budgets, plan)
	return nil
}

// roles holds ana as a custom "designer" in the hub and as MEMBER at the workspace, the most
// specific first.
type roles struct{}

func (roles) RolesBy(_ context.Context, _ appshared.ActorContext, account shared.ID, _ []idmodel.Scope) ([]string, error) {
	if account != ana {
		return nil, nil
	}
	return []string{"designer", "MEMBER"}, nil
}

// assigned is a plan whose checkout form is ana's, due Friday, its payment ben's without dates.
type assigned struct{ planrepo.Plans }

func (assigned) Entries(context.Context, shared.ID, int) ([]planrepo.PlanEntry, error) {
	start := monday
	due := monday.AddDate(0, 0, 4)
	return []planrepo.PlanEntry{
		{ID: checkout, HubID: hubID, CollectionID: listID, AssigneeID: ana, StartAt: &start, DueAt: &due},
		{ID: payment, HubID: hubID, CollectionID: listID, AssigneeID: ben},
		{ID: secret, HubID: hubID, CollectionID: listID, AssigneeID: ana, StartAt: &start, DueAt: &due},
	}, nil
}

func (f *fixture) withResources() *resources {
	r := newResources()
	f.keeper.Resources, f.keeper.Roles = r, roles{}
	return r
}

// UC-TIM-06 check 1: a person writes their own capacity; another person's needs MANAGE_MEMBERS.
func TestAPersonSetsTheirCapacityAndOnlyAManagerAnothers(t *testing.T) {
	f := newFixture()
	r := f.withResources()
	ctx := context.Background()
	week := [7]int{360, 360, 360, 360, 0, 0, 0}
	c, err := SetCapacity{Keeper: f.keeper}.Execute(ctx, as(ana), domain.Capacity{From: day(t, "2026-11-02"), WeekMinutes: week})
	if err != nil || c.AccountID != ana || c.Version != 1 || len(r.capacities) != 1 {
		t.Fatalf("own capacity: %+v %v", c, err)
	}
	if _, err := (SetCapacity{Keeper: f.keeper}).Execute(ctx, as(ben), domain.Capacity{AccountID: ana, From: day(t, "2026-11-09"), WeekMinutes: week}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("another person's capacity without MANAGE_MEMBERS: %v", err)
	}
	f.gate.approvers[lead] = append(f.gate.approvers[lead], tenantID)
	if _, err := (SetCapacity{Keeper: f.keeper}).Execute(ctx, as(lead), domain.Capacity{AccountID: ana, From: day(t, "2026-11-09"), WeekMinutes: week}); err != nil {
		t.Fatalf("a manager writes it: %v", err)
	}
	if _, err := (SetCapacity{Keeper: f.keeper}).Execute(ctx, as(ana), domain.Capacity{From: day(t, "2026-11-02"), WeekMinutes: [7]int{2000}}); detail(err) != "time.capacity_out_of_range" {
		t.Fatalf("a day of more than a day: %v", err)
	}
	if _, err := (ListCapacities{Keeper: f.keeper}).Execute(ctx, as(ben), ana); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("ben reads ana's capacity: %v", err)
	}
	if err := (RemoveCapacity{Keeper: f.keeper}).Execute(ctx, as(ana), "", day(t, "2026-11-09")); err != nil || len(r.capacities) != 1 {
		t.Fatalf("remove: %v %+v", err, r.capacities)
	}
}

// UC-TIM-07 checks 1 to 3: an absence is recorded once, never twice on a day, and every member
// reads the shared calendar - with the kind, and no reason.
func TestAnAbsenceIsRecordedOnceAndSharedByKind(t *testing.T) {
	f := newFixture()
	f.withResources()
	ctx := context.Background()
	leave, err := RecordAbsence{Keeper: f.keeper}.Execute(ctx, as(ana), domain.Absence{
		From: day(t, "2026-11-09"), To: day(t, "2026-11-13"), Kind: domain.AbsenceLeave,
	})
	if err != nil || leave.AccountID != ana {
		t.Fatalf("record: %+v %v", leave, err)
	}
	if _, err := (RecordAbsence{Keeper: f.keeper}).Execute(ctx, as(ana), domain.Absence{
		From: day(t, "2026-11-13"), To: day(t, "2026-11-16"), Kind: domain.AbsenceOther,
	}); detail(err) != "time.absence_overlaps" {
		t.Fatalf("an overlap: %v", err)
	}
	if _, err := (RecordAbsence{Keeper: f.keeper}).Execute(ctx, as(ana), domain.Absence{
		From: day(t, "2026-11-20"), To: day(t, "2026-11-20"), Kind: domain.AbsenceKind("SICK"),
	}); detail(err) != "time.absence_kind_unknown" {
		t.Fatalf("a reason as a kind: %v", err)
	}
	if _, err := (RecordAbsence{Keeper: f.keeper}).Execute(ctx, as(ben), domain.Absence{
		AccountID: ana, From: day(t, "2026-11-20"), To: day(t, "2026-11-20"), Kind: domain.AbsenceOther,
	}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("ben records ana's absence: %v", err)
	}
	seen, err := ListAbsences{Keeper: f.keeper}.Execute(ctx, as(ben), "", day(t, "2026-11-01"), day(t, "2026-11-30"))
	if err != nil || len(seen) != 1 {
		t.Fatalf("the shared calendar: %+v %v", seen, err)
	}
	out := AbsenceOutput(seen[0])
	if out["kind"] != "LEAVE" || len(out) != 7 {
		t.Fatalf("the calendar tells more than the kind: %+v", out)
	}
	moved, err := UpdateAbsence{Keeper: f.keeper}.Execute(ctx, as(ana), domain.Absence{
		ID: leave.ID, From: day(t, "2026-11-10"), To: day(t, "2026-11-13"), Kind: domain.AbsenceTraining, Half: true,
	}, 1)
	if err != nil || moved.Version != 2 || moved.AccountID != ana {
		t.Fatalf("update: %+v %v", moved, err)
	}
	if err := (RemoveAbsence{Keeper: f.keeper}).Execute(ctx, as(ben), leave.ID, 2); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("ben removes ana's absence: %v", err)
	}
	if err := (RemoveAbsence{Keeper: f.keeper}).Execute(ctx, as(ana), leave.ID, 2); err != nil {
		t.Fatalf("remove: %v", err)
	}
}

// UC-TIM-06 checks 2 to 4: the workload sets each person's available minutes - capacity less
// absences - against what their open entries' remaining estimates ask over the scheduled days,
// marks an overload, keeps an unreadable entry out, and under AGGREGATED gives a reader who does
// not approve only the team's sums.
func TestTheWorkloadShowsWhoHasRoom(t *testing.T) {
	f := newFixture()
	r := f.withResources()
	f.keeper.Plans = assigned{}
	ctx := context.Background()
	r.capacities = []domain.Capacity{
		{AccountID: ana, From: day(t, "2026-01-01"), WeekMinutes: [7]int{60, 60, 60, 60, 60, 0, 0}},
		{AccountID: lead, From: day(t, "2026-01-01"), WeekMinutes: [7]int{480, 480, 480, 480, 480, 0, 0}},
	}
	r.absences[ben] = domain.Absence{ID: ben, AccountID: ana, From: day(t, "2026-11-06"), To: day(t, "2026-11-06"), Kind: domain.AbsenceLeave}
	f.log(t, ana, checkout, "2026-10-30", 60)
	w, err := ReadWorkload{Keeper: f.keeper}.Execute(ctx, as(ben), WorkloadQuery{PlanID: listID, From: day(t, "2026-11-02"), Weeks: 2})
	if err != nil || w.Aggregated {
		t.Fatalf("workload: %+v %v", w, err)
	}
	cells := map[string]domain.WorkloadCell{}
	for _, c := range w.Cells {
		cells[c.AccountID.String()+c.Week.String()] = c
	}
	first := cells[ana.String()+"2026-11-02"]
	// 4 hours estimated, 1 recorded: 180 minutes on four working days; 240 available less Friday.
	if first.Available != 240 || first.Allocated != 180 || first.Over() {
		t.Fatalf("ana's first week: %+v", first)
	}
	if room := cells[lead.String()+"2026-11-02"]; !room.Under() || room.Allocated != 0 {
		t.Fatalf("the lead has room: %+v", room)
	}
	if w.UnscheduledMinutes != 120 {
		t.Fatalf("ben's payment without dates: %d", w.UnscheduledMinutes)
	}
	r.capacities[0].WeekMinutes = [7]int{30, 30, 30, 30, 30, 0, 0}
	w, _ = ReadWorkload{Keeper: f.keeper}.Execute(ctx, as(ben), WorkloadQuery{PlanID: listID, From: day(t, "2026-11-02"), Weeks: 1})
	for _, c := range w.Cells {
		if c.AccountID == ana && !c.Over() {
			t.Fatalf("ana is over allocated: %+v", c)
		}
	}
	f.store.policy.Visibility = domain.VisibilityAggregated
	w, err = ReadWorkload{Keeper: f.keeper}.Execute(ctx, as(ben), WorkloadQuery{PlanID: listID, From: day(t, "2026-11-02"), Weeks: 1})
	if err != nil || !w.Aggregated || len(w.Cells) != 0 || len(w.Team) != 1 || w.Team[0].People != 2 {
		t.Fatalf("aggregated for a reader: %+v %v", w, err)
	}
	if out := fmt.Sprint(WorkloadOutput(listID, w)); strings.Contains(out, ana.String()) {
		t.Fatalf("the sums name a person: %s", out)
	}
	w, err = ReadWorkload{Keeper: f.keeper}.Execute(ctx, as(lead), WorkloadQuery{PlanID: listID, From: day(t, "2026-11-02"), Weeks: 1})
	if err != nil || w.Aggregated || len(w.Cells) == 0 {
		t.Fatalf("an approver reads the people: %+v %v", w, err)
	}
}

// UC-TIM-08 checks 1 to 5: rates and budgets are only MANAGE_COSTS's; a person's rate comes before
// their role's; the burn prices each day by its rate and tells a passed budget; a rate's amount
// never reaches the trail.
func TestABudgetBurnsByTheRates(t *testing.T) {
	f := newFixture()
	r := f.withResources()
	ctx := context.Background()
	if _, err := (ListCostRates{Keeper: f.keeper}).Execute(ctx, as(ana)); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member reads the rates: %v", err)
	}
	f.gate.approvers[lead] = append(f.gate.approvers[lead], tenantID)
	from := day(t, "2026-01-01")
	designer, err := SetCostRate{Keeper: f.keeper}.Execute(ctx, as(lead), domain.CostRate{Role: "designer", HourMinor: 6000, Currency: "EUR", From: from}, 0)
	if err != nil {
		t.Fatalf("a role's rate: %v", err)
	}
	if _, err := (SetCostRate{Keeper: f.keeper}).Execute(ctx, as(lead), domain.CostRate{Role: "designer", HourMinor: 7000, Currency: "EUR", From: day(t, "2026-06-01")}, 0); detail(err) != "time.rate_overlaps" {
		t.Fatalf("two rates on a day: %v", err)
	}
	until := day(t, "2026-11-02")
	if _, err := (SetCostRate{Keeper: f.keeper}).Execute(ctx, as(lead), domain.CostRate{AccountID: ana, HourMinor: 9000, Currency: "EUR", From: from, To: &until}, 0); err != nil {
		t.Fatalf("a person's rate: %v", err)
	}
	f.log(t, ana, checkout, "2026-11-02", 60) // ana's own rate: 90.00
	f.log(t, ana, payment, "2026-11-03", 120) // her role's in the hub: 2 x 60.00
	f.log(t, ben, checkout, "2026-11-03", 30) // no rate: unpriced
	if _, err := (SetBudget{Keeper: f.keeper}).Execute(ctx, as(ana), domain.Budget{PlanID: hubID, Minutes: 120}, 0); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member sets the budget: %v", err)
	}
	if _, err := (SetBudget{Keeper: f.keeper}).Execute(ctx, as(lead), domain.Budget{PlanID: hubID, Minutes: 180, AmountMinor: 20000, Currency: "EUR"}, 0); err != nil {
		t.Fatalf("set the budget: %v", err)
	}
	burn, held, err := ReadBudget{Keeper: f.keeper}.Execute(ctx, as(lead), hubID)
	if err != nil || !held {
		t.Fatalf("burn: %+v %v", burn, err)
	}
	if burn.Minutes != 210 || burn.AmountMinor != 21000 || burn.UnpricedMinutes != 30 || !burn.OverTime() || !burn.OverMoney() {
		t.Fatalf("burn = %+v", burn)
	}
	if len(burn.Weeks) != 1 || burn.Weeks[0].AmountMinor != 21000 {
		t.Fatalf("weeks = %+v", burn.Weeks)
	}
	if _, _, err := (ReadBudget{Keeper: f.keeper}).Execute(ctx, as(ana), hubID); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member reads the burn: %v", err)
	}
	for _, e := range f.audit.entries {
		if strings.Contains(fmt.Sprint(e.Changes), "6000") || strings.Contains(fmt.Sprint(e.Changes), "9000") {
			t.Fatalf("a rate's amount reached the trail: %+v", e)
		}
	}
	if err := (RemoveCostRate{Keeper: f.keeper}).Execute(ctx, as(lead), designer.ID, 1); err != nil || len(r.rates) != 1 {
		t.Fatalf("remove the rate: %v", err)
	}
}
