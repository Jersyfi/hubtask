// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// An entry's amount (UC-WRK-25 check 2) as the row holds it: written by the create, the update and
// the copy, read by every read, carried by a restore - and an archive written before the column
// existed restores to none.

func TestAnAmountIsWrittenChangedAndCopied(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)

	task := taskIn(tenantA, authorA, collection, freshID(t), "milk", "a0")
	task.Amount = "2"
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, task)
	}); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if got := findItem(ctx, t, tenantA, task.ID).Amount; got != "2" {
		t.Fatalf("amount after the insert = %q", got)
	}

	changed := findItem(ctx, t, tenantA, task.ID)
	changed.Amount = "1 kg"
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetAttributes(ctx, changed, changed.Version)
	}); err != nil {
		t.Fatalf("changing: %v", err)
	}
	if got := findItem(ctx, t, tenantA, task.ID).Amount; got != "1 kg" {
		t.Errorf("amount after the update = %q", got)
	}

	copied := findItem(ctx, t, tenantA, task.ID)
	copied.ID = freshID(t)
	copied.Path = work.RootPath(copied.ID)
	copied.OrderKey = "a1"
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().InsertCopy(ctx, repository.Copy{Item: copied})
	}); err != nil {
		t.Fatalf("copying: %v", err)
	}
	if got := findItem(ctx, t, tenantA, copied.ID).Amount; got != "1 kg" {
		t.Errorf("the copy's amount = %q", got)
	}

	cleared := findItem(ctx, t, tenantA, task.ID)
	cleared.Amount = ""
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetAttributes(ctx, cleared, cleared.Version)
	}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	var stored *string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT amount FROM work_item WHERE tenant_id = $1 AND id = $2`,
		tenantA.String(), task.ID.String()).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != nil {
		t.Errorf("a cleared amount is stored as %q, want NULL", *stored)
	}
}

// The column's own guarantee, for a writer that reaches it without the domain.
func TestTheRowRefusesAnAmountOutsideItsBounds(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	task := taskIn(tenantA, authorA, collection, freshID(t), "milk", "a0")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, task)
	}); err != nil {
		t.Fatal(err)
	}
	for _, amount := range []string{"", "12345678901234567890123456789012345678901"} {
		if _, err := adminPool(ctx, t).Exec(ctx,
			`UPDATE work_item SET amount = $3 WHERE tenant_id = $1 AND id = $2`,
			tenantA.String(), task.ID.String(), amount); err == nil {
			t.Errorf("the row accepted the amount %q", amount)
		}
	}
}

// Another tenant reads no amount: the entry itself is not found (T-04), as for every other field.
func TestAnAmountIsNotReadAcrossTheTenantBoundary(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	task := taskIn(tenantA, authorA, collection, freshID(t), "milk", "a0")
	task.Amount = "2"
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, task)
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := itemRepo().Find(ctx, task.ID)
		return err
	}); err == nil {
		t.Error("tenant B read tenant A's entry")
	}
}

// listCollectionFor gives a tenant a collection whose own layer is *List*'s, as choosing the preset
// stores it.
func listCollectionFor(ctx context.Context, t *testing.T, tenant, author shared.ID) shared.ID {
	t.Helper()
	collection := collectionFor(ctx, t, tenant, author)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		stored, err := containerRepo().Find(ctx, collection)
		if err != nil {
			return err
		}
		for _, preset := range work.BuiltInPresets() {
			if preset.Key == work.PresetList {
				stored.Structure = work.StructureLayer{Preset: preset.Key, Values: preset.Values}
			}
		}
		return postgres.NewStructureRepository().SetContainerStructure(ctx, stored, stored.Version)
	}); err != nil {
		t.Fatalf("choosing List: %v", err)
	}
	return collection
}

// UC-WRK-25 check 2 through the use case against the seeded profiles: a *List* collection takes
// an amount, a collection on Hubtask's default refuses it, and nothing is written for the refusal.
func TestAnAmountIsTakenInAListAndRefusedElsewhere(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := itemCatalogueFor(ctx, t)
	actor := itemWriter(tenantB, authorB)

	list := listCollectionFor(ctx, t, tenantB, authorB)
	out, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "TASK", "collection_id": list.String(), "title": "milk", "amount": "2",
	})
	if err != nil {
		t.Fatalf("creating in a list: %v", err)
	}
	if got := findItem(ctx, t, tenantB, shared.MustParseID(out.String("id"))).Amount; got != "2" {
		t.Errorf("stored amount %q", got)
	}

	plain := collectionFor(ctx, t, tenantB, authorB)
	_, err = registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "TASK", "collection_id": plain.String(), "title": "milk", "amount": "2",
	})
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("an amount in a collection on the default answered %v", err)
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM work_item WHERE collection_id = $1`, plain.String()); rows != 0 {
		t.Errorf("%d entries written for a refusal", rows)
	}
}

// An archive written before the column existed carries no `amount` key, and restores to none.
func TestARestoreFromAnArchiveWithoutTheAmountLeavesNone(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	task := taskIn(tenantA, authorA, collection, freshID(t), "milk", "a0")
	task.Amount = "2"
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, task)
	}); err != nil {
		t.Fatal(err)
	}
	row := exportedShape(ctx, t, tenantA, task.ID)
	if row["amount"] != "2" {
		t.Fatalf("the export carries amount %#v", row["amount"])
	}
	delete(row, "amount")

	if _, err := adminPool(ctx, t).Exec(ctx,
		`DELETE FROM work_item WHERE tenant_id = $1 AND id = $2`,
		tenantA.String(), task.ID.String()); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := importRepo().Write(ctx, "work_item", row, false)
		return err
	}); err != nil {
		t.Fatalf("restoring the older row: %v", err)
	}
	if got := findItem(ctx, t, tenantA, task.ID).Amount; got != "" {
		t.Errorf("an archive without the column restored amount %q", got)
	}
}
