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

// SetAttributes against a real database (B-05), with the cross-tenant negative gate SG-3 asks of
// every new repository method.

// writableProfile is the profile of a task as far as these tests need it: NOTES is the capability
// under test, and the domain refuses notes without it.
func writableProfile() work.CapabilityProfile {
	return work.CapabilityProfile{
		Type:         work.ItemTask,
		Capabilities: []work.Capability{work.CapabilityCompletion, work.CapabilityNotes},
		MaxDepth:     3,
	}
}

// pointerTo spells "the caller sent this field", which is what ItemAttributes distinguishes from
// an absent one.
func pointerTo(value string) *string { return &value }

func updatedAt(hours int) time.Time { return created.Add(time.Duration(hours) * time.Hour) }

// The whole of what an update writes: both columns, and one version.
func TestSetAttributesWritesTheFieldsAndBumpsTheVersion(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	id := seedTask(ctx, t, tenantA, authorA, collection)

	before := findItem(ctx, t, tenantA, id)
	renamed, changes, err := before.Updated(
		work.ItemAttributes{Title: pointerTo("Buy oat milk"), Notes: pointerTo("Whole, not semi.")},
		writableProfile(), updatedAt(1))
	if err != nil {
		t.Fatalf("applying the update: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("%d fields changed rather than two", len(changes))
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetAttributes(ctx, renamed, before.Version)
	}); err != nil {
		t.Fatalf("writing the attributes: %v", err)
	}

	after := findItem(ctx, t, tenantA, id)
	if after.Title != "Buy oat milk" {
		t.Errorf("the stored title is %q", after.Title)
	}
	if after.Notes != "Whole, not semi." {
		t.Errorf("the stored notes are %q", after.Notes)
	}
	if after.Version != before.Version+1 {
		t.Errorf("the version is %d, want %d", after.Version, before.Version+1)
	}
	if !after.UpdatedAt.Equal(updatedAt(1)) {
		t.Errorf("updated_at is %s, want %s", after.UpdatedAt, updatedAt(1))
	}
}

// Clearing the notes has to reach the column as NULL. The statement writes both fields on every
// call for exactly this reason: a COALESCE would make "no notes" indistinguishable from "leave
// them alone", and a client that cleared them would keep getting the old text back.
func TestSetAttributesClearsTheNotes(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	id := seedTask(ctx, t, tenantA, authorA, collection)

	noted := findItem(ctx, t, tenantA, id)
	withNotes, _, err := noted.Updated(
		work.ItemAttributes{Notes: pointerTo("Three metres.")}, writableProfile(), updatedAt(1))
	if err != nil {
		t.Fatalf("applying the notes: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetAttributes(ctx, withNotes, noted.Version)
	}); err != nil {
		t.Fatalf("writing the notes: %v", err)
	}

	stored := findItem(ctx, t, tenantA, id)
	cleared, changes, err := stored.Updated(
		work.ItemAttributes{Notes: pointerTo("")}, writableProfile(), updatedAt(2))
	if err != nil {
		t.Fatalf("clearing the notes: %v", err)
	}
	if len(changes) != 1 || changes[0].Field != work.FieldNotes {
		t.Fatalf("clearing reported %v", changes)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetAttributes(ctx, cleared, stored.Version)
	}); err != nil {
		t.Fatalf("writing the cleared notes: %v", err)
	}

	if after := findItem(ctx, t, tenantA, id); after.Notes != "" {
		t.Errorf("the notes are still %q", after.Notes)
	}
}

// The lost update this exists to prevent: two callers read version 1, and the second one to write
// is told rather than silently overwriting the first (api-guidelines.md §5).
func TestSetAttributesRefusesAStaleVersion(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	id := seedTask(ctx, t, tenantA, authorA, collection)

	item := findItem(ctx, t, tenantA, id)
	first, _, err := item.Updated(work.ItemAttributes{Title: pointerTo("Buy oat milk")}, writableProfile(), updatedAt(1))
	if err != nil {
		t.Fatalf("applying the first update: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetAttributes(ctx, first, item.Version)
	}); err != nil {
		t.Fatalf("the first write: %v", err)
	}

	second, _, err := item.Updated(work.ItemAttributes{Title: pointerTo("Buy soy milk")}, writableProfile(), updatedAt(2))
	if err != nil {
		t.Fatalf("applying the second update: %v", err)
	}
	err = write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetAttributes(ctx, second, item.Version)
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale version answered %v, want a version conflict", err)
	}

	// Nothing was written: the loser of the race leaves the winner's title in place.
	if after := findItem(ctx, t, tenantA, id); after.Title != "Buy oat milk" {
		t.Errorf("the stale write landed anyway: the title is %q", after.Title)
	}
}

// Gate SG-3. A row another tenant owns is out of the update's reach, and the answer is the one a
// moved version gives - a caller must not be able to tell the two apart (multi-tenancy.md §2).
func TestSetAttributesCannotReachAnotherTenant(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	id := seedTask(ctx, t, tenantA, authorA, collection)

	item := findItem(ctx, t, tenantA, id)
	renamed, _, err := item.Updated(work.ItemAttributes{Title: pointerTo("Renamed by B")}, writableProfile(), updatedAt(1))
	if err != nil {
		t.Fatalf("applying the update: %v", err)
	}

	err = write(ctx, t, tenantB, func(ctx context.Context) error {
		return itemRepo().SetAttributes(ctx, renamed, item.Version)
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("writing across the boundary answered %v", err)
	}

	if after := findItem(ctx, t, tenantA, id); after.Title != item.Title {
		t.Errorf("tenant B renamed tenant A's item to %q", after.Title)
	}
}
