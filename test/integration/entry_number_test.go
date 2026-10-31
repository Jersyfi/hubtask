// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
)

// Entries that arrive with their numbers - a restore, a bulk load - leave the collection's counter
// row alone, and the next drawn number still comes after every number the collection holds
// (migration 0289). A counter row rewritten per entry inside one transaction made a large load
// grow with the square of its entries.
func TestALoadWithNumbersLeavesTheCounterAloneAndTheDrawCatchesUp(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	admin := adminPool(ctx, t)

	counterVersion := func() string {
		t.Helper()
		var xmin string
		if err := admin.QueryRow(ctx, `SELECT xmin::text FROM container WHERE id = $1`,
			collection.String()).Scan(&xmin); err != nil {
			t.Fatalf("reading the counter row: %v", err)
		}
		return xmin
	}
	before := counterVersion()

	tx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning the load: %v", err)
	}
	for _, number := range []int64{40, 41, 42} {
		id := freshID(t)
		if _, err := tx.Exec(ctx, `
			INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key,
			                       created_by, number)
			VALUES ($1::uuid, $2, $3, 'TASK', '/' || $1::text || '/', 0, 'loaded', 'a0', $4, $5)`,
			id.String(), tenantA.String(), collection.String(), authorA.String(), number); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("loading entry %d: %v", number, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("committing the load: %v", err)
	}
	if after := counterVersion(); after != before {
		t.Errorf("the load rewrote the counter row (xmin %s -> %s)", before, after)
	}

	draw := func() int64 {
		t.Helper()
		var number int64
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			item, err := itemRepo().Insert(ctx, taskIn(tenantA, authorA, collection, freshID(t), freshName(t), "b0"))
			number = item.Number
			return err
		}); err != nil {
			t.Fatalf("drawing a number: %v", err)
		}
		return number
	}
	if got := draw(); got != 43 {
		t.Errorf("the first draw after the load: %d, want 43", got)
	}
	if got := draw(); got != 44 {
		t.Errorf("the second draw: %d, want 44", got)
	}
}
