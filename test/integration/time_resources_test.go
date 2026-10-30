// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A capacity, an absence, a cost rate and a budget stay in their workspace (migration 0247,
// UC-TIM-06 check 5, UC-TIM-07 check 5, UC-TIM-08 check 6).
func TestTimeResourcesStayInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewTimeResourceRepository()
	_, collection, _ := seedPlan(ctx, t, tenantA)
	now := time.Date(2026, 11, 3, 17, 0, 0, 0, time.UTC)
	day, _ := timekeeping.ParseDay("2026-11-02")
	absence := timekeeping.Absence{ID: freshID(t), AccountID: authorA, From: day + 7, To: day + 11, Kind: timekeeping.AbsenceLeave}
	rate := timekeeping.CostRate{ID: freshID(t), AccountID: authorA, HourMinor: 8000, Currency: "EUR", From: day}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		capacity := timekeeping.Capacity{AccountID: authorA, From: day, WeekMinutes: [7]int{480, 480, 480, 480, 240, 0, 0}}
		if err := store.PutCapacity(ctx, freshID(t), capacity, authorA, now); err != nil {
			return err
		}
		if err := store.PutAbsence(ctx, absence, 0, authorA, now); err != nil {
			return err
		}
		if err := store.PutRate(ctx, rate, 0, authorA, now); err != nil {
			return err
		}
		return store.PutBudget(ctx, freshID(t), timekeeping.Budget{PlanID: collection, Minutes: 6000, AmountMinor: 800000, Currency: "EUR"}, 0, authorA, now)
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		capacities, err := store.Capacities(ctx, []shared.ID{authorA}, 10)
		if err != nil || len(capacities) != 1 || capacities[0].WeekMinutes[4] != 240 {
			t.Errorf("capacities = %+v %v", capacities, err)
		}
		absences, err := store.Absences(ctx, "", day, day+30, 10)
		if err != nil || len(absences) != 1 || absences[0].Kind != timekeeping.AbsenceLeave {
			t.Errorf("absences = %+v %v", absences, err)
		}
		rates, err := store.Rates(ctx, 10)
		if err != nil || len(rates) != 1 || rates[0].HourMinor != 8000 || rates[0].To != nil {
			t.Errorf("rates = %+v %v", rates, err)
		}
		budget, held, err := store.Budget(ctx, collection)
		if err != nil || !held || budget.AmountMinor != 800000 {
			t.Errorf("budget = %+v %v %v", budget, held, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if c, err := store.Capacities(ctx, nil, 10); err != nil || len(c) != 0 {
			t.Errorf("the other workspace reads capacities: %v %v", c, err)
		}
		if a, err := store.Absences(ctx, "", day, day+30, 10); err != nil || len(a) != 0 {
			t.Errorf("the other workspace reads absences: %v %v", a, err)
		}
		if _, err := store.FindAbsence(ctx, absence.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace finds the absence: %v", err)
		}
		if _, err := store.FindRate(ctx, rate.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace finds the rate: %v", err)
		}
		if _, held, err := store.Budget(ctx, collection); err != nil || held {
			t.Errorf("the other workspace reads the budget: %v %v", held, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return store.RemoveRate(ctx, rate.ID, 1)
	}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("the other workspace removed the rate: %v", err)
	}
}
