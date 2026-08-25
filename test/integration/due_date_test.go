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
)

// The statement the due date pair writes with, against a real database (D-01): the trio round
// trips whole, the constraint holds the qualifiers to their date, and the tenant boundary answers
// a foreign write the way it answers every other (gate SG-3).

func TestTheDueDateRoundTripsWhole(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)

	id := freshID(t)
	task := taskIn(tenantA, authorA, collection, id, freshName(t), "a0")
	startAt := created.Add(30 * time.Minute)
	task.StartAt = &startAt
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, task)
	}); err != nil {
		t.Fatalf("writing the task: %v", err)
	}

	// The insert carries the start, and no due date: the due date arrives through its writer.
	item := findWorkItem(ctx, t, tenantA, id)
	if item.StartAt == nil || !item.StartAt.Equal(startAt) {
		t.Fatalf("the start came back as %v", item.StartAt)
	}
	if item.Due != nil {
		t.Fatalf("a fresh entry carries a due date: %+v", item.Due)
	}

	item.Due = &work.DueDate{
		At: created.Add(48 * time.Hour), DateOnly: true, TimeZone: "Europe/Berlin",
	}
	item.UpdatedAt = created.Add(time.Hour)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, item, item.Version)
	}); err != nil {
		t.Fatalf("setting the due date: %v", err)
	}

	stored := findWorkItem(ctx, t, tenantA, id)
	if !stored.Due.Equal(item.Due) {
		t.Fatalf("the due date came back as %+v, want %+v", stored.Due, item.Due)
	}
	if stored.Version != item.Version+1 {
		t.Errorf("the write left the version at %d", stored.Version)
	}

	// Clearing removes the trio whole.
	stored.Due = nil
	stored.UpdatedAt = created.Add(2 * time.Hour)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, stored, stored.Version)
	}); err != nil {
		t.Fatalf("clearing the due date: %v", err)
	}
	if bare := findWorkItem(ctx, t, tenantA, id); bare.Due != nil {
		t.Fatalf("the due date survived its clearing: %+v", bare.Due)
	}
}

// Migration 0023: the qualifiers cannot outlive their date, whatever writes the row. The
// application refuses the combination at construction; the constraint is what makes the sentence
// true of the table rather than of one writer.
func TestTheDatabaseRefusesAQualifierWithoutItsDate(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	id := seedTask(ctx, t, tenantA, authorA, collection)

	if _, err := adminPool(ctx, t).Exec(ctx,
		"UPDATE work_item SET due_time_zone = 'Europe/Berlin' WHERE id = $1",
		id.String()); err == nil {
		t.Error("the database stored a time zone without a due date")
	}
	if _, err := adminPool(ctx, t).Exec(ctx,
		"UPDATE work_item SET due_date_only = true WHERE id = $1",
		id.String()); err == nil {
		t.Error("the database stored an all-day flag without a due date")
	}
}

// Gate SG-3 for the new statement.
func TestADueDateCannotBeWrittenAcrossTheTenantBoundary(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	id := seedTask(ctx, t, tenantA, authorA, collection)

	item := findWorkItem(ctx, t, tenantA, id)
	item.Due = &work.DueDate{At: created.Add(24 * time.Hour)}
	item.UpdatedAt = created.Add(time.Hour)

	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, item, item.Version)
	})
	// Row level security removed the row from the update's reach, and a caller must not be able
	// to tell that apart from a version that moved (multi-tenancy.md §2).
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("tenant B's due date write answered %v", err)
	}
	if stored := findWorkItem(ctx, t, tenantA, id); stored.Due != nil {
		t.Error("tenant B put a due date on tenant A's entry")
	}
}
