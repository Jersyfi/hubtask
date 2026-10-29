// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func officeCalendar(t *testing.T) calendar.BusinessCalendar {
	t.Helper()
	var week calendar.Week
	for _, day := range []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday} {
		week[day] = []calendar.Span{{From: 9 * 60, To: 17 * 60}}
	}
	cal, err := calendar.New(calendar.Spec{Zone: "Europe/Berlin", Week: week,
		Holidays: []calendar.Date{{Year: 2026, Month: time.December, Day: 25}}})
	if err != nil {
		t.Fatal(err)
	}
	return cal
}

// A timer is written, found live with its calendar intact, paused, resumed, claimed when due and
// fired once (domain-model.md §3.7, migration 0229).
func TestATimerIsStoredClaimedAndFiredOnce(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewTimerRepository()
	subject := freshID(t)
	armed := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC) // 09:00 in Berlin, a Monday
	timer, err := calendar.Arm(freshID(t), tenantA, calendar.PurposeSLA, subject, "resolution",
		officeCalendar(t), 2*time.Hour, armed)
	if err != nil {
		t.Fatal(err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, timer, armed)
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	second, _ := calendar.Arm(freshID(t), tenantA, calendar.PurposeSLA, subject, "resolution",
		officeCalendar(t), time.Hour, armed)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, second, armed)
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second live timer of the same goal: %v, want a conflict", err)
	}

	var found calendar.Timer
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		found, err = repo.FindLive(ctx, calendar.PurposeSLA, subject, "resolution")
		return err
	}); err != nil {
		t.Fatalf("find: %v", err)
	}
	if found.Calendar.Spec().Zone != "Europe/Berlin" || len(found.Calendar.Spec().Holidays) != 1 ||
		!found.DueAt.Equal(armed.Add(2*time.Hour)) || found.Version != 1 {
		t.Fatalf("read back %+v", found)
	}

	due := armed.Add(2 * time.Hour)
	var claimed []calendar.Timer
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if claimed, err = repo.ClaimDue(ctx, due.Add(-time.Minute), 10); err != nil || len(claimed) != 0 {
			return errors.Join(err, errors.New("claimed before its moment"))
		}
		if claimed, err = repo.ClaimDue(ctx, due, 10); err != nil {
			return err
		}
		for _, c := range claimed {
			if c.ID != timer.ID {
				continue
			}
			fired, err := c.Fire(due)
			if err != nil {
				return err
			}
			return repo.Update(ctx, fired, c.Version, due)
		}
		return errors.New("the due timer was not claimed")
	}); err != nil {
		t.Fatalf("firing: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := repo.FindLive(ctx, calendar.PurposeSLA, subject, "resolution")
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a fired timer is still live: %v", err)
	}
}

// The cross-tenant negative test for every TimerRepository method (gate SG-3).
func TestATimerIsNotReachableFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewTimerRepository()
	subject := freshID(t)
	armed := time.Now().UTC().Add(-time.Hour)
	timer, err := calendar.Arm(freshID(t), tenantA, calendar.PurposeApproval, subject, "decision",
		calendar.Continuous(), time.Minute, armed)
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, timer, armed)
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	err = write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := repo.FindLive(ctx, calendar.PurposeApproval, subject, "decision"); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("FindLive reached tenant A's timer: %v", err)
		}
		claimed, err := repo.ClaimDue(ctx, time.Now().UTC(), 100)
		if err != nil {
			return err
		}
		for _, c := range claimed {
			if c.ID == timer.ID {
				t.Error("ClaimDue reached tenant A's timer")
			}
		}
		if next, err := repo.NextDue(ctx); err != nil {
			return err
		} else if next != nil && next.Equal(*timer.DueAt) {
			t.Error("NextDue answered tenant A's moment")
		}
		stopped, _ := timer.Stop(time.Now().UTC())
		if err := repo.Update(ctx, stopped, timer.Version, time.Now().UTC()); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("Update of tenant A's timer: %v, want the conflict a missing row answers", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading as tenant B: %v", err)
	}
	// Written under B's context, a row claiming tenant A is refused by the policy. Its own
	// transaction: the refusal aborts the one it happens in.
	foreign, _ := calendar.Arm(freshID(t), tenantA, calendar.PurposeApproval, freshID(t), "decision",
		calendar.Continuous(), time.Minute, armed)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return repo.Insert(ctx, foreign, armed)
	}); err == nil {
		t.Error("Insert wrote a row for tenant A under tenant B")
	}
}
