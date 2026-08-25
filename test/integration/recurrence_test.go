// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The statements the series runs on, against a real database (D-04): the round trip, the pointer
// the entry and the rule share, the invariant that keeps them from disagreeing, and the tenant
// boundary per method (gate SG-3).

func recurrenceRepo() postgres.RecurrenceRepository { return postgres.NewRecurrenceRepository() }

func seriesFor(t *testing.T, tenant, item shared.ID, rrule string) work.RecurrenceRule {
	t.Helper()

	due := created.Add(24 * time.Hour)
	dueDate, err := work.NewDueDate(&due, false, "Europe/Berlin")
	if err != nil {
		t.Fatalf("the due date was refused: %v", err)
	}
	rule, err := work.NewRecurrenceRule(work.NewRecurrenceRuleInput{
		ID: freshID(t), TenantID: tenant, ItemID: item,
		Spec: work.RecurrenceSpec{
			RRULE: rrule, TimeZone: "Europe/Berlin",
			Mode: string(work.RecurrenceOnSchedule),
		},
		Due: dueDate, Now: created,
	})
	if err != nil {
		t.Fatalf("the series was refused: %v", err)
	}
	return rule
}

func TestASeriesRoundTripsAndTheEntryPointsAtIt(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	rule := seriesFor(t, tenantA, task, "FREQ=WEEKLY;BYDAY=MO")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return recurrenceRepo().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the series: %v", err)
	}

	var stored work.RecurrenceRule
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = recurrenceRepo().FindForItem(ctx, task)
		return err
	}); err != nil {
		t.Fatalf("reading the series: %v", err)
	}
	if stored.RRULE != "FREQ=WEEKLY;BYDAY=MO" || stored.TimeZone != "Europe/Berlin" ||
		stored.Mode != work.RecurrenceOnSchedule || stored.HorizonDays != 90 ||
		stored.Version != 1 {
		t.Errorf("the series came back as %+v", stored)
	}

	// The entry points at it, which is what lets a reader see that it repeats without a second
	// query - and the read answers the column rather than always null (D-04).
	if item := findWorkItem(ctx, t, tenantA, task); item.RecurrenceRuleID != rule.ID {
		t.Errorf("the entry points at %q rather than at its series", item.RecurrenceRuleID)
	}

	// The whole document, under the lock.
	due := created.Add(24 * time.Hour)
	dueDate, err := work.NewDueDate(&due, false, "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	changed, changes, err := stored.Changed(work.RecurrenceSpec{
		RRULE: "FREQ=WEEKLY;BYDAY=TU", TimeZone: "Europe/Berlin",
		Mode: string(work.RecurrenceOnCompletion), HorizonDays: 30,
	}, dueDate, created.Add(time.Hour))
	if err != nil {
		t.Fatalf("the change was refused: %v", err)
	}
	if len(changes) != 3 {
		t.Errorf("the change reported %v", changes)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return recurrenceRepo().Update(ctx, changed, stored.Version)
	}); err != nil {
		t.Fatalf("writing the change: %v", err)
	}

	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = recurrenceRepo().FindForItem(ctx, task)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if stored.Mode != work.RecurrenceOnCompletion || stored.HorizonDays != 30 ||
		stored.Version != 2 || stored.UpdatedAt == nil {
		t.Errorf("the change left the series as %+v", stored)
	}

	conflict := write(ctx, t, tenantA, func(ctx context.Context) error {
		return recurrenceRepo().Update(ctx, changed, 1)
	})
	if !errors.Is(conflict, shared.ErrVersionConflict) {
		t.Errorf("a stale change answered %v", conflict)
	}

	// Removing takes the row and the pointer, and nothing else.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return recurrenceRepo().Delete(ctx, stored, stored.Version)
	}); err != nil {
		t.Fatalf("removing the series: %v", err)
	}
	err = read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := recurrenceRepo().FindForItem(ctx, task)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the removed series answered %v", err)
	}
	if item := findWorkItem(ctx, t, tenantA, task); !item.RecurrenceRuleID.IsZero() {
		t.Errorf("the entry still points at %q", item.RecurrenceRuleID)
	}
}

// The invariant the two pointers rest on: one series per entry. Without it, which rule an entry
// repeats by would be whichever pointer was read.
func TestAnEntryCarriesAtMostOneSeries(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return recurrenceRepo().Insert(ctx, seriesFor(t, tenantA, task, "FREQ=DAILY"))
	}); err != nil {
		t.Fatalf("writing the series: %v", err)
	}

	second := write(ctx, t, tenantA, func(ctx context.Context) error {
		return recurrenceRepo().Insert(ctx, seriesFor(t, tenantA, task, "FREQ=WEEKLY"))
	})
	if second == nil {
		t.Fatal("an entry took a second series")
	}
}

// Purging the entry takes its series with it, which is the deletion path the schema promises.
func TestPurgingTheEntryTakesItsSeries(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	rule := seriesFor(t, tenantA, task, "FREQ=DAILY")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return recurrenceRepo().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the series: %v", err)
	}

	if _, err := adminPool(ctx, t).Exec(ctx,
		`DELETE FROM work_item WHERE id = $1`, task.String()); err != nil {
		t.Fatalf("purging the entry: %v", err)
	}

	var remaining int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM recurrence_rule WHERE id = $1`, rule.ID.String()).
		Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Error("the series outlived the entry it repeated")
	}
}

// Gate SG-3: one negative per port method.
func TestASeriesIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	rule := seriesFor(t, tenantA, task, "FREQ=DAILY")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return recurrenceRepo().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("seeding the series: %v", err)
	}

	t.Run("find", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := recurrenceRepo().FindForItem(ctx, task)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("tenant B's find answered %v", err)
		}
	})

	t.Run("insert", func(t *testing.T) {
		// The insert writes current_tenant_id() and never the caller's claim, so a row naming
		// tenant A's entry finds no such entry in tenant B and the composite key refuses it.
		foreign := seriesFor(t, tenantA, task, "FREQ=MONTHLY")
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return recurrenceRepo().Insert(ctx, foreign)
		}); err == nil {
			t.Error("tenant B wrote a series on tenant A's entry")
		}
	})

	t.Run("update", func(t *testing.T) {
		due := created.Add(24 * time.Hour)
		dueDate, err := work.NewDueDate(&due, false, "Europe/Berlin")
		if err != nil {
			t.Fatal(err)
		}
		changed, _, err := rule.Changed(work.RecurrenceSpec{
			RRULE: "FREQ=YEARLY", TimeZone: "Europe/Berlin",
			Mode: string(work.RecurrenceOnSchedule),
		}, dueDate, created.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		writeErr := write(ctx, t, tenantB, func(ctx context.Context) error {
			return recurrenceRepo().Update(ctx, changed, rule.Version)
		})
		if !errors.Is(writeErr, shared.ErrVersionConflict) {
			t.Fatalf("tenant B's change answered %v", writeErr)
		}
	})

	t.Run("delete", func(t *testing.T) {
		writeErr := write(ctx, t, tenantB, func(ctx context.Context) error {
			return recurrenceRepo().Delete(ctx, rule, rule.Version)
		})
		if !errors.Is(writeErr, shared.ErrVersionConflict) {
			t.Fatalf("tenant B's removal answered %v", writeErr)
		}

		// And tenant A's entry still points at its series: nothing was cleared by the attempt.
		if item := findWorkItem(ctx, t, tenantA, task); item.RecurrenceRuleID != rule.ID {
			t.Errorf("tenant A's entry now points at %q", item.RecurrenceRuleID)
		}
	})
}
