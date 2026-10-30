// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/timekeeping"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Time records, a timer, a week and the time policy stay in their workspace (migration 0246,
// UC-TIM-01 check 6, UC-TIM-02 check 6); one timer runs per person, a record summed reaches its
// entry's collection.
func TestTimeStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewTimeRepository()
	_, collection, tasks := seedPlan(ctx, t, tenantA)
	now := time.Date(2026, 11, 3, 17, 0, 0, 0, time.UTC)
	day, _ := timekeeping.ParseDay("2026-11-03")
	record := timekeeping.Record{
		ID: freshID(t), AccountID: authorA, EntryID: tasks[0], WorkType: "development", Day: day,
		Minutes: 90, Note: "Checkout form", Billable: true, Source: timekeeping.SourceManual,
		CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	locked := day - 7
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := store.PutWorkType(ctx, freshID(t), timekeeping.WorkType{Key: "development", Name: "Development", Billable: true}, now); err != nil {
			return err
		}
		if err := store.InsertRecord(ctx, record); err != nil {
			return err
		}
		if err := store.StartTimer(ctx, freshID(t), timekeeping.Timer{AccountID: authorA, StartedAt: now}); err != nil {
			return err
		}
		sheet, _ := timekeeping.OpenSheet(authorA, day).Submit(90, now)
		if err := store.PutSheet(ctx, freshID(t), sheet, 0, now); err != nil {
			return err
		}
		return store.PutPolicy(ctx, freshID(t), timekeeping.Policy{
			LockedThrough: &locked, Visibility: timekeeping.VisibilityAggregated, UpdatedAt: now, UpdatedBy: authorA,
		}, 0)
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return store.StartTimer(ctx, freshID(t), timekeeping.Timer{AccountID: authorA, StartedAt: now})
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second timer ran: %v", err)
	}
	week := repository.Filter{From: day - 1, To: day + 5}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		placed, err := store.Records(ctx, week, 10)
		if err != nil {
			return err
		}
		if len(placed) != 1 || placed[0].CollectionID != collection || placed[0].Note != "Checkout form" {
			t.Errorf("records = %+v", placed)
		}
		lines, err := store.Lines(ctx, repository.Filter{From: day, To: day, CollectionID: collection}, 10)
		if err == nil && (len(lines) != 1 || lines[0].Minutes != 90 || lines[0].EntryID != tasks[0]) {
			t.Errorf("lines = %+v", lines)
		}
		on, err := store.MinutesOn(ctx, tasks)
		if err == nil && on[tasks[0]] != 90 {
			t.Errorf("minutes on = %+v", on)
		}
		sheet, held, err := store.Sheet(ctx, authorA, day)
		if err == nil && (!held || sheet.State != timekeeping.SheetSubmitted || sheet.Minutes != 90) {
			t.Errorf("sheet = %+v %v", sheet, held)
		}
		policy, err := store.Policy(ctx)
		if err == nil && (policy.Visibility != timekeeping.VisibilityAggregated || *policy.LockedThrough != locked) {
			t.Errorf("policy = %+v", policy)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := store.FindRecord(ctx, record.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace reads the record: %v", err)
		}
		if placed, err := store.Records(ctx, week, 10); err != nil || len(placed) != 0 {
			t.Errorf("the other workspace lists records: %v %v", placed, err)
		}
		if on, err := store.MinutesOn(ctx, tasks); err != nil || len(on) != 0 {
			t.Errorf("the other workspace sums the entries: %v %v", on, err)
		}
		if _, running, err := store.Timer(ctx, authorA); err != nil || running {
			t.Errorf("the other workspace reads the timer: %v %v", running, err)
		}
		if _, held, err := store.Sheet(ctx, authorA, day); err != nil || held {
			t.Errorf("the other workspace reads the week: %v %v", held, err)
		}
		if policy, err := store.Policy(ctx); err != nil || policy.LockedThrough != nil {
			t.Errorf("the other workspace reads the policy: %+v %v", policy, err)
		}
		if kinds, err := store.WorkTypes(ctx); err != nil || len(kinds) != 0 {
			t.Errorf("the other workspace reads the kinds of work: %v %v", kinds, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return store.DeleteRecord(ctx, record.ID, 1)
	}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("the other workspace removed the record: %v", err)
	}
}
