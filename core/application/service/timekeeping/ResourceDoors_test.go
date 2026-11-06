// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// resourceDoors is the capacity, absence, workload, cost and billing use cases as REST, MCP and
// automation reach them.
func resourceDoors(t *testing.T, k Keeper) *usecase.Registry {
	t.Helper()
	doors, err := usecase.NewRegistry(nil,
		ListCapacities{k}.Descriptor(), SetCapacity{k}.Descriptor(), RemoveCapacity{k}.Descriptor(),
		ListAbsences{k}.Descriptor(), RecordAbsence{k}.Descriptor(), UpdateAbsence{k}.Descriptor(),
		RemoveAbsence{k}.Descriptor(), ReadWorkload{k}.Descriptor(),
		ListCostRates{k}.Descriptor(), SetCostRate{k}.Descriptor(), RemoveCostRate{k}.Descriptor(),
		ReadBudget{k}.Descriptor(), SetBudget{k}.Descriptor(), RemoveBudget{k}.Descriptor(),
		ListBillingRates{k}.Descriptor(), SetBillingRate{k}.Descriptor(), RemoveBillingRate{k}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return doors
}

// UC-TIM-06 checks 1-2 through the doors: a capacity is seven whole numbers of minutes, Monday
// first; one's own is written by anybody, another person's only by a manager; the workload reads
// it, refusing a plan, a day or a zone of no shape.
func TestACapacityAndTheWorkloadThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	r := f.withResources()
	f.keeper.Plans = assigned{}
	doors := resourceDoors(t, f.keeper)
	week := []any{float64(360), float64(360), float64(360), float64(360), float64(0), float64(0), float64(0)}

	for name, in := range map[string]usecase.Input{
		"six days":        {"from": "2026-11-02", "week_minutes": week[:6]},
		"a part of a day": {"from": "2026-11-02", "week_minutes": []any{1.5, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}},
	} {
		if _, err := doors.Invoke(ctx, SetCapacityName, as(ana), in); detail(err) != "time.capacity_week_required" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := doors.Invoke(ctx, SetCapacityName, as(ana), usecase.Input{"from": "Monday", "week_minutes": week}); detail(err) != "time.day_invalid" {
		t.Errorf("a capacity from no day: %v", err)
	}
	if _, err := doors.Invoke(ctx, SetCapacityName, as(ben), usecase.Input{"account_id": ana.String(), "from": "2026-11-02", "week_minutes": week}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("another person's capacity without MANAGE_MEMBERS: %v", err)
	}
	if len(r.capacities) != 0 {
		t.Fatal("a refused capacity was written")
	}
	c, err := doors.Invoke(ctx, SetCapacityName, as(ana), usecase.Input{"from": "2026-11-02", "week_minutes": week})
	if err != nil || c["account_id"] != ana.String() || c["week_minutes"].([]int)[0] != 360 {
		t.Fatalf("her own capacity: %v %v", c, err)
	}
	if out, err := doors.Invoke(ctx, ListCapacitiesName, as(ana), usecase.Input{}); err != nil || len(out["data"].([]usecase.Output)) != 1 {
		t.Fatalf("her capacities: %v %v", out, err)
	}
	if _, err := doors.Invoke(ctx, ListCapacitiesName, as(ben), usecase.Input{"account_id": ana.String()}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("ben reads ana's capacity: %v", err)
	}

	for name, c := range map[string]struct {
		in   usecase.Input
		want string
	}{
		"a day of no shape": {usecase.Input{"plan_id": listID.String(), "from": "soon"}, "time.day_invalid"},
		"an unknown zone":   {usecase.Input{"plan_id": listID.String(), "from": "2026-11-02", "zone": "Atlantis/Main"}, "time.zone_unknown"},
	} {
		if _, err := doors.Invoke(ctx, ReadWorkloadName, as(ben), c.in); detail(err) != c.want {
			t.Errorf("%s: %v", name, err)
		}
	}
	w, err := doors.Invoke(ctx, ReadWorkloadName, as(ben), usecase.Input{"plan_id": listID.String(), "from": "2026-11-02", "weeks": float64(1)})
	if err != nil || w["aggregated"] != false || w.Int("unscheduled_minutes") != 120 {
		t.Fatalf("the workload: %v %v", w, err)
	}
	found := false
	for _, cell := range w["cells"].([]usecase.Output) {
		found = found || (cell["account_id"] == ana.String() && cell["available_minutes"] == 1440)
	}
	if !found {
		t.Fatalf("ana's week of four six-hour days is not in %v", w["cells"])
	}

	if _, err := doors.Invoke(ctx, RemoveCapacityName, as(ben), usecase.Input{"account_id": ana.String(), "from": "2026-11-02"}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("ben removed ana's capacity: %v", err)
	}
	if _, err := doors.Invoke(ctx, RemoveCapacityName, as(ana), usecase.Input{"from": "2026-11-02"}); err != nil || len(r.capacities) != 0 {
		t.Fatalf("removing her capacity: %v", err)
	}
}

// UC-TIM-07 checks 1-4 through the doors: an absence is recorded once by its kind, read by every
// member without a reason, changed and removed by its person only at the version read.
func TestAnAbsenceIsKeptThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	f.withResources()
	doors := resourceDoors(t, f.keeper)
	leave := usecase.Input{"from": "2026-11-09", "to": "2026-11-13", "kind": "LEAVE"}

	if _, err := doors.Invoke(ctx, RecordAbsenceName, as(ana), with(leave, usecase.Input{"to": "Friday"})); detail(err) != "time.day_invalid" {
		t.Fatalf("an absence to no day: %v", err)
	}
	if _, err := doors.Invoke(ctx, RecordAbsenceName, as(ana), with(leave, usecase.Input{"kind": "SICK"})); err == nil {
		t.Fatal("a reason as a kind was recorded")
	}
	if _, err := doors.Invoke(ctx, RecordAbsenceName, as(ben), with(leave, usecase.Input{"account_id": ana.String()})); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("ben records ana's absence: %v", err)
	}
	recorded, err := doors.Invoke(ctx, RecordAbsenceName, as(ana), leave)
	if err != nil || recorded["kind"] != "LEAVE" || recorded["half"] != false {
		t.Fatalf("recording: %v %v", recorded, err)
	}
	if _, err := doors.Invoke(ctx, RecordAbsenceName, as(ana), with(leave, usecase.Input{"from": "2026-11-13", "to": "2026-11-16"})); detail(err) != "time.absence_overlaps" {
		t.Fatalf("an overlap: %v", err)
	}
	seen, err := doors.Invoke(ctx, ListAbsencesName, as(ben), usecase.Input{"from": "2026-11-01", "to": "2026-11-30"})
	if err != nil || len(seen["data"].([]usecase.Output)) != 1 {
		t.Fatalf("the shared calendar: %v %v", seen, err)
	}
	if _, err := doors.Invoke(ctx, ListAbsencesName, as(ben), usecase.Input{"from": "2026-11-01", "to": "2026-11-30", "account_id": "her"}); err == nil {
		t.Fatal("a person who is no id was listed")
	}

	id := recorded.String("id")
	moved := with(leave, usecase.Input{"absence_id": id, "expected_version": float64(1), "from": "2026-11-10", "kind": "TRAINING", "half": true})
	if _, err := doors.Invoke(ctx, UpdateAbsenceName, as(ana), with(moved, usecase.Input{"expected_version": "first"})); detail(err) != "time.version_required" {
		t.Fatalf("a change without a version: %v", err)
	}
	if _, err := doors.Invoke(ctx, UpdateAbsenceName, as(ana), with(moved, usecase.Input{"from": "Tuesday"})); detail(err) != "time.day_invalid" {
		t.Fatalf("a change from no day: %v", err)
	}
	changed, err := doors.Invoke(ctx, UpdateAbsenceName, as(ana), moved)
	if err != nil || changed["kind"] != "TRAINING" || changed["half"] != true || changed.Int("version") != 2 {
		t.Fatalf("changing: %v %v", changed, err)
	}
	if _, err := doors.Invoke(ctx, UpdateAbsenceName, as(ana), moved); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a change on a stale version: %v", err)
	}

	remove := usecase.Input{"absence_id": id, "expected_version": float64(2)}
	if _, err := doors.Invoke(ctx, RemoveAbsenceName, as(ben), remove); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("ben removes ana's absence: %v", err)
	}
	if _, err := doors.Invoke(ctx, RemoveAbsenceName, as(ana), with(remove, usecase.Input{"expected_version": false})); detail(err) != "time.version_required" {
		t.Fatalf("a removal without a version: %v", err)
	}
	if out, err := doors.Invoke(ctx, RemoveAbsenceName, as(ana), remove); err != nil || out["removed"] != true {
		t.Fatalf("removing: %v %v", out, err)
	}
}

// UC-TIM-08 checks 1-5 and 9 through the doors: rates, budgets and billing rates are read and
// written only with MANAGE_COSTS, a change of a rate needs the version read, a budget's burn is
// read back, and no amount reaches the trail.
func TestCostsBudgetsAndBillingThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	r := f.withResources()
	doors := resourceDoors(t, f.keeper)
	rate := usecase.Input{"role": "designer", "hour_minor": float64(6000), "currency": "EUR", "from": "2026-01-01"}

	if _, err := doors.Invoke(ctx, ListCostRatesName, as(ana), usecase.Input{}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member reads the rates: %v", err)
	}
	if _, err := doors.Invoke(ctx, SetCostRateName, as(ana), rate); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member set a rate: %v", err)
	}
	f.gate.approvers[lead] = append(f.gate.approvers[lead], tenantID)
	if _, err := doors.Invoke(ctx, SetCostRateName, as(lead), with(rate, usecase.Input{"to": "end of year"})); detail(err) != "time.day_invalid" {
		t.Fatalf("a rate to no day: %v", err)
	}
	if len(r.rates) != 0 {
		t.Fatal("a refused rate was written")
	}
	set, err := doors.Invoke(ctx, SetCostRateName, as(lead), with(rate, usecase.Input{"to": "2026-12-31"}))
	if err != nil || set["role"] != "designer" || set["to"] != "2026-12-31" || set["account_id"] != nil {
		t.Fatalf("a role's rate: %v %v", set, err)
	}
	change := with(rate, usecase.Input{"rate_id": set.String("id"), "hour_minor": float64(6500)})
	if _, err := doors.Invoke(ctx, SetCostRateName, as(lead), change); detail(err) != "time.version_required" {
		t.Fatalf("a change of a rate without the version read: %v", err)
	}
	if _, err := doors.Invoke(ctx, SetCostRateName, as(lead), with(change, usecase.Input{"expected_version": float64(1)})); err != nil {
		t.Fatalf("changing the rate: %v", err)
	}
	if out, err := doors.Invoke(ctx, ListCostRatesName, as(lead), usecase.Input{}); err != nil || len(out["data"].([]usecase.Output)) != 1 {
		t.Fatalf("the rates: %v %v", out, err)
	}

	budget := usecase.Input{"plan_id": hubID.String(), "minutes": float64(180), "amount_minor": float64(20000), "currency": "EUR", "expected_version": float64(0)}
	if _, err := doors.Invoke(ctx, SetBudgetName, as(ana), budget); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member set the budget: %v", err)
	}
	if _, err := doors.Invoke(ctx, SetBudgetName, as(lead), with(budget, usecase.Input{"expected_version": "new"})); detail(err) != "time.version_required" {
		t.Fatalf("a budget without the version read: %v", err)
	}
	if out, err := doors.Invoke(ctx, SetBudgetName, as(lead), budget); err != nil || out == nil {
		t.Fatalf("setting the budget: %v %v", out, err)
	}
	if _, err := doors.Invoke(ctx, SetBudgetName, as(lead), budget); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale budget: %v", err)
	}
	f.log(t, ana, checkout, "2026-11-02", 60)
	burn, err := doors.Invoke(ctx, ReadBudgetName, as(lead), usecase.Input{"plan_id": hubID.String()})
	if err != nil || burn == nil {
		t.Fatalf("the burn: %v %v", burn, err)
	}
	if _, err := doors.Invoke(ctx, ReadBudgetName, as(ana), usecase.Input{"plan_id": hubID.String()}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member read the burn: %v", err)
	}
	if _, err := doors.Invoke(ctx, RemoveBudgetName, as(lead), usecase.Input{"plan_id": hubID.String(), "expected_version": "one"}); detail(err) != "time.version_required" {
		t.Fatalf("a removal without the version read: %v", err)
	}
	if out, err := doors.Invoke(ctx, RemoveBudgetName, as(lead), usecase.Input{"plan_id": hubID.String(), "expected_version": float64(1)}); err != nil || out["removed"] != true {
		t.Fatalf("removing the budget: %v %v", out, err)
	}

	bill := usecase.Input{"plan_id": hubID.String(), "hour_minor": float64(10000), "currency": "EUR", "from": "2026-01-01"}
	if _, err := doors.Invoke(ctx, SetBillingRateName, as(ana), bill); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member set a billing rate: %v", err)
	}
	billed, err := doors.Invoke(ctx, SetBillingRateName, as(lead), with(bill, usecase.Input{"to": "2026-12-31"}))
	if err != nil || billed["to"] != "2026-12-31" || billed["plan_id"] != hubID.String() {
		t.Fatalf("the hub's billing rate: %v %v", billed, err)
	}
	if _, err := doors.Invoke(ctx, SetBillingRateName, as(lead), with(bill, usecase.Input{"rate_id": billed.String("id")})); detail(err) != "time.version_required" {
		t.Fatalf("a change of a billing rate without the version read: %v", err)
	}
	if _, err := doors.Invoke(ctx, SetBillingRateName, as(lead), with(bill, usecase.Input{"from": "January"})); detail(err) != "time.day_invalid" {
		t.Fatalf("a billing rate from no day: %v", err)
	}
	if out, err := doors.Invoke(ctx, ListBillingRatesName, as(lead), usecase.Input{"plan_id": hubID.String()}); err != nil || len(out["data"].([]usecase.Output)) != 1 {
		t.Fatalf("the hub's billing rates: %v %v", out, err)
	}
	if _, err := doors.Invoke(ctx, ListBillingRatesName, as(ana), usecase.Input{"plan_id": hubID.String()}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member read the billing rates: %v", err)
	}
	if _, err := doors.Invoke(ctx, RemoveBillingRateName, as(lead), usecase.Input{
		"plan_id": hubID.String(), "rate_id": billed.String("id"), "expected_version": nil,
	}); err == nil {
		t.Fatal("a billing rate was removed without the version read")
	}
	if out, err := doors.Invoke(ctx, RemoveBillingRateName, as(lead), usecase.Input{
		"plan_id": hubID.String(), "rate_id": billed.String("id"), "expected_version": float64(1),
	}); err != nil || out["removed"] != true || len(r.billing) != 0 {
		t.Fatalf("removing the billing rate: %v %v", out, err)
	}

	if _, err := doors.Invoke(ctx, RemoveCostRateName, as(lead), usecase.Input{"rate_id": set.String("id"), "expected_version": "two"}); detail(err) != "time.version_required" {
		t.Fatalf("a rate removed without the version read: %v", err)
	}
	if out, err := doors.Invoke(ctx, RemoveCostRateName, as(lead), usecase.Input{"rate_id": set.String("id"), "expected_version": float64(2)}); err != nil || out["removed"] != true {
		t.Fatalf("removing the rate: %v %v", out, err)
	}
	for _, e := range f.audit.entries {
		if text := fmt.Sprint(e.Changes); strings.Contains(text, "6000") || strings.Contains(text, "6500") || strings.Contains(text, "10000") {
			t.Fatalf("a rate's amount reached the trail: %+v", e)
		}
	}
}
