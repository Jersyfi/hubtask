// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The cross-tenant suite for the items repository. Every method gets a negative test, because the
// tenant boundary is not something the code asserts - it is row level security underneath, and the
// only way to know it is switched on for a new table is to try to reach across it (gate SG-3).

// collectionFor gives a tenant a hub and a collection to put items in, written through the
// container repository so that the fixture goes through the same boundary the items do.
func collectionFor(ctx context.Context, t *testing.T, tenant, author shared.ID) shared.ID {
	t.Helper()
	seedContainerTenants(ctx, t)

	hubID, collectionID := freshID(t), freshID(t)
	containers := postgres.NewContainerRepository()

	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		hub := containerIn(tenant, author, hubID, freshName(t), "a0")
		if err := containers.Insert(ctx, hub); err != nil {
			return err
		}
		collection := containerIn(tenant, author, collectionID, freshName(t), "a0")
		collection.Type = work.ContainerCollection
		collection.ParentID = hubID
		return containers.Insert(ctx, collection)
	}); err != nil {
		t.Fatalf("seeding the collection: %v", err)
	}
	return collectionID
}

func taskIn(tenant, author, collection, id shared.ID, title, orderKey string) work.WorkItem {
	return work.WorkItem{
		ID: id, TenantID: tenant, CollectionID: collection, Type: work.ItemTask,
		Path: work.RootPath(id), Depth: 1, Title: title, OrderKey: orderKey,
		CreatedBy: author, CreatedAt: created, UpdatedAt: created, Version: 1,
	}
}

func TestAnItemIsWrittenAndReadBack(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	repo := postgres.NewItemRepository()

	id := freshID(t)
	task := taskIn(tenantA, authorA, collection, id, "Buy milk", "a0")
	task.Notes = "Semi-skimmed, two litres"

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, task)
	}); err != nil {
		t.Fatalf("writing the task: %v", err)
	}

	var stored work.WorkItem
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = repo.Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the task: %v", err)
	}

	if stored.Title != "Buy milk" || stored.Type != work.ItemTask || stored.Version != 1 {
		t.Errorf("unexpected item: %+v", stored)
	}
	if stored.CollectionID != collection || !stored.ParentID.IsZero() {
		t.Errorf("the placement did not survive the round trip: %+v", stored)
	}
	if stored.Path != work.RootPath(id) || stored.Depth != 1 {
		t.Errorf("path = %q, depth = %d", stored.Path, stored.Depth)
	}
	if stored.Notes != "Semi-skimmed, two litres" {
		t.Errorf("notes = %q", stored.Notes)
	}
	// An item is written open, and the column pair has to agree: the CHECK constraint refuses a
	// completed item without a timestamp, so this proves the mapping writes neither by accident.
	if stored.Completion.IsCompleted || stored.Completion.CompletedAt != nil {
		t.Errorf("completion = %+v, want open", stored.Completion)
	}
	if stored.IsArchived() || stored.IsTrashed() {
		t.Error("a new item is archived or trashed")
	}
}

// A subtree survives the round trip, and the paths still nest - which is what every later subtree
// query rests on.
func TestASubtreeIsWrittenAtEveryLevel(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	repo := postgres.NewItemRepository()

	taskID, packageID, activityID := freshID(t), freshID(t), freshID(t)
	task := taskIn(tenantA, authorA, collection, taskID, "Weekly shop", "a0")
	pkg := work.WorkItem{
		ID: packageID, TenantID: tenantA, CollectionID: collection, Type: work.ItemWorkPackage,
		ParentID: taskID, Path: task.ChildPath(packageID), Depth: 2, Title: "Dairy aisle",
		OrderKey: "a0", CreatedBy: authorA, CreatedAt: created, UpdatedAt: created, Version: 1,
	}
	activity := work.WorkItem{
		ID: activityID, TenantID: tenantA, CollectionID: collection, Type: work.ItemActivity,
		ParentID: packageID, Path: pkg.ChildPath(activityID), Depth: 3, Title: "Milk",
		OrderKey: "a0", CreatedBy: authorA, CreatedAt: created, UpdatedAt: created, Version: 1,
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, item := range []work.WorkItem{task, pkg, activity} {
			if err := repo.Insert(ctx, item); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("writing the subtree: %v", err)
	}

	// The prefix query is what the materialised path exists for: everything at or below the task,
	// in one index scan rather than a recursive walk.
	if rows := countIn(ctx, t,
		`SELECT count(*) FROM work_item WHERE tenant_id = $1 AND path LIKE $2 || '%'`,
		tenantA.String(), task.Path); rows != 3 {
		t.Errorf("%d items under the task, want 3", rows)
	}
}

// The cross-tenant negative test for Find.
func TestAnItemIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	repo := postgres.NewItemRepository()

	id := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, taskIn(tenantA, authorA, collection, id, "Buy milk", "a0"))
	}); err != nil {
		t.Fatalf("writing the task: %v", err)
	}

	err := read(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := repo.Find(ctx, id)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("tenant B read tenant A's item: %v", err)
	}
}

// The cross-tenant negative test for Insert: the tenant comes from the transaction and not from
// the object, so an item built for another tenant lands in the caller's own - it cannot be
// smuggled across the boundary.
//
// What this deliberately does not claim is that the write fails. It does not: the collection is a
// plain foreign key, and a foreign key is checked without regard to row level security, so a row
// may be written that points at a collection its own tenant cannot see. The row is still tenant
// B's and tenant A cannot read it, which is what the boundary guarantees - but the reference
// dangles, and no adapter in this schema enforces otherwise. Enforcing it belongs to the schema
// as a whole rather than to this one table.
func TestInsertCannotWriteAnItemIntoAnotherTenant(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	repo := postgres.NewItemRepository()
	smuggled := freshID(t)

	// The object claims tenant A, the transaction belongs to tenant B.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return repo.Insert(ctx, taskIn(tenantA, authorB, collection, smuggled, "Buy milk", "a0"))
	}); err != nil {
		t.Fatalf("writing the task: %v", err)
	}

	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := repo.Find(ctx, smuggled)
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the row landed in tenant A although the transaction was tenant B's: %v", err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := repo.Find(ctx, smuggled)
		return err
	}); err != nil {
		t.Fatalf("the row is not in the tenant that wrote it: %v", err)
	}
}

// The cross-tenant negative test for LastOrderKey: ranks are counted per tenant, so a busy
// neighbour cannot push an item to the end of somebody else's list.
func TestTheLastItemOrderKeyIsPerTenant(t *testing.T) {
	ctx := context.Background()
	collectionA := collectionFor(ctx, t, tenantA, authorA)
	collectionB := collectionFor(ctx, t, tenantB, authorB)
	repo := postgres.NewItemRepository()

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, taskIn(tenantA, authorA, collectionA, freshID(t), "Buy milk", "z9"))
	}); err != nil {
		t.Fatalf("writing tenant A's task: %v", err)
	}

	var key string
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		key, err = repo.LastOrderKey(ctx, collectionB, "")
		return err
	}); err != nil {
		t.Fatalf("reading tenant B's last rank: %v", err)
	}
	if key != "" {
		t.Errorf("tenant B sees %q, which can only have come from tenant A", key)
	}
}

// The sibling level is the pair, not the parent alone: items under the same parent in one
// collection rank against each other, and the level directly under a collection is its own.
func TestTheSiblingLevelIsTheCollectionAndTheParentTogether(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	repo := postgres.NewItemRepository()

	taskID := freshID(t)
	task := taskIn(tenantA, authorA, collection, taskID, "Weekly shop", "a0")
	child := work.WorkItem{
		ID: freshID(t), TenantID: tenantA, CollectionID: collection, Type: work.ItemWorkPackage,
		ParentID: taskID, Path: task.ChildPath(freshID(t)), Depth: 2, Title: "Dairy aisle",
		OrderKey: "a7", CreatedBy: authorA, CreatedAt: created, UpdatedAt: created, Version: 1,
	}
	child.Path = task.ChildPath(child.ID)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.Insert(ctx, task); err != nil {
			return err
		}
		return repo.Insert(ctx, child)
	}); err != nil {
		t.Fatalf("writing the items: %v", err)
	}

	var topLevel, underTask string
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if topLevel, err = repo.LastOrderKey(ctx, collection, ""); err != nil {
			return err
		}
		underTask, err = repo.LastOrderKey(ctx, collection, taskID)
		return err
	}); err != nil {
		t.Fatalf("reading the ranks: %v", err)
	}

	if topLevel != "a0" {
		t.Errorf("the top level sees %q, want the task's own rank", topLevel)
	}
	if underTask != "a7" {
		t.Errorf("under the task sees %q, want the child's rank", underTask)
	}
}

func TestAnEmptyCollectionHasNoLastItemOrderKey(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)

	var key string
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		key, err = postgres.NewItemRepository().LastOrderKey(ctx, collection, "")
		return err
	}); err != nil {
		t.Fatalf("an empty collection is reported as an error: %v", err)
	}
	if key != "" {
		t.Errorf("an empty collection answered %q rather than nothing", key)
	}
}

// The title is normalised in the database, so two spellings of the same word are one value to
// every query that compares or searches them (I-W7).
func TestTitlesAreNormalisedOnTheWayIn(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	repo := postgres.NewItemRepository()

	composed := "\u00dcbung"    // "Übung" with Ü as one code point
	decomposed := "U\u0308bung" // the same word with a combining diaeresis

	id := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, taskIn(tenantA, authorA, collection, id, decomposed, "a0"))
	}); err != nil {
		t.Fatalf("writing the task: %v", err)
	}

	var stored work.WorkItem
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = repo.Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the task: %v", err)
	}
	if stored.Title != composed {
		t.Errorf("title = %q, want it normalised to %q", stored.Title, composed)
	}
}
