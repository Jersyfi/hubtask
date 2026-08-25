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

// TestTwoDevicesMovingDateAndZoneConvergeToBoth is the acceptance sentence about the merge rule,
// at the level this milestone owns: the trio merges as scalars per field (offline-sync.md §4.2),
// and the version predicate is what turns a concurrent write into a re-read and a retry rather
// than an overwrite. Device A moves the date, device B the zone; B is told the row moved, re-reads
// - the merge onto what A left - and nothing of either device's field is lost.
func TestTwoDevicesMovingDateAndZoneConvergeToBoth(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	id := seedTask(ctx, t, tenantA, authorA, collection)

	start := findWorkItem(ctx, t, tenantA, id)
	start.Due = &work.DueDate{At: created.Add(24 * time.Hour), TimeZone: "Europe/Berlin"}
	start.UpdatedAt = created.Add(time.Hour)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, start, start.Version)
	}); err != nil {
		t.Fatalf("seeding the due date: %v", err)
	}

	// Both devices read the same state.
	seen := findWorkItem(ctx, t, tenantA, id)

	// Device A moves the date and lands first.
	fromA := seen
	fromA.Due = &work.DueDate{At: created.Add(96 * time.Hour), TimeZone: seen.Due.TimeZone}
	fromA.UpdatedAt = created.Add(2 * time.Hour)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, fromA, seen.Version)
	}); err != nil {
		t.Fatalf("device A: %v", err)
	}

	// Device B moves the zone against the state it read, and the version is what catches it.
	fromB := seen
	fromB.Due = &work.DueDate{At: seen.Due.At, TimeZone: "America/Sao_Paulo"}
	fromB.UpdatedAt = created.Add(2 * time.Hour)
	staleErr := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, fromB, seen.Version)
	})
	if !errors.Is(staleErr, shared.ErrVersionConflict) {
		t.Fatalf("the concurrent write answered %v, want a version conflict", staleErr)
	}

	// B re-reads and retries with its one field on top of what is now there - the per-field
	// merge, which is the whole client protocol.
	current := findWorkItem(ctx, t, tenantA, id)
	retryB := current
	retryB.Due = &work.DueDate{At: current.Due.At, TimeZone: "America/Sao_Paulo"}
	retryB.UpdatedAt = created.Add(3 * time.Hour)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, retryB, current.Version)
	}); err != nil {
		t.Fatalf("device B's retry: %v", err)
	}

	converged := findWorkItem(ctx, t, tenantA, id)
	if converged.Due == nil || !converged.Due.At.Equal(created.Add(96*time.Hour)) ||
		converged.Due.TimeZone != "America/Sao_Paulo" {
		t.Fatalf("the trio converged to %+v", converged.Due)
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
